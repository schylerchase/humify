package main

import (
	"fmt"
	"os"
	"strings"

	"humify/internal/output"
	"humify/internal/pipeline"
)

// cmdVerify re-runs a stage's deterministic gate read-only, without doing the
// stage's work. `humify verify <stage>` checks one stage; `humify verify` checks
// every stage in order. Exit 0 when the checked gate(s) pass, 2 on an incomplete
// gate (so CI or a wrapping loop can branch on completeness without trusting an
// agent's self-report), and 1 on a usage error (an unknown stage name).
func untangleVerify(opts options) int {
	root := resolveRoot(opts)
	if opts.stage == "" {
		// Stages before the first-incomplete one are genuinely complete; the
		// first-incomplete one fails; stages after it are simply not reached yet
		// (reporting them PASS would read as "done" when they are merely vacuous).
		nextStage := pipeline.Next(root).Stage
		cutoff := stagePos(nextStage)
		results := make([]pipeline.StageResult, 0, len(pipeline.Order))
		for i, st := range pipeline.Order {
			if i > cutoff {
				results = append(results, pipeline.StageResult{Stage: st, Reason: "pending", Detail: "not reached yet"})
				continue
			}
			results = append(results, pipeline.Check(root, st))
		}
		return emitVerify(opts, results, nextStage == pipeline.StageDone)
	}
	st, ok := parseStage(opts.stage)
	if !ok {
		return fail(opts, "unknown_stage", exitError,
			"unknown stage "+opts.stage+" (valid: "+strings.Join(stageNames(), " ")+")")
	}
	r := pipeline.Check(root, st)
	return emitVerify(opts, []pipeline.StageResult{r}, r.Pass)
}

func parseStage(s string) (pipeline.Stage, bool) {
	for _, st := range pipeline.Order {
		if string(st) == s {
			return st, true
		}
	}
	return "", false
}

func stageNames() []string {
	n := make([]string, len(pipeline.Order))
	for i, st := range pipeline.Order {
		n[i] = string(st)
	}
	return n
}

// stagePos returns a stage's index in Order, or len(Order) for "done" (so a
// completed pipeline leaves no stage marked unreached).
func stagePos(s pipeline.Stage) int {
	for i, st := range pipeline.Order {
		if st == s {
			return i
		}
	}
	return len(pipeline.Order)
}

func emitVerify(opts options, results []pipeline.StageResult, pass bool) int {
	code := exitOK
	if !pass {
		code = exitDrift
	}
	if opts.json {
		reason := "verified"
		if !pass {
			reason = "incomplete"
		}
		output.EmitJSON(os.Stdout, output.Result{Ok: pass, ReasonCode: reason,
			Data: map[string]any{"results": results}})
		return code
	}
	for _, r := range results {
		mark := "PASS"
		switch {
		case r.Reason == "pending":
			mark = " -- "
		case !r.Pass:
			mark = "FAIL"
		}
		fmt.Printf("[%s] %-12s %s\n", mark, r.Stage, r.Detail)
	}
	return code
}
