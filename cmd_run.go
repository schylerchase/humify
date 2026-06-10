package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"humify/internal/audit"
	"humify/internal/handoff"
	"humify/internal/layout"
	"humify/internal/output"
	"humify/internal/pipeline"
	"humify/internal/plan"
	"humify/internal/spawn"
)

// untangleRun is the autonomous driver: it walks the same next-step decision
// `humify resume` prints (nextActionable) and ACTS on it, advancing one stage per
// loop iteration — spawning the stage's agents where a stage needs them — until
// the pipeline reaches done or hits a genuine blocker. It is "resume that acts":
// no new state machine, just the existing disk-derived reducer with an executor
// bolted on, so the advisory and acting surfaces can never disagree.
//
// Termination rests on three independent layers, because no single one is
// complete:
//  1. Stage-intrinsic bounds. plan self-bounds via its replan budget (→ escalated,
//     surfaced as Blocked by nextActionable's guard); execute self-bounds via the
//     merge barrier and the build/test gate (→ Blocked runStep).
//  2. A no-progress guard over VALIDATED state (progressSig). An agent that writes
//     an invalid fragment, an invalid plan-check, or nothing at all advances no
//     validated artifact, so two consecutive iterations read identical and the run
//     is declared stuck — rather than spinning to the cap on raw file churn.
//  3. A hard iteration cap backstops any logic bug.
//
// Source mutation is gated. Without --execute the driver stops at plan-converged:
// audit, consolidate, and plan only ever write under .humify/, never your source.
// With --execute it continues through execute (forking worktrees, spawning
// executors that rewrite source, auto-merging behind the barrier + optional gate)
// and patchlog. Autonomy over source is opt-in, mirroring apply's --yes.
func untangleRun(opts options) int {
	root := resolveRoot(opts)
	if opts.agentCmd == "" {
		return fail(opts, "missing_agent_cmd", exitError,
			"the autonomous driver requires --agent-cmd (the agent command; each stage's prompt is piped to it on stdin)")
	}
	cfg := spawn.Config{AgentCmd: opts.agentCmd, Jobs: opts.jobs, Timeout: opts.timeout}

	// The manual stage commands each leave a handoff cursor; the driver advances the
	// same disk state across many stages but left none, so a pre-run cursor (e.g.
	// heatmap's "humify audit") survived and made the next resume report a false
	// stale_handoff. Refresh it on every exit so resume/status agree with disk.
	defer recordRunCursor(root)

	var steps []runStep
	stuck := 0
	maxIters := iterationCap(root, opts.maxReplans)
	for i := 0; i < maxIters; i++ {
		step := nextActionable(root)
		if code, reason, msg, halt := terminalVerdict(step, opts); halt {
			return emitRun(opts, reason, code, steps, msg)
		}
		before := progressSig(root)
		st, err := advanceStage(root, step.Stage, opts, cfg)
		if err != nil {
			return fail(opts, "drive_error", exitError, fmt.Sprintf("advancing %s: %v", step.Stage, err))
		}
		steps = append(steps, st)
		if st.Blocked {
			// A stage-level block (merge barrier or build/test gate) is terminal and
			// invisible to pipeline.Next — stop here rather than loop into the same wall.
			return emitRun(opts, "blocked", exitDrift, steps, "blocked at execute: "+st.Note)
		}
		if progressSig(root) == before {
			if stuck++; stuck >= 2 {
				return emitRun(opts, "stuck", exitDrift, steps, stuckMsg(step, st))
			}
		} else {
			stuck = 0
		}
	}
	return fail(opts, "iteration_cap", exitError,
		"driver hit its iteration cap without converging — likely a logic bug; inspect .humify/ and run `humify resume`")
}

// recordRunCursor refreshes the one-shot handoff cursor to match the disk-derived
// next step at the moment the driver returns, so a follow-up `resume`/`status`
// agrees instead of flagging the pre-run cursor the driver advanced past. When
// nothing is actionable (pipeline done), it clears any prior cursor rather than
// writing a blank one that would itself read as stale. nextActionable is the same
// disk-derived decision resume prints, so the cursor and disk can never disagree.
func recordRunCursor(root string) {
	step := nextActionable(root)
	if step.NextCommand == "" {
		_, _, _ = handoff.Consume(root)
		return
	}
	saveHandoff(root, handoff.Handoff{
		Stage:       string(step.Stage),
		Action:      "proceed",
		NextCommand: step.NextCommand,
		Note:        "advanced by untangle run",
	})
}

// runStep is one driver iteration's record, for the trace the driver prints.
type runStep struct {
	Stage     string   `json:"stage"`
	Spawned   int      `json:"spawned,omitempty"`
	Succeeded int      `json:"succeeded,omitempty"`
	Failed    []string `json:"failed,omitempty"`
	Blocked   bool     `json:"blocked,omitempty"`
	Note      string   `json:"note,omitempty"`
}

// terminalVerdict classifies a step the driver cannot or must not advance: the
// pipeline is done, a genuine blocker needs a human, the project needs
// bootstrapping (heatmap is the human's deliberate entry, never the driver's), or
// execute is next but --execute was not given (stop at plan-converged). halt=false
// means the step is an ordinary advanceable stage.
func terminalVerdict(step pipeline.Step, opts options) (code int, reason, msg string, halt bool) {
	switch {
	case step.Stage == pipeline.StageDone:
		return exitOK, "complete", "pipeline complete — nothing left to do", true
	case step.Blocked:
		return exitDrift, step.Reason,
			fmt.Sprintf("blocked at %s: %s\n  resolve, then: %s", step.Stage, step.Detail, step.NextCommand), true
	case step.Stage == pipeline.StageHeatmap:
		return exitDrift, step.Reason, "run heatmap first — " + step.Detail + ": " + step.NextCommand, true
	case step.Stage == pipeline.StageExecute && !opts.execute:
		return exitOK, "plan_converged",
			"plans accepted — re-run with --execute to rewrite source through execute (forks worktrees, auto-merges)", true
	default:
		return 0, "", "", false
	}
}

// progressSig is a signature of the VALIDATED state the pipeline reducer consumes,
// not raw file bytes. It changes only when an agent advances real, validated work:
//   - audit: BuildPlan.Done lists only fragments that parse, validate, and match
//     their area — an invalid or junk fragment never moves it;
//   - consolidate/patchlog: presence of the rolled-up AUDIT.md / PATCHLOG.md;
//   - plan: per area, a present PLAN.md, a VALID PLAN-CHECK.json, acceptance, and
//     the replan counter (so a genuine re-plan round — which keeps the area
//     un-accepted, so counts look unchanged — still reads as progress);
//   - execute: the merged SUMMARY.
//
// So an auditor that writes an invalid fragment, or a checker that writes an
// invalid verdict, makes no progress here and trips the stuck guard — instead of
// churning a byte/mtime fingerprint and spinning to the iteration cap.
func progressSig(root string) string {
	var b strings.Builder
	if p, err := audit.BuildPlan(root); err == nil {
		fmt.Fprintf(&b, "frag:%s|pend:%d;", strings.Join(p.Done, ","), len(p.Pending))
	}
	fmt.Fprintf(&b, "auditmd:%t;patchlog:%t;",
		fileExists(layout.AuditFile(root)), fileExists(layout.PatchlogFile(root)))
	ids, _ := layout.DiscoverAreas(root)
	st, _ := plan.Load(root)
	for _, o := range plan.Observe(root, ids) {
		fmt.Fprintf(&b, "a:%s:%t%t%t:%d:%t;", o.AreaID, o.HasPlan, o.HasCheck, o.Accepted(),
			st.Areas[o.AreaID].Replans, fileExists(layout.AreaSummary(root, o.AreaID)))
	}
	return b.String()
}

// iterationCap bounds the driver loop far above any real run: audit + consolidate,
// a bounded plan loop per area (≤ maxReplans+… rounds), up to two execute steps per
// area (fork + merge), and patchlog. It exists only so a logic bug terminates with
// a diagnostic instead of spinning; the no-progress guard is what actually catches
// stuck agents, long before this. maxReplans (the operator's --max-replans, or the
// default) sizes the plan-loop term so the cap tracks the real replan budget.
func iterationCap(root string, maxReplans int) int {
	if maxReplans <= 0 {
		maxReplans = plan.DefaultMaxReplans
	}
	ids, _ := layout.DiscoverAreas(root)
	n := len(ids)
	if n < 1 {
		n = 1
	}
	return 50 + n*(maxReplans+6)*2
}

// guardAreaIDs fails closed if any id is unsafe to interpolate into a derived path
// (a ".." or separator that would escape the project root). Area ids come from the
// manifest/fragments, which are not charset-validated on the read path.
func guardAreaIDs(ids ...string) error {
	for _, id := range ids {
		if !layout.SafeAreaID(id) {
			return fmt.Errorf("unsafe area id %q (would escape the project root)", id)
		}
	}
	return nil
}

func stuckMsg(step pipeline.Step, st runStep) string {
	return fmt.Sprintf("no progress at %s after a retry — a stage produced nothing valid (failed: %s); inspect %s and run `humify resume`",
		step.Stage, joinOrNone(st.Failed), filepath.Join(layout.Dir, "tmp"))
}

func joinOrNone(ids []string) string {
	if len(ids) == 0 {
		return "none"
	}
	return strings.Join(ids, " ")
}

// emitRun renders the driver's trace and final verdict. ok runs exit 0; a block,
// a stuck stage, an unbootstrapped project, or the iteration cap exit 2/1.
func emitRun(opts options, reason string, code int, steps []runStep, msg string) int {
	if opts.json {
		output.EmitJSON(os.Stdout, output.Result{Ok: code == exitOK, ReasonCode: reason,
			Data: map[string]any{"steps": steps, "message": msg}})
		return code
	}
	for _, s := range steps {
		marker := "✓"
		if s.Blocked {
			marker = "✗"
		}
		line := fmt.Sprintf("  %s %s", marker, s.Stage)
		if s.Spawned > 0 {
			line += fmt.Sprintf(" — spawned %d, ok %d", s.Spawned, s.Succeeded)
			if len(s.Failed) > 0 {
				line += ", failed " + strings.Join(s.Failed, " ")
			}
		}
		if s.Note != "" {
			line += " (" + s.Note + ")"
		}
		fmt.Println(line)
	}
	fmt.Println(msg)
	return code
}
