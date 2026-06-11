// Package plan turns a read-only Analysis into a prioritized, evidence-backed
// refactor plan. It groups findings by signal into refactor items (HMF-001, ...),
// explains each one, and marks which are safe for Humify's conservative apply
// command versus which need a human. plan never touches source — it only reasons
// over the analysis.
package plan

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/schylerryan/humify/internal/humify/analyze"
	"github.com/schylerryan/humify/internal/humify/state"
)

// Item is one ranked refactor pass.
type Item struct {
	ID                 string   `json:"id"` // HMF-001, ...
	Signal             string   `json:"signal"`
	Title              string   `json:"title"`
	Problem            string   `json:"problem"`
	WhyItMatters       string   `json:"why_it_matters"`
	RecommendedChange  string   `json:"recommended_change"`
	ExpectedBenefit    string   `json:"expected_benefit"`
	RiskLevel          string   `json:"risk_level"`        // low|medium|high
	AutomationSafety   string   `json:"automation_safety"` // safe|assisted|manual
	ValidationStrategy string   `json:"validation_strategy"`
	Action             string   `json:"action"`     // machine hint for apply: "quarantine" or ""
	AgentSpec          string   `json:"agent_spec"` // structured prompt for --unsafe-permission agent execution
	Applyable          bool     `json:"applyable"`
	Files              []string `json:"files"`
	FindingIDs         []string `json:"finding_ids"`
	Evidence           []string `json:"evidence"`
}

// Plan is the full prioritized plan.
type Plan struct {
	Schema      int    `json:"schema"`
	Tool        string `json:"tool"`
	Version     string `json:"version"`
	Target      string `json:"target"`
	GeneratedAt string `json:"generated_at"`
	AnalysisAt  string `json:"analysis_at"`
	Goal        string `json:"goal"`
	Items       []Item `json:"items"`
}

// template carries the fixed prose and policy for a finding signal.
type template struct {
	title, problem, why, change, benefit string
	risk, safety                         string
	action                               string
	applyable                            bool
}

// templates maps each finding signal to how its refactor item reads. Only
// stale_file is applyable in the primary version — a reversible quarantine — so
// apply never performs an edit it cannot safely undo.
var templates = map[string]template{
	"stale_file": {
		title: "Quarantine stale files", action: "quarantine", applyable: true,
		problem: "Files that look empty or throwaway add noise and false surface area.",
		why:     "Dead files mislead readers about what the codebase actually contains and uses.",
		change:  "Move each file to .humify/delete-me/<plan-id>/ (reversible), then re-run validation.",
		benefit: "A smaller, honest file tree with zero behavior change if validation still passes.",
		risk:    "low", safety: "safe",
	},
	"swallowed_error": {
		title:   "Stop swallowing errors",
		problem: "Errors are caught and discarded, so real failures pass silently.",
		why:     "Silent failure is the most expensive class of bug to diagnose in production.",
		change:  "Handle, wrap with context, or at minimum log each swallowed error.",
		benefit: "Failures become observable and debuggable instead of invisible.",
		risk:    "high", safety: "manual",
	},
	"broad_catch": {
		title:   "Narrow broad exception handlers",
		problem: "Catch-all handlers absorb errors the author never anticipated.",
		why:     "A broad catch hides new failure modes behind code that looks defensive.",
		change:  "Catch the specific exception you can handle; let the rest propagate.",
		benefit: "Unexpected errors surface instead of being quietly absorbed.",
		risk:    "medium", safety: "manual",
	},
	"giant_file": {
		title:   "Split giant files",
		problem: "Oversized files usually bundle several unrelated responsibilities.",
		why:     "Large files are hard to navigate, review, and test as a unit.",
		change:  "Separate the file by responsibility into cohesive, individually testable units.",
		benefit: "Each unit becomes readable and changeable in isolation.",
		risk:    "medium", safety: "manual",
	},
	"long_function": {
		title:   "Break up long functions",
		problem: "Long functions perform many unrelated steps in one scope.",
		why:     "A reader must hold the whole function in their head to change one line.",
		change:  "Extract cohesive helpers with intention-revealing names.",
		benefit: "Smaller functions are easier to name, test, and reason about.",
		risk:    "medium", safety: "manual",
	},
	"deep_nesting": {
		title:   "Flatten deep nesting",
		problem: "Deeply nested control flow obscures the path a value takes.",
		why:     "Each nesting level multiplies the states a reader must track.",
		change:  "Use early returns and guard clauses to flatten the happy path.",
		benefit: "Control flow becomes linear and skimmable.",
		risk:    "low", safety: "assisted",
	},
	"vague_name": {
		title:   "Rename vague symbols",
		problem: "Names like data, manager, and helper hide what the code is for.",
		why:     "A reader cannot trust code they cannot name.",
		change:  "Rename to the concept the symbol represents, updating all references.",
		benefit: "The code documents its own intent.",
		risk:    "low", safety: "assisted",
	},
	"noisy_comment": {
		title:   "Remove noise comments",
		problem: "Comments that restate the code add maintenance cost and no information.",
		why:     "Redundant comments drift out of sync and train readers to ignore comments.",
		change:  "Delete restating comments; keep comments that explain the why.",
		benefit: "Remaining comments are trustworthy and worth reading.",
		risk:    "low", safety: "assisted",
	},
	"todo_marker": {
		title:   "Resolve leftover TODO/FIXME markers",
		problem: "Unfinished markers signal abandoned or machine-stubbed work.",
		why:     "Stale markers accumulate into uncertainty about what is actually done.",
		change:  "Resolve each marker or convert it into a tracked issue, then remove it.",
		benefit: "The code reflects finished work, not intentions.",
		risk:    "low", safety: "manual",
	},
}

// order ranks signals into tiers so safe quick wins (the reversible quarantine)
// come first; everything else is ordered by severity weight within its tier.
func order(signal string) int {
	if signal == "stale_file" {
		return 0
	}
	return 1
}

// Build produces a prioritized plan from an analysis.
func Build(a analyze.Analysis) Plan {
	groups := groupBySignal(a.Findings)
	items := make([]Item, 0, len(groups))
	for signal, fs := range groups {
		tpl, ok := templates[signal]
		if !ok {
			continue
		}
		items = append(items, buildItem(signal, tpl, fs))
	}
	sortItems(items, a.Findings)
	for i := range items {
		items[i].ID = fmt.Sprintf("HMF-%03d", i+1)
	}
	return Plan{
		Schema:      state.Schema,
		Tool:        analyze.Tool,
		Version:     analyze.Version,
		Target:      a.Target,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		AnalysisAt:  a.GeneratedAt,
		Goal:        goalLine(a),
		Items:       items,
	}
}

// buildItem assembles one refactor item from a signal's findings.
func buildItem(signal string, tpl template, fs []analyze.Finding) Item {
	item := Item{
		Signal: signal, Title: tpl.title, Problem: tpl.problem, WhyItMatters: tpl.why,
		RecommendedChange: tpl.change, ExpectedBenefit: tpl.benefit,
		RiskLevel: tpl.risk, AutomationSafety: tpl.safety, Action: tpl.action,
		Applyable:          tpl.applyable,
		ValidationStrategy: validationFor(tpl),
	}
	seenFile := map[string]bool{}
	for _, f := range fs {
		item.FindingIDs = append(item.FindingIDs, f.ID)
		if !seenFile[f.File] {
			seenFile[f.File] = true
			item.Files = append(item.Files, f.File)
		}
		if len(item.Evidence) < 5 {
			item.Evidence = append(item.Evidence, fmt.Sprintf("%s:%d %s", f.File, f.Line, f.Evidence))
		}
	}
	sort.Strings(item.Files)
	item.AgentSpec = buildAgentSpec(signal, tpl, item)
	return item
}

// signalInstructions gives the precise transformation rule an agent must apply
// for each signal when --unsafe-permission is used. These are injected verbatim
// into the AgentSpec so the agent has unambiguous instructions without guessing.
var signalInstructions = map[string]string{
	"swallowed_error": "Find each empty or near-empty error handler (empty catch block, `except: pass`, `if err != nil { }` with no body, etc.) and replace it with an action: return the error with context, log it and re-raise, or handle it explicitly. Never leave the handler empty. If you cannot determine the right action from context alone, add a log line and re-raise — that is always better than silence.",
	"broad_catch":     "Find each broad exception handler (`except Exception`, `catch (Exception e)`, `rescue StandardError`, etc.) and narrow it to the specific exception type(s) the wrapped code can actually raise. Read the try/catch body to determine which exceptions are plausible. If multiple types are possible, catch each explicitly. Do not change the handler's body logic.",
	"giant_file":      "Separate the file by responsibility. Read the file, identify its distinct concerns, and extract each into its own file with a name that states its responsibility. Update all import sites. The original file should shrink to a thin facade or be deleted if it is fully superseded.",
	"long_function":   "Extract cohesive sub-steps out of each long function into helpers with intention-revealing names. Each extracted helper should do exactly one thing and be nameable without 'and'. Do not change observable behavior — only reorganize structure.",
	"deep_nesting":    "Flatten deeply nested control flow using early returns and guard clauses. The outermost happy path should be linear. Move the error/edge cases to the top so they return or raise early, leaving the main path unindented.",
	"vague_name":      "Rename each vague symbol (data, manager, helper, result, info, obj, etc.) to a name that states what it represents in the domain. Update every reference. Do not rename symbols whose names are correct; only rename the ones listed in the evidence.",
	"noisy_comment":   "Delete each comment that merely restates the code (e.g. `// increment i` above `i++`, `# call save` above `save()`). Keep comments that explain WHY, not WHAT. If a comment has useful intent buried in noise, rewrite it to the useful part only.",
	"todo_marker":     "Resolve each TODO/FIXME/HACK marker. For each one: if the work is done, delete the marker. If it is genuinely outstanding, convert it to a specific issue reference or a concrete comment that names the constraint. Never leave a vague marker with no resolution path.",
	"stale_file":      "Remove or archive each file listed. Confirm it has no live importers or callers before removing. If it is imported, investigate whether the import is itself dead code and trace the dependency chain before acting.",
}

// buildAgentSpec constructs a structured prompt an agent can execute verbatim
// when --unsafe-permission is used. It names the exact files, evidence, and
// transformation rule so the agent needs no additional context to act.
func buildAgentSpec(signal string, tpl template, item Item) string {
	if tpl.safety == "safe" {
		return "" // safe items use the quarantine path, not an agent
	}
	instr, ok := signalInstructions[signal]
	if !ok {
		instr = tpl.change
	}
	var b strings.Builder
	b.WriteString("You are executing a targeted refactor for humify. Apply the following change precisely and conservatively.\n\n")
	fmt.Fprintf(&b, "Signal: %s\nTask: %s\n\n", signal, tpl.title)
	b.WriteString("Files to modify:\n")
	for _, f := range item.Files {
		fmt.Fprintf(&b, "  - %s\n", f)
	}
	if len(item.Evidence) > 0 {
		b.WriteString("\nEvidence (file:line finding):\n")
		for _, e := range item.Evidence {
			fmt.Fprintf(&b, "  - %s\n", e)
		}
	}
	fmt.Fprintf(&b, "\nTransformation rule:\n  %s\n", instr)
	b.WriteString("\nConstraints:\n")
	b.WriteString("  - Do not change observable behavior beyond what the transformation requires.\n")
	b.WriteString("  - Do not touch files not listed above. This is a hard rule — no exceptions.\n")
	b.WriteString("  - Never modify generated or compiled output: dist/, build/, .next/, out/, coverage/, node_modules/, vendor/, target/, __pycache__/.\n")
	b.WriteString("  - Never create planning, notes, or scratch files in the repository.\n")
	b.WriteString("  - If a listed file no longer exists or the finding no longer applies, skip it and say why.\n")
	b.WriteString("  - After all changes, run `humify verify` and report the result.\n")
	b.WriteString("\nWhen complete, output a short summary of what you changed and why.\n")
	return b.String()
}

// validationFor states how to confirm an item's change is behavior-preserving.
func validationFor(tpl template) string {
	if tpl.applyable {
		return "Re-run `humify verify`; if any previously-passing check fails, restore the quarantined files."
	}
	return "Add or confirm characterization tests, then re-run `humify verify` after the manual change."
}

// goalLine summarizes the plan's intent.
func goalLine(a analyze.Analysis) string {
	return fmt.Sprintf("Make %s more human-readable, maintainable, and safe to change by resolving %d findings across %d source files, highest-impact first.",
		a.Target, len(a.Findings), a.Summary.SourceFiles)
}

// groupBySignal buckets findings by their signal.
func groupBySignal(findings []analyze.Finding) map[string][]analyze.Finding {
	groups := map[string][]analyze.Finding{}
	for _, f := range findings {
		groups[f.Signal] = append(groups[f.Signal], f)
	}
	return groups
}

// sortItems orders items by signal priority, then by total severity weight
// (descending), then by title for stability.
func sortItems(items []Item, findings []analyze.Finding) {
	weight := penaltyBySignal(findings)
	sort.SliceStable(items, func(i, j int) bool {
		if oi, oj := order(items[i].Signal), order(items[j].Signal); oi != oj {
			return oi < oj
		}
		if weight[items[i].Signal] != weight[items[j].Signal] {
			return weight[items[i].Signal] > weight[items[j].Signal]
		}
		return items[i].Title < items[j].Title
	})
}

// penaltyBySignal sums a severity weight per signal to rank impact.
func penaltyBySignal(findings []analyze.Finding) map[string]int {
	w := map[string]int{}
	for _, f := range findings {
		switch f.Severity {
		case "major":
			w[f.Signal] += 10
		case "warning":
			w[f.Signal] += 5
		default:
			w[f.Signal] += 2
		}
	}
	return w
}

// Find returns the item with the given id, or false.
func (p Plan) Find(id string) (Item, bool) {
	for _, it := range p.Items {
		if it.ID == id {
			return it, true
		}
	}
	return Item{}, false
}
