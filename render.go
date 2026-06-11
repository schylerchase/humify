package main

// Terminal and markdown rendering for the product commands. These functions only
// format already-computed state; they make no decisions.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/schylerryan/humify/internal/humify/analyze"
	"github.com/schylerryan/humify/internal/humify/apply"
	hplan "github.com/schylerryan/humify/internal/humify/plan"
	hstate "github.com/schylerryan/humify/internal/humify/state"
	"github.com/schylerryan/humify/internal/humify/verify"
)

const topFindings = 8
const worstFiles = 5

// printAnalysis renders an analysis summary to the terminal.
func printAnalysis(a analyze.Analysis) {
	fmt.Printf("Humify analysis — %s\n", a.Target)
	fmt.Printf("stack: %s · package manager: %s\n", join(a.Project.Stack), a.Project.PackageManager)
	fmt.Printf("files: %d source · %d test · %d config\n",
		a.Project.Counts.Source, a.Project.Counts.Test, a.Project.Counts.Config)
	fmt.Printf("\noverall health: %d/100  (higher is healthier)\n", a.Scores.Overall)
	s := a.Scores
	fmt.Printf("  readability %-3d maintainability %-3d correctness %-3d testability %-3d efficiency %-3d\n",
		s.Readability, s.Maintainability, s.Correctness, s.Testability, s.Efficiency)
	fmt.Printf("\nfindings: %d  (%d major, %d warning, %d info)\n", a.Summary.Findings,
		a.Summary.BySeverity["major"], a.Summary.BySeverity["warning"], a.Summary.BySeverity["info"])
	printTopFindings(a.Findings)
	printWorstFiles(a.Files)
	fmt.Println("\nnext: humify plan")
}

func printTopFindings(findings []analyze.Finding) {
	if len(findings) == 0 {
		fmt.Println("  no findings — this repo reads clean to Humify")
		return
	}
	fmt.Println("\ntop findings:")
	for i, f := range findings {
		if i >= topFindings {
			fmt.Printf("  … and %d more (see .humify/analysis.json)\n", len(findings)-topFindings)
			break
		}
		fmt.Printf("  [%-7s] %-16s %s:%d — %s\n", f.Severity, f.Signal, f.File, f.Line, f.Evidence)
	}
}

func printWorstFiles(files []analyze.FileScore) {
	shown := 0
	for _, f := range files {
		if f.Findings == 0 {
			continue
		}
		if shown == 0 {
			fmt.Println("\nlowest-scoring files:")
		}
		fmt.Printf("  %3d/100  %s  (%d findings)\n", f.Score, f.Path, f.Findings)
		if shown++; shown >= worstFiles {
			break
		}
	}
}

// printPlan renders a refactor plan summary.
func printPlan(p hplan.Plan) {
	fmt.Printf("Humify plan — %s\n", p.Target)
	fmt.Printf("goal: %s\n\n", p.Goal)
	if len(p.Items) == 0 {
		fmt.Println("no refactor items — nothing actionable found.")
		return
	}
	var firstApplyable string
	for _, it := range p.Items {
		tag := it.AutomationSafety
		if it.Applyable {
			tag += ", auto-applyable"
			if firstApplyable == "" {
				firstApplyable = it.ID
			}
		}
		fmt.Printf("  %s  %-30s [%s] — %d file(s), %d finding(s)\n",
			it.ID, it.Title, tag, len(it.Files), len(it.FindingIDs))
	}
	if firstApplyable != "" {
		fmt.Printf("\nnext: humify apply --target %s --dry-run\n", firstApplyable)
	} else {
		fmt.Println("\nnext: address the highest-ranked item by hand, then `humify verify`")
	}
}

// printValidation renders a validation report.
func printValidation(v verify.Validation) {
	fmt.Printf("Humify verify — %s\n", v.Target)
	for _, c := range v.Commands {
		switch {
		case c.Skipped:
			fmt.Printf("  [skip] %s\n", c.Reason)
		case c.Passed:
			fmt.Printf("  [pass] %-9s %s\n", c.Kind, c.Command)
		default:
			fmt.Printf("  [FAIL] %-9s %s  (exit %d)\n", c.Kind, c.Command, c.ExitCode)
		}
	}
	switch {
	case !v.Validated:
		fmt.Println("overall: NOT VALIDATED — no validation commands detected for this project")
	case v.Passed:
		fmt.Println("overall: PASSED")
	default:
		fmt.Println("overall: FAILED (see .humify/validation.json)")
	}
}

// printApply renders the outcome of an apply.
func printApply(res apply.Result) {
	fmt.Printf("Humify apply — %s\n", res.ItemID)
	if res.RepoDirty {
		fmt.Println("warning: the repository has uncommitted changes — review carefully before trusting this apply.")
	}
	fmt.Println(res.Message)
	// On a rollback or a refusal nothing remains moved, so listing moves would
	// mislead; the message already explains the outcome.
	if res.RolledBack || res.Skipped {
		return
	}
	for _, m := range res.Moves {
		arrow := "would move"
		if res.Applied {
			arrow = "moved"
		}
		fmt.Printf("  %s %s → %s\n", arrow, m.Original, m.New)
	}
	if res.Applied {
		fmt.Printf("manifest: %s\n", relTo(res.ItemID))
	}
}

// printStatus renders whichever Humify state files are present.
func printStatus(root string, a analyze.Analysis, haveA bool, p hplan.Plan, haveP bool, v verify.Validation, haveV bool) {
	fmt.Printf("Humify status — %s\n", root)
	if !haveA && !haveP && !haveV {
		fmt.Println("no Humify state yet — run `humify analyze` to begin.")
		return
	}
	if haveA {
		fmt.Printf("  analysis : %d findings, overall %d/100 (%s)\n", a.Summary.Findings, a.Scores.Overall, a.GeneratedAt)
	}
	if haveP {
		fmt.Printf("  plan     : %d refactor items (%s)\n", len(p.Items), p.GeneratedAt)
	}
	if haveV {
		fmt.Printf("  verify   : %s (%s)\n", verifyVerdict(v), v.GeneratedAt)
	}
}

// printDoctor renders the doctor checklist.
func printDoctor(root string, checks []check) {
	fmt.Printf("Humify doctor — %s\n", root)
	for _, c := range checks {
		fmt.Printf("  [%-4s] %-16s %s\n", c.Status, c.Name, c.Detail)
	}
}

// verifyVerdict reports an honest status: a validation that ran nothing is "NOT
// VALIDATED", never "PASSED". On failure it names the failing kind(s) and notes any
// that passed, so a single failing check does not read as a total wipeout — and so a
// pre-existing failure (which apply tolerates) is not mistaken for a hard block.
func verifyVerdict(v verify.Validation) string {
	if !v.Validated {
		return "NOT VALIDATED"
	}
	if v.Passed {
		return "PASSED"
	}
	var failed, passed []string
	for _, c := range v.Commands {
		if !c.Ran {
			continue
		}
		if c.Passed {
			passed = append(passed, c.Kind)
		} else {
			failed = append(failed, c.Kind)
		}
	}
	if len(failed) == 0 {
		return "FAILED"
	}
	verdict := "FAILED: " + strings.Join(failed, ", ")
	if len(passed) > 0 {
		verdict += " (" + strings.Join(passed, ", ") + " passed)"
	}
	return verdict
}

func relTo(planID string) string {
	return filepath.ToSlash(filepath.Join(hstate.Dir, hstate.DeleteMeDir, planID, "manifest.json"))
}

func join(xs []string) string {
	if len(xs) == 0 {
		return "none"
	}
	return strings.Join(xs, ", ")
}

// writeAnalysisMarkdown writes the optional human report .humify/HUMIFY_REPORT.md.
func writeAnalysisMarkdown(root string, a analyze.Analysis) {
	var b strings.Builder
	fmt.Fprintf(&b, "# Humify Report — %s\n\n", a.Target)
	fmt.Fprintf(&b, "## Project Summary\n\n- Stack: %s\n- Package manager: %s\n- Files: %d source, %d test, %d config\n- Entry points: %s\n\n",
		join(a.Project.Stack), a.Project.PackageManager, a.Project.Counts.Source, a.Project.Counts.Test, a.Project.Counts.Config, join(a.Project.EntryPoints))
	fmt.Fprintf(&b, "## Overall Humify Score\n\n**%d / 100** (higher is healthier)\n\n", a.Scores.Overall)
	s := a.Scores
	b.WriteString("## Category Scores\n\n| Category | Score |\n| --- | --- |\n")
	fmt.Fprintf(&b, "| Readability | %d |\n| Maintainability | %d |\n| Correctness Risk | %d |\n| Testability | %d |\n| Efficiency | %d |\n\n",
		s.Readability, s.Maintainability, s.Correctness, s.Testability, s.Efficiency)
	b.WriteString("## Top Findings\n\n| Severity | Signal | Location | Evidence |\n| --- | --- | --- | --- |\n")
	for i, f := range a.Findings {
		if i >= 20 {
			break
		}
		fmt.Fprintf(&b, "| %s | %s | %s:%d | %s |\n", f.Severity, f.Signal, mdEscape(f.File), f.Line, mdEscape(f.Evidence))
	}
	b.WriteString("\n## AI-Slop Signals\n\n")
	for sig, n := range a.Summary.BySignal {
		fmt.Fprintf(&b, "- %s: %d\n", sig, n)
	}
	b.WriteString("\n## Recommended Next Command\n\n`humify plan`\n")
	writeUnder(root, "HUMIFY_REPORT.md", b.String())
}

// writePlanMarkdown writes the optional human plan .humify/HUMIFY_PLAN.md.
func writePlanMarkdown(root string, p hplan.Plan) {
	var b strings.Builder
	fmt.Fprintf(&b, "# Humify Plan — %s\n\n## Goal\n\n%s\n\n## Refactor Passes\n\n", p.Target, p.Goal)
	for _, it := range p.Items {
		fmt.Fprintf(&b, "### %s — %s\n\n", it.ID, it.Title)
		fmt.Fprintf(&b, "- **Problem:** %s\n- **Why it matters:** %s\n- **Recommended change:** %s\n- **Expected benefit:** %s\n",
			it.Problem, it.WhyItMatters, it.RecommendedChange, it.ExpectedBenefit)
		fmt.Fprintf(&b, "- **Risk:** %s · **Automation safety:** %s\n- **Validation:** %s\n- **Files:** %s\n\n",
			it.RiskLevel, it.AutomationSafety, it.ValidationStrategy, join(it.Files))
	}
	writeUnder(root, "HUMIFY_PLAN.md", b.String())
}

// writeUnder writes a markdown file under .humify/, best effort.
func writeUnder(root, name, content string) {
	dir := filepath.Join(root, hstate.Dir)
	if os.MkdirAll(dir, 0o755) == nil {
		_ = os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
	}
}

// mdEscape neutralizes the pipe so evidence text cannot break a markdown table.
func mdEscape(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
