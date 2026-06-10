package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"humify/internal/audit"
	"humify/internal/handoff"
	"humify/internal/intel"
	"humify/internal/layout"
	"humify/internal/output"
)

// cmdAudit plans the audit fan-out: derive which areas still need an auditor,
// then run the selected runner. The default dispatch runner writes one prompt
// per pending area for the orchestrator to spawn; the gather barrier and merge
// are the separate `humify consolidate` stage. Dispatching is a success (exit
// 0) even when areas remain pending — work to do is not a failure. Manifest/
// intel drift (a manifest area absent from intel) is corruption and exits 2.
func untangleAudit(opts options) int {
	root := opts.root
	if root == "" {
		if found, ok := layout.FindRoot(opts.path); ok {
			root = found
		} else {
			root = "."
		}
	}
	plan, err := audit.BuildPlan(root)
	if err != nil {
		return fail(opts, auditReason(err), exitError, err.Error())
	}
	runner, err := selectRunner(opts)
	if err != nil {
		return fail(opts, "unknown_runner", exitError, err.Error())
	}
	out, err := runner.Dispatch(plan)
	if err != nil {
		return fail(opts, "dispatch_error", exitError, "dispatch failed: "+err.Error())
	}
	return emitAudit(opts, plan, out)
}

// selectRunner maps --runner to an implementation. dispatch only writes prompts;
// spawn (alias: claude) also runs an operator-supplied --agent-cmd per area,
// barriers on all of them, and reports which produced a valid fragment.
func selectRunner(opts options) (audit.Runner, error) {
	switch opts.runner {
	case "", "dispatch":
		return audit.DispatchRunner{}, nil
	case "spawn", "claude":
		if opts.agentCmd == "" {
			return nil, errors.New("the spawn runner requires --agent-cmd (the agent command; the prompt is piped to it on stdin)")
		}
		return audit.SpawnRunner{AgentCmd: opts.agentCmd, Jobs: opts.jobs, Timeout: opts.timeout}, nil
	default:
		return nil, fmt.Errorf("unknown runner %q (use: dispatch, spawn)", opts.runner)
	}
}

func auditReason(err error) string {
	switch {
	case errors.Is(err, intel.ErrNotExist):
		return "no_intel"
	case errors.Is(err, audit.ErrNoManifest):
		return "no_manifest"
	default:
		return "audit_error"
	}
}

// auditData is the structured payload: the plan plus what the runner did. The
// spawn fields are zero/absent for the dispatch runner (omitempty), so the JSON
// shape only grows the spawn keys when an agent actually ran.
type auditData struct {
	audit.Plan
	Runner    string   `json:"runner"`
	Prompts   []string `json:"prompts,omitempty"`
	Spawned   int      `json:"spawned,omitempty"`
	Succeeded int      `json:"succeeded,omitempty"`
	Failed    []string `json:"failed,omitempty"`
}

// auditResult builds the JSON envelope from a plan + runner outcome. It is the
// single place auditData is assembled, so the dispatch and spawn paths cannot
// drift in what they report.
func auditResult(plan audit.Plan, out audit.Outcome, ok bool, reason string) output.Result {
	return output.Result{
		Ok:         ok,
		ReasonCode: reason,
		Data: auditData{
			Plan: plan, Runner: out.Runner, Prompts: out.Prompts,
			Spawned: out.Spawned, Succeeded: out.Succeeded, Failed: out.Failed,
		},
	}
}

func emitAudit(opts options, plan audit.Plan, out audit.Outcome) int {
	// The spawn runner actually ran the agents, so its result is post-barrier
	// truth (out.Succeeded/Failed), reported separately. Intel drift still wins:
	// a corrupt bootstrap is reported here even if spawn ran, and the spawned
	// fragments persist on disk for a re-run after heatmap is fixed.
	if len(plan.Missing) == 0 && out.Spawned > 0 {
		return emitSpawn(opts, plan, out)
	}
	ok := len(plan.Missing) == 0
	code := exitOK
	reason := "dispatch_ready"
	switch {
	case len(plan.Missing) > 0:
		reason, code = "intel_drift", exitDrift
	case len(plan.Pending) == 0:
		reason = "ok"
	}
	switch {
	case len(plan.Missing) > 0:
		saveHandoff(plan.Root, handoff.Handoff{Stage: "audit", Action: "blocked",
			NextCommand: "humify heatmap", Note: "intel drift — re-run heatmap"})
	case len(plan.Pending) == 0:
		saveHandoff(plan.Root, handoff.Handoff{Stage: "audit", Action: "proceed",
			NextCommand: "humify consolidate", Note: "all areas audited — gather fragments into AUDIT.md"})
	default:
		saveHandoff(plan.Root, handoff.Handoff{Stage: "audit", Action: "spawn",
			NextCommand: "humify consolidate", Prompts: out.Prompts,
			Note: "spawn one read-only auditor per prompt, then consolidate"})
	}
	if opts.json {
		output.EmitJSON(os.Stdout, auditResult(plan, out, ok, reason))
		return code
	}

	fmt.Printf("audit dispatch: %d areas, %d pending, %d already audited\n",
		plan.Total, len(plan.Pending), len(plan.Done))
	if len(plan.Missing) > 0 {
		fmt.Printf("INTEL DRIFT: %d manifest area(s) absent from intel: %s — re-run `humify heatmap`\n",
			len(plan.Missing), strings.Join(plan.Missing, ", "))
	}
	if len(plan.Pending) == 0 {
		fmt.Println("all areas audited — run `humify consolidate` to gather fragments into AUDIT.md")
		return code
	}
	fmt.Printf("runner: %s — wrote %d auditor prompt(s) under %s\n",
		out.Runner, len(out.Prompts), filepath.Join(layout.Dir, "tmp", "auditors"))
	printWaves(plan.Pending)
	fmt.Println("next: spawn one read-only auditor per prompt (they are independent — auditing never writes source),")
	fmt.Println("      then `humify consolidate` to merge fragments into AUDIT.md")
	return code
}

// emitSpawn reports the spawn runner's post-barrier reconciliation. The agents
// have already run, so success is the fragments that actually appeared
// (out.Succeeded / out.Failed) — never the pre-run plan. An area whose agent ran
// but left no valid fragment is real drift: surfaced (exit 2), not swallowed.
// Re-running audit retries only the stragglers (BuildPlan skips audited areas).
func emitSpawn(opts options, plan audit.Plan, out audit.Outcome) int {
	ok, code, reason := true, exitOK, "spawn_complete"
	if len(out.Failed) > 0 {
		ok, code, reason = false, exitDrift, "spawn_incomplete"
	}
	if ok {
		saveHandoff(plan.Root, handoff.Handoff{Stage: "audit", Action: "proceed",
			NextCommand: "humify consolidate",
			Note: fmt.Sprintf("%d auditor(s) spawned, all fragments present — gather into AUDIT.md", out.Spawned)})
	} else {
		saveHandoff(plan.Root, handoff.Handoff{Stage: "audit", Action: "blocked",
			NextCommand: "humify audit",
			Note: fmt.Sprintf("%d of %d auditor(s) wrote no valid fragment: %s — re-run audit to retry",
				len(out.Failed), out.Spawned, strings.Join(out.Failed, ", "))})
	}
	if opts.json {
		output.EmitJSON(os.Stdout, auditResult(plan, out, ok, reason))
		return code
	}
	fmt.Printf("audit spawn: %s — %d spawned, %d valid fragment(s), %d failed\n",
		out.Runner, out.Spawned, out.Succeeded, len(out.Failed))
	if len(out.Failed) > 0 {
		fmt.Printf("FAILED (no valid fragment after the agent ran): %s\n", strings.Join(out.Failed, ", "))
		fmt.Println("re-run `humify audit --runner=spawn --agent-cmd=...` to retry the stragglers (audited areas are skipped)")
		return code
	}
	fmt.Println("all auditors produced fragments — run `humify consolidate` to gather them into AUDIT.md")
	return code
}

// printWaves lists pending areas grouped by topo wave. Auditors are read-only
// and independent, so all may run at once; the grouping is shown only so a
// resource-bounded orchestrator can batch wave-by-wave if it wants.
func printWaves(pending []audit.Job) {
	byWave := map[int][]string{}
	order := []int{}
	for _, j := range pending {
		if _, ok := byWave[j.Wave]; !ok {
			order = append(order, j.Wave)
		}
		byWave[j.Wave] = append(byWave[j.Wave], j.AreaID)
	}
	sort.Ints(order)
	for _, w := range order {
		fmt.Printf("  wave %d: %s\n", w, strings.Join(byWave[w], " "))
	}
}
