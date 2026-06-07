package plan

import (
	"fmt"
	"path"
	"strings"

	"humify-ng/internal/plancheck"
	"humify-ng/internal/textutil"
)

// Finding is one consolidated audit hazard the planner must address. It is a
// flattened view of a consolidate.Merged finding so this package need not
// depend on the consolidate engine.
type Finding struct {
	Severity string
	File     string
	Line     int
	Title    string
	Detail   string
}

// Feedback carries a rejected attempt back into a re-plan: the prior PLAN.md
// text and the checker issues that sank it.
type Feedback struct {
	PriorPlan string
	Issues    []plancheck.Issue
}

// PlannerJob is one area's planning assignment.
type PlannerJob struct {
	AreaID   string
	Target   string
	Files    []string
	Findings []Finding
	PlanPath string    // where to write PLAN.md, relative to project root
	Feedback *Feedback // nil for an initial plan; set for a re-plan
}

// CheckerJob is one area's adversarial plan-review assignment.
type CheckerJob struct {
	AreaID    string
	Target    string
	PlanPath  string // PLAN.md to review, relative to project root
	CheckPath string // where to write PLAN-CHECK.json, relative to project root
}

// RenderPlannerPrompt builds the planner prompt: a code-grounded, behavior-
// preserving refactoring plan that addresses every confirmed audit finding,
// with concrete file paths and a characterization-test scenario per unit.
func RenderPlannerPrompt(j PlannerJob) string {
	target := textutil.ToForwardSlash(j.Target)
	var b strings.Builder
	fmt.Fprintf(&b, "# Humify planner — area %s\n\n", j.AreaID)

	b.WriteString("## Task\n")
	b.WriteString("Turn the confirmed audit findings below into a concrete, behavior-preserving " +
		"refactoring plan for this area. The plan is executed later by a separate worker, so it must " +
		"be specific enough to follow without re-deriving anything: every step names real files and a " +
		"real change. Behaviour must be preserved — each unit that changes code is paired with a " +
		"characterization test that locks the CURRENT behaviour before the edit.\n\n")

	fmt.Fprintf(&b, "- Area: `%s`\n- Target codebase root: `%s`\n", j.AreaID, target)
	b.WriteString("- Files in this area (read them before planning):\n")
	for _, f := range j.Files {
		fmt.Fprintf(&b, "    - `%s`\n", textutil.ToForwardSlash(path.Join(target, f)))
	}
	b.WriteString("\n## Confirmed findings to address (every one must map to a unit)\n")
	if len(j.Findings) == 0 {
		b.WriteString("- (none listed; review the files and plan only what the findings imply)\n")
	}
	for i, f := range j.Findings {
		// Findings flow from auditor agents reading the (untrusted) target code;
		// flatten any embedded newlines so a finding can't forge prompt structure
		// (a fake heading/section) that would hijack the planner.
		fmt.Fprintf(&b, "%d. [%s] `%s:%d` — %s\n      %s\n",
			i+1, f.Severity, textutil.OneLine(f.File), f.Line, textutil.OneLine(f.Title), textutil.OneLine(f.Detail))
	}

	if j.Feedback != nil {
		b.WriteString("\n## This is a RE-PLAN — your previous plan was rejected\n")
		b.WriteString("Fix every issue the checker raised; do not regress what already passed.\n")
		for _, is := range j.Feedback.Issues {
			fmt.Fprintf(&b, "- [%s] %s: %s\n", is.Severity, textutil.OneLine(is.Title), textutil.OneLine(is.Detail))
		}
		if strings.TrimSpace(j.Feedback.PriorPlan) != "" {
			b.WriteString("\n<details><summary>your prior plan (revise it)</summary>\n\n")
			b.WriteString(j.Feedback.PriorPlan)
			b.WriteString("\n</details>\n")
		}
	}

	b.WriteString("\n## Output — write exactly one file\n")
	fmt.Fprintf(&b, "Write the plan to `%s` (relative to the humify project root) as Markdown. "+
		"Do NOT modify any source file. Return a one-line confirmation when done.\n\n", j.PlanPath)
	b.WriteString("Structure it as an ordered list of implementation units. For EACH unit give:\n")
	b.WriteString("- **what & where**: the file(s) and the specific change.\n")
	b.WriteString("- **addresses**: which finding number(s) above it resolves.\n")
	b.WriteString("- **characterization test**: the test that pins current behaviour before the change " +
		"(what it asserts, against which inputs/outputs).\n")
	b.WriteString("- **risk / sequencing**: anything that must happen first, or could break the build.\n")
	return b.String()
}

// RenderCheckerPrompt builds the adversarial, read-only plan-checker prompt. The
// checker writes a structured verdict; zero blocker/warning issues accepts the
// plan and ends the loop for this area.
func RenderCheckerPrompt(j CheckerJob) string {
	target := textutil.ToForwardSlash(j.Target)
	var b strings.Builder
	fmt.Fprintf(&b, "# Humify plan-checker — area %s\n\n", j.AreaID)

	b.WriteString("## Stance\n")
	b.WriteString("You are an adversarial plan reviewer: assume the plan is flawed until you have " +
		"checked it against the code. You are READ-ONLY — you may read the plan and the source, but you " +
		"write only the verdict file below. You do not fix the plan; you find what is wrong with it.\n\n")

	fmt.Fprintf(&b, "- Area: `%s`\n- Target codebase root: `%s`\n", j.AreaID, target)
	fmt.Fprintf(&b, "- Plan to review: `%s` (relative to the humify project root)\n\n", j.PlanPath)

	b.WriteString("## What to check\n")
	b.WriteString("- **Coverage**: does the plan address every confirmed finding, or are some ignored?\n")
	b.WriteString("- **Concreteness**: does each unit name real files and a real change, or is it vague?\n")
	b.WriteString("- **Behaviour preservation**: does every code-changing unit have a characterization " +
		"test that locks current behaviour FIRST? Flag any edit that changes behaviour without one.\n")
	b.WriteString("- **Buildability & sequencing**: would any step break the build, or depend on a later " +
		"step? Is the order safe?\n")
	b.WriteString("- **Scope**: does the plan stay within this area, or silently reach into others?\n\n")

	b.WriteString("## Output — write exactly one file\n")
	fmt.Fprintf(&b, "Write your verdict to `%s` and nothing else. It MUST be JSON of this exact shape:\n\n", j.CheckPath)
	b.WriteString("```json\n")
	fmt.Fprintf(&b, `{
  "area_id": %q,
  "issues": [
    { "severity": "blocker | warning | info", "title": "single-line problem name", "detail": "what is wrong and what the plan must change to fix it" }
  ]
}`, j.AreaID)
	b.WriteString("\n```\n\n")

	b.WriteString("## Rules\n")
	b.WriteString("- `severity` is mandatory and must be exactly `blocker`, `warning`, or `info`.\n")
	b.WriteString("    - `blocker`: the plan is unsafe or wrong as written (would break behaviour/build, or ignores a finding).\n")
	b.WriteString("    - `warning`: a real weakness that should be fixed before executing.\n")
	b.WriteString("    - `info`: a minor suggestion.\n")
	b.WriteString("- `title` must be single-line (no newlines).\n")
	b.WriteString("- If the plan is sound, return `{\"area_id\": \"" + j.AreaID + "\", \"issues\": []}` — " +
		"an empty issues list ACCEPTS the plan and ends the loop for this area. Do not invent issues to fill it.\n")
	return b.String()
}
