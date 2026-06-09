package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"humify-ng/internal/handoff"
	"humify-ng/internal/output"
	"humify-ng/internal/pipeline"
	"humify-ng/internal/plan"
)

// cmdResume names the next step in the pipeline — deterministically and advisory.
// It prints the command to run next (and, when a HANDOFF cursor agrees, the exact
// prompts to spawn) but never runs it, consistent with the binary's "orchestrate,
// don't execute" stance. Disk is authoritative: pipeline.Next derives the step
// from on-disk artifacts; the one-shot HANDOFF cursor only enriches it. A stale
// cursor — left by a command whose dispatched agents have since advanced the
// disk — can therefore never make resume wrong.
func untangleResume(opts options) int {
	root := resolveRoot(opts)
	step := nextActionable(root)
	return emitResume(opts, step, reconcileHandoff(root, step))
}

// nextActionable is the one decision both `resume` (which prints it) and the
// autonomous driver (which acts on it) consume: the disk-derived next step from
// pipeline.Next, refined by the plan-escalation guard. pipeline.Next alone does
// not know the replan budget, so without this guard an area that exhausted its
// budget would read as "plan_pending" forever — resume would loop "run humify
// plan" and the driver would re-spawn planners forever. Sharing one function is
// what guarantees the advisory and acting surfaces can never disagree about
// where the project stands or whether it is blocked. Read-only; no replication
// of plan.Decide's stateful logic (escalatedAreas reads persisted loop state).
func nextActionable(root string) pipeline.Step {
	step := pipeline.Next(root)
	if step.Stage == pipeline.StagePlan {
		if esc := escalatedAreas(root); len(esc) > 0 {
			step.Blocked = true
			step.Reason = "plan_escalated"
			step.NextCommand = "humify plan --max-replans=N  (or fix the plan(s) by hand)"
			step.Detail = fmt.Sprintf("%d area(s) exhausted their replan budget — inspect %s",
				len(esc), strings.Join(esc, " "))
		}
	}
	return step
}

// reconcileHandoff consumes the one-shot cursor and returns a human note. It does
// NOT surface the cursor's prompt list: a spawn cursor records the dispatch that
// just happened, but resume only sees it once disk has advanced to match the
// cursor's next_command — by which point those prompts are already spent. The
// actionable output is next_command; re-running it regenerates the correct, fresh
// prompts. Disk always wins; the cursor only contributes its note (and a stale
// flag when it disagrees with disk).
func reconcileHandoff(root string, step pipeline.Step) string {
	h, found, err := handoff.Consume(root)
	if err != nil || !found {
		return ""
	}
	if sameNextCommand(h.NextCommand, step.NextCommand) {
		return h.Note
	}
	return fmt.Sprintf("stale_handoff: cursor pointed at %q but disk shows %q",
		h.NextCommand, step.NextCommand)
}

// sameNextCommand compares two "humify <verb> ..." strings on their verb, so an
// argument difference (e.g. a "--target=<dir>" placeholder) is not a mismatch.
func sameNextCommand(a, b string) bool {
	va := nextVerb(a)
	return va != "" && va == nextVerb(b)
}

func nextVerb(s string) string {
	if f := strings.Fields(s); len(f) >= 2 && f[0] == "humify" {
		return f[1]
	}
	return ""
}

// escalatedAreas returns areas the plan loop would mark terminally stuck — budget
// exhausted OR stalled — read straight from the persisted loop state, no rerun and
// no mutation. It uses plan.Escalated (the same predicate Decide uses) and only
// consults it where Decide would: an area with a failing check on disk (HasPlan +
// HasCheck + blocking issues). An area merely awaiting its first check is not
// escalated. Without the stall arm, resume would loop "run humify plan" forever on
// an area that stalls below its replan cap.
func escalatedAreas(root string) []string {
	st, err := plan.Load(root)
	if err != nil {
		return nil
	}
	var esc []string
	for id, as := range st.Areas {
		obs := plan.Observe(root, []string{id})
		if len(obs) != 1 {
			continue
		}
		o := obs[0]
		if !o.HasPlan || !o.HasCheck || o.Issues == 0 {
			continue // not in an escalation-decidable state (no failing check yet)
		}
		if plan.Escalated(as, st.MaxReplans, o.Issues) {
			esc = append(esc, id)
		}
	}
	sort.Strings(esc)
	return esc
}

func emitResume(opts options, step pipeline.Step, note string) int {
	code := exitOK
	if step.Blocked {
		code = exitDrift
	}
	if opts.json {
		data := map[string]any{
			"stage": step.Stage, "next_command": step.NextCommand,
			"reason": step.Reason, "detail": step.Detail, "blocked": step.Blocked,
		}
		if note != "" {
			data["note"] = note
		}
		output.EmitJSON(os.Stdout, output.Result{Ok: !step.Blocked, ReasonCode: step.Reason, Data: data})
		return code
	}
	switch {
	case step.Stage == pipeline.StageDone:
		fmt.Println("pipeline complete — nothing to resume")
	case step.Blocked:
		fmt.Printf("BLOCKED at %s: %s\n  resolve, then: %s\n", step.Stage, step.Detail, step.NextCommand)
	default:
		fmt.Printf("next: %s\n  %s\n", step.NextCommand, step.Detail)
	}
	if note != "" {
		fmt.Printf("note: %s\n", note)
	}
	return code
}
