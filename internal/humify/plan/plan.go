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
	Verification       string   `json:"verification,omitempty"` // behavior-verified|build-only|unmeasured (applyable items)
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

// signalDescriptor is the single source of truth for one finding signal: its plan
// prose and policy, its ordering tier, and the verbatim transformation rule an agent
// applies under --unsafe-permission. Folding what were three independently-keyed
// maps (templates, signalInstructions, order) into one record makes drift between
// them impossible — a descriptor cannot disagree with itself.
type signalDescriptor struct {
	title, problem, why, change, benefit string
	risk, safety                         string
	action                               string
	applyable                            bool
	behaviorPreserving                   bool   // change cannot regress behavior; confirm via re-analyze, not verify
	tier                                 int    // ordering rank; lower runs first
	instruction                          string // agent transformation rule (non-safe signals only)
}

// descriptors maps each finding signal to its full metadata, keyed by the canonical
// analyze.Signal* names. Only the safe signals (stale_file, dead_module) are
// applyable — a reversible quarantine — so apply never performs an edit it cannot
// undo; they carry no agent instruction because buildAgentSpec short-circuits safe
// items. tier puts the reversible quarantines first.
var descriptors = map[string]signalDescriptor{
	analyze.SignalStaleFile: {
		title: "Quarantine stale files", action: "quarantine", applyable: true, tier: 0,
		problem: "Files that look empty or throwaway add noise and false surface area.",
		why:     "Dead files mislead readers about what the codebase actually contains and uses.",
		change:  "Move each file to .humify/delete-me/<plan-id>/ (reversible), then re-run validation.",
		benefit: "A smaller, honest file tree with zero behavior change if validation still passes.",
		risk:    "low", safety: "safe",
	},
	analyze.SignalDeadModule: {
		title: "Quarantine unreferenced modules", action: "quarantine", applyable: true, tier: 1,
		problem: "These source files are imported by no other module and are not entry points, so they look dead.",
		why:     "Dead modules inflate the tree and rot silently — but only an empirical check proves a file is truly unused.",
		change:  "Quarantine each candidate to .humify/delete-me/<plan-id>/ (reversible), re-run validation, and keep the move only if every previously-passing check still passes.",
		benefit: "Genuinely-dead code is removed with proof; a wrong guess is caught by validation and rolled back, not shipped.",
		risk:    "low", safety: "safe",
	},
	analyze.SignalSwallowedError: {
		title: "Stop swallowing errors", tier: 2,
		problem: "Errors are caught and discarded, so real failures pass silently.",
		why:     "Silent failure is the most expensive class of bug to diagnose in production.",
		change:  "Handle, wrap with context, or at minimum log each swallowed error.",
		benefit: "Failures become observable and debuggable instead of invisible.",
		risk:    "high", safety: "manual",
		instruction: "Find each empty or near-empty error handler (empty catch block, `except: pass`, `if err != nil { }` with no body, etc.) and replace it with an action: return the error with context, log it and re-raise, or handle it explicitly. Never leave the handler empty. If you cannot determine the right action from context alone, add a log line and re-raise — that is always better than silence.",
	},
	analyze.SignalBroadCatch: {
		title: "Narrow broad exception handlers", tier: 2,
		problem: "Catch-all handlers absorb errors the author never anticipated.",
		why:     "A broad catch hides new failure modes behind code that looks defensive.",
		change:  "Catch the specific exception you can handle; let the rest propagate.",
		benefit: "Unexpected errors surface instead of being quietly absorbed.",
		risk:    "medium", safety: "manual",
		instruction: "Find each broad exception handler (`except Exception`, `catch (Exception e)`, `rescue StandardError`, etc.) and narrow it to the specific exception type(s) the wrapped code can actually raise. Read the try/catch body to determine which exceptions are plausible. If multiple types are possible, catch each explicitly. Do not change the handler's body logic.",
	},
	analyze.SignalGiantFile: {
		title: "Split giant files", tier: 2,
		problem: "Oversized files usually bundle several unrelated responsibilities.",
		why:     "Large files are hard to navigate, review, and test as a unit.",
		change:  "Separate the file by responsibility into cohesive, individually testable units.",
		benefit: "Each unit becomes readable and changeable in isolation.",
		risk:    "medium", safety: "manual",
		instruction: "Separate the file by responsibility. Read the file, identify its distinct concerns, and extract each into its own file with a name that states its responsibility. Update all import sites. The original file should shrink to a thin facade or be deleted if it is fully superseded.",
	},
	analyze.SignalLongFunction: {
		title: "Break up long functions", tier: 2,
		problem: "Long functions perform many unrelated steps in one scope.",
		why:     "A reader must hold the whole function in their head to change one line.",
		change:  "Extract cohesive helpers with intention-revealing names.",
		benefit: "Smaller functions are easier to name, test, and reason about.",
		risk:    "medium", safety: "manual",
		instruction: "Extract cohesive sub-steps out of each long function into helpers with intention-revealing names. Each extracted helper should do exactly one thing and be nameable without 'and'. Do not change observable behavior — only reorganize structure.",
	},
	analyze.SignalDeepNesting: {
		title: "Flatten deep nesting", tier: 2,
		problem: "Deeply nested control flow obscures the path a value takes.",
		why:     "Each nesting level multiplies the states a reader must track.",
		change:  "Use early returns and guard clauses to flatten the happy path.",
		benefit: "Control flow becomes linear and skimmable.",
		risk:    "low", safety: "assisted",
		instruction: "Flatten deeply nested control flow using early returns and guard clauses. The outermost happy path should be linear. Move the error/edge cases to the top so they return or raise early, leaving the main path unindented.",
	},
	analyze.SignalVagueName: {
		title: "Rename vague symbols", tier: 2,
		problem: "Names like data, manager, and helper hide what the code is for.",
		why:     "A reader cannot trust code they cannot name.",
		change:  "Rename to the concept the symbol represents, updating all references.",
		benefit: "The code documents its own intent.",
		risk:    "low", safety: "assisted", behaviorPreserving: true,
		instruction: "Rename each vague symbol (data, manager, helper, result, info, obj, etc.) to a name that states what it represents in the domain. Update every reference. Do not rename symbols whose names are correct; only rename the ones listed in the evidence.",
	},
	analyze.SignalNoisyComment: {
		title: "Remove noise comments", tier: 2,
		problem: "Comments that restate the code add maintenance cost and no information.",
		why:     "Redundant comments drift out of sync and train readers to ignore comments.",
		change:  "Delete restating comments; keep comments that explain the why.",
		benefit: "Remaining comments are trustworthy and worth reading.",
		risk:    "low", safety: "assisted", behaviorPreserving: true,
		instruction: "Delete each comment that merely restates the code (e.g. `// increment i` above `i++`, `# call save` above `save()`). Keep comments that explain WHY, not WHAT. If a comment has useful intent buried in noise, rewrite it to the useful part only.",
	},
	analyze.SignalTodoMarker: {
		title: "Resolve leftover TODO/FIXME markers", tier: 2,
		problem: "Unfinished markers signal abandoned or machine-stubbed work.",
		why:     "Stale markers accumulate into uncertainty about what is actually done.",
		change:  "Resolve each marker or convert it into a tracked issue, then remove it.",
		benefit: "The code reflects finished work, not intentions.",
		risk:    "low", safety: "manual", behaviorPreserving: true,
		instruction: "Resolve each TODO/FIXME/HACK marker. For each one: if the work is done, delete the marker. If it is genuinely outstanding, convert it to a specific issue reference or a concrete comment that names the constraint. Never leave a vague marker with no resolution path.",
	},
}

// order returns a signal's ranking tier (lower runs first) so safe quick wins (the
// reversible quarantines) come before everything else; unknown signals sort last.
func order(signal string) int {
	if d, ok := descriptors[signal]; ok {
		return d.tier
	}
	return 2
}

// agentFileSizeLimit is the max LOC a file may have to be included in an agent
// spec. Files larger than this tend to exhaust agent context or cause OOM kills;
// they are noted in the spec as "too large" rather than silently dropped.
const agentFileSizeLimit = 3000

// Build produces a prioritized plan from an analysis.
func Build(a analyze.Analysis) Plan {
	fileLOC := make(map[string]int, len(a.Files))
	for _, fs := range a.Files {
		fileLOC[fs.Path] = fs.Metrics.LOC
	}
	deadFiles := deadCandidateFiles(a.Findings)
	groups := groupBySignal(a.Findings)
	items := make([]Item, 0, len(groups))
	for signal, fs := range groups {
		d, ok := descriptors[signal]
		if !ok {
			continue
		}
		items = append(items, buildItem(signal, d, fs, fileLOC, deadFiles))
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

// deadCandidateFiles is the set of files nominated as dead_module candidates. A
// refactor agent skips these — they are slated for a reversible quarantine — but
// they stay in the analysis and the plan. Nothing is erased on an unconfirmed
// heuristic; only apply's validation re-run ever decides a file is really gone.
func deadCandidateFiles(findings []analyze.Finding) map[string]bool {
	dead := map[string]bool{}
	for _, f := range findings {
		if f.Signal == analyze.SignalDeadModule {
			dead[f.File] = true
		}
	}
	return dead
}

// buildItem assembles one refactor item from a signal's findings.
func buildItem(signal string, d signalDescriptor, fs []analyze.Finding, fileLOC map[string]int, deadFiles map[string]bool) Item {
	item := Item{
		Signal: signal, Title: d.title, Problem: d.problem, WhyItMatters: d.why,
		RecommendedChange: d.change, ExpectedBenefit: d.benefit,
		RiskLevel: d.risk, AutomationSafety: d.safety, Action: d.action,
		Applyable:          d.applyable,
		ValidationStrategy: validationFor(d),
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
	item.AgentSpec = buildAgentSpec(signal, d, item, fs, fileLOC, deadFiles)
	return item
}

// buildAgentSpec constructs a structured prompt an agent can execute verbatim
// when --unsafe-permission is used. It names the exact files, evidence, and
// transformation rule (from the signal's descriptor) so the agent needs no
// additional context to act. Files exceeding agentFileSizeLimit LOC are excluded
// with an explicit note so the agent is not overwhelmed and the user knows.
func buildAgentSpec(signal string, d signalDescriptor, item Item, fs []analyze.Finding, fileLOC map[string]int, deadFiles map[string]bool) string {
	if d.safety == "safe" {
		return "" // safe items use the quarantine path, not an agent
	}
	instr := d.instruction
	if instr == "" {
		instr = d.change
	}

	var within, tooLarge, deadCand []string
	modifiable := map[string]bool{}
	for _, f := range item.Files {
		switch {
		case deadFiles[f]:
			deadCand = append(deadCand, f) // slated for dead_module quarantine; do not refactor
		case fileLOC[f] > agentFileSizeLimit:
			tooLarge = append(tooLarge, fmt.Sprintf("%s (%d lines)", f, fileLOC[f]))
		default:
			within = append(within, f)
			modifiable[f] = true
		}
	}

	var b strings.Builder
	b.WriteString("You are executing a targeted refactor for humify. Apply the following change precisely and conservatively.\n\n")
	fmt.Fprintf(&b, "Signal: %s\nTask: %s\n\n", signal, d.title)
	b.WriteString("Files to modify:\n")
	for _, f := range within {
		fmt.Fprintf(&b, "  - %s\n", f)
	}
	if len(tooLarge) > 0 {
		b.WriteString("\nFiles excluded (too large for a single agent pass — address separately):\n")
		for _, f := range tooLarge {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
	}
	if len(deadCand) > 0 {
		b.WriteString("\nFiles excluded (flagged as possibly-dead modules — resolve via the dead_module quarantine first; do not refactor):\n")
		for _, f := range deadCand {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
	}
	// Evidence for EVERY modifiable file (not the 5-capped item.Evidence used for
	// human render), so the spec never commands an edit to a file it justifies with
	// no finding — the agent acts on this verbatim.
	if ev := evidenceFor(fs, modifiable); len(ev) > 0 {
		b.WriteString("\nEvidence (file:line finding):\n")
		for _, e := range ev {
			fmt.Fprintf(&b, "  - %s\n", e)
		}
	}
	fmt.Fprintf(&b, "\nTransformation rule:\n  %s\n", instr)
	b.WriteString("\nConstraints:\n")
	b.WriteString("  - Do not change observable behavior beyond what the transformation requires.\n")
	b.WriteString("  - Do not touch files not listed under \"Files to modify\" above. This is a hard rule — no exceptions.\n")
	b.WriteString("  - Never modify generated or compiled output: dist/, build/, .next/, out/, coverage/, node_modules/, vendor/, target/, __pycache__/.\n")
	b.WriteString("  - Never modify or delete humify's own state directory (.humify/, .humify-dev/, .humify-runs/, .humify-worktrees/) — it holds the analysis, plan, and quarantine records this run depends on.\n")
	b.WriteString("  - Never create planning, notes, or scratch files in the repository.\n")
	b.WriteString("  - If a listed file no longer exists or the finding no longer applies, skip it and say why.\n")
	b.WriteString("  - Do not run builds or test suites. Humify will validate the change after you exit.\n")
	b.WriteString("\nWhen complete, output a short summary of what you changed and why.\n")
	return b.String()
}

// evidenceFor returns "file:line finding" lines for every finding on a modifiable
// file, so the agent spec justifies each file it lists to modify. Unlike the
// 5-capped item.Evidence (for human rendering), this is uncapped — a partial list
// would tell the agent to edit files it has no evidence for.
func evidenceFor(fs []analyze.Finding, modifiable map[string]bool) []string {
	var out []string
	for _, f := range fs {
		if modifiable[f.File] {
			out = append(out, fmt.Sprintf("%s:%d %s", f.File, f.Line, f.Evidence))
		}
	}
	return out
}

// validationFor states how to confirm an item's change did no harm. The right
// signal depends on the change class: a reversible quarantine just re-runs verify;
// a behavior-preserving edit (comment/marker/rename) has no behavior to regress, so
// the honest check is re-analyzing to confirm the finding cleared (verify would
// pass either way and give false confidence); a behavior-changing edit uses the
// baseline-aware two-step so an ambient red is never mistaken for a regression.
func validationFor(d signalDescriptor) string {
	switch {
	case d.applyable:
		return "Re-run `humify verify`; if any previously-passing check fails, restore the quarantined files."
	case d.behaviorPreserving:
		return "Re-run `humify analyze` and confirm this finding has cleared — that is the efficacy signal here. `humify verify` is not: there is no behavior to regress, so it would pass regardless."
	default:
		return "BEFORE editing, capture a baseline: `humify verify --save-baseline`. AFTER editing, compare: `humify verify --baseline` — it separates a regression your change caused from a check that was already failing (ambient)."
	}
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
	maxSev := maxSeverityBySignal(findings)
	sort.SliceStable(items, func(i, j int) bool {
		if oi, oj := order(items[i].Signal), order(items[j].Signal); oi != oj {
			return oi < oj
		}
		// Severity tier dominates volume: a signal carrying any major-severity
		// finding outranks one with only warnings, no matter how many. Without this,
		// a numerous low-severity signal (broad_catch fires on every JS catch — JS
		// has no narrow catch) buries a scarce high-severity one (swallowed_error, a
		// silent-failure bug) on summed weight alone. Volume only breaks ties within
		// a severity tier.
		if si, sj := maxSev[items[i].Signal], maxSev[items[j].Signal]; si != sj {
			return si > sj
		}
		if weight[items[i].Signal] != weight[items[j].Signal] {
			return weight[items[i].Signal] > weight[items[j].Signal]
		}
		return items[i].Title < items[j].Title
	})
}

// maxSeverityBySignal records the highest severity seen per signal (major > warning
// > info), so ranking can let severity dominate raw finding volume.
func maxSeverityBySignal(findings []analyze.Finding) map[string]int {
	rank := func(sev string) int {
		switch sev {
		case "major":
			return 3
		case "warning":
			return 2
		default:
			return 1
		}
	}
	m := map[string]int{}
	for _, f := range findings {
		if r := rank(f.Severity); r > m[f.Signal] {
			m[f.Signal] = r
		}
	}
	return m
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
