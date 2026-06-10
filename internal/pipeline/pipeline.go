// Package pipeline is the read-only stage reducer. It answers two questions for
// the resilience surface — "what is the next step?" (resume) and "is stage X
// complete?" (verify) — purely by composing the existing per-stage predicates
// over the on-disk .humify/ topology. It is the single place the whole-lifecycle
// ordering lives, and it mutates nothing: every input is derived, never stored,
// so a context reset loses no progress (the project's defining property).
package pipeline

import (
	"fmt"
	"os"
	"path/filepath"

	"humify/internal/audit"
	"humify/internal/consolidate"
	"humify/internal/exec"
	"humify/internal/intel"
	"humify/internal/layout"
	"humify/internal/plan"
	"humify/internal/state"
)

// Stage is one lifecycle stage (or the terminal "done").
type Stage string

const (
	StageHeatmap     Stage = "heatmap"
	StageAudit       Stage = "audit"
	StageConsolidate Stage = "consolidate"
	StagePlan        Stage = "plan"
	StageExecute     Stage = "execute"
	StagePatchlog    Stage = "patchlog"
	StageDone        Stage = "done"
)

// Order is the lifecycle sequence verify walks when given no specific stage.
var Order = []Stage{StageHeatmap, StageAudit, StageConsolidate, StagePlan, StageExecute, StagePatchlog}

// Step is resume's answer: the first incomplete stage and how to advance it.
type Step struct {
	Stage       Stage  `json:"stage"`
	NextCommand string `json:"next_command"`
	Reason      string `json:"reason"`  // machine code: needs_bootstrap|intel_drift|audit_pending|audit_incomplete|blocked|plan_pending|execute_pending|patchlog_pending|complete
	Blocked     bool   `json:"blocked"` // a gate failed — needs a human, not just "run the next command"
	Detail      string `json:"detail"`
}

// StageResult is verify's answer for one stage.
type StageResult struct {
	Stage  Stage  `json:"stage"`
	Pass   bool   `json:"pass"`
	Reason string `json:"reason"`
	Detail string `json:"detail"`
}

// snap is the whole lifecycle observed from disk in one pass, so Next and Check
// share identical inputs and can never disagree about where the project stands.
type snap struct {
	bootstrapped    bool
	missing         int // manifest areas absent from intel (corruption)
	auditPending    int // areas still needing an auditor
	auditIncomplete int  // fragments on disk not gathered into AUDIT.md
	conErr          bool // consolidate.Run failed (e.g. empty/unreadable manifest)
	blockers        int  // consolidation blockers
	planPending     int  // finding-bearing areas without an accepted plan
	execDone        bool
	unpatched       int // executed areas not yet in PATCHLOG.md
}

// observe reads the full lifecycle state for root. Everything is best-effort and
// read-only: a not-yet-bootstrapped project simply leaves later fields zero.
func observe(root string) snap {
	var s snap
	if _, err := intel.Load(root); err != nil {
		return s // no intel → not bootstrapped
	}
	ap, err := audit.BuildPlan(root)
	if err != nil {
		return s // no manifest → not bootstrapped
	}
	s.bootstrapped = true
	s.missing = len(ap.Missing)
	s.auditPending = len(ap.Pending)

	auditDoc := readFile(layout.AuditFile(root))
	patchDoc := readFile(layout.PatchlogFile(root))
	ids, _ := layout.DiscoverAreas(root)
	for _, id := range ids {
		a := state.Derive(filepath.Join(layout.AreasDir(root), id), id, auditDoc, patchDoc)
		switch a.Status {
		case state.AuditIncomplete:
			s.auditIncomplete++
		case state.Executed:
			s.unpatched++
		}
	}

	// A consolidate.Run error in a bootstrapped project (e.g. ErrEmptyManifest)
	// must NOT be swallowed: leaving the counters at zero would let Next fall all
	// the way through to "done". Record it as a blocking condition instead.
	if con, cerr := consolidate.Run(root); cerr != nil {
		s.conErr = true
	} else {
		s.blockers = con.Blockers
		for _, o := range plan.Observe(root, consolidate.FindingAreas(con)) {
			if !o.Accepted() {
				s.planPending++
			}
		}
	}

	in, _ := intel.Load(root)
	planned, executed := exec.ScanPlanState(root)
	_, _, s.execDone = exec.CurrentWave(in.Waves, planned, executed)
	return s
}

// Next returns the first incomplete stage in lifecycle order, with the command
// that advances it. Disk is authoritative; resume reconciles any HANDOFF cursor
// against this, never the reverse.
func Next(root string) Step {
	s := observe(root)
	switch {
	case !s.bootstrapped:
		return Step{StageHeatmap, "humify heatmap --target=<dir>", "needs_bootstrap", false,
			"no .humify/ project here — run heatmap on a target codebase"}
	case s.missing > 0:
		return Step{StageHeatmap, "humify heatmap", "intel_drift", true,
			fmt.Sprintf("%d manifest area(s) absent from intel — re-run heatmap", s.missing)}
	case s.auditPending > 0:
		return Step{StageAudit, "humify audit", "audit_pending", false,
			fmt.Sprintf("%d area(s) still need an auditor", s.auditPending)}
	case s.auditIncomplete > 0:
		return Step{StageConsolidate, "humify consolidate", "audit_incomplete", false,
			fmt.Sprintf("%d area(s) have fragments not yet gathered into AUDIT.md", s.auditIncomplete)}
	case s.conErr:
		return Step{StageConsolidate, "humify heatmap", "blocked", true,
			"AUDIT_MANIFEST is empty or unreadable — re-run heatmap"}
	case s.blockers > 0:
		return Step{StageConsolidate, "humify audit", "blocked", true,
			fmt.Sprintf("%d consolidation blocker(s) — re-audit the affected area(s)", s.blockers)}
	case s.planPending > 0:
		return Step{StagePlan, "humify plan", "plan_pending", false,
			fmt.Sprintf("%d area(s) without an accepted plan", s.planPending)}
	case !s.execDone:
		return Step{StageExecute, "humify execute", "execute_pending", false,
			"planned slices not yet executed"}
	case s.unpatched > 0:
		return Step{StagePatchlog, "humify patchlog", "patchlog_pending", false,
			fmt.Sprintf("%d executed area(s) not yet in PATCHLOG.md", s.unpatched)}
	default:
		return Step{StageDone, "", "complete", false, "pipeline complete — nothing to do"}
	}
}

// Check reports whether one stage's deterministic gate is satisfied. It reads the
// same snapshot Next does, so a "pass" here means exactly that Next would not stop
// at this stage.
func Check(root string, st Stage) StageResult {
	s := observe(root)
	switch st {
	case StageHeatmap:
		return result(st, s.bootstrapped && s.missing == 0, bootstrapDetail(s))
	case StageAudit:
		return result(st, s.bootstrapped && s.missing == 0 && s.auditPending == 0,
			fmt.Sprintf("%d area(s) pending an auditor", s.auditPending))
	case StageConsolidate:
		return result(st, s.bootstrapped && !s.conErr && s.auditIncomplete == 0 && s.blockers == 0,
			consolidateDetail(s))
	case StagePlan:
		return result(st, s.bootstrapped && !s.conErr && s.planPending == 0,
			fmt.Sprintf("%d area(s) without an accepted plan", s.planPending))
	case StageExecute:
		return result(st, s.bootstrapped && s.execDone, "planned slices still pending execution")
	case StagePatchlog:
		return result(st, s.bootstrapped && s.unpatched == 0,
			fmt.Sprintf("%d executed area(s) not in PATCHLOG.md", s.unpatched))
	default:
		return StageResult{Stage: st, Pass: false, Reason: "unknown_stage", Detail: "unknown stage " + string(st)}
	}
}

func result(st Stage, pass bool, failDetail string) StageResult {
	r := StageResult{Stage: st, Pass: pass, Reason: "complete", Detail: "complete"}
	if !pass {
		r.Reason, r.Detail = "incomplete", failDetail
	}
	return r
}

func bootstrapDetail(s snap) string {
	if !s.bootstrapped {
		return "not bootstrapped — run heatmap"
	}
	return fmt.Sprintf("%d manifest area(s) absent from intel", s.missing)
}

func consolidateDetail(s snap) string {
	switch {
	case !s.bootstrapped:
		return "not bootstrapped — run heatmap"
	case s.conErr:
		return "AUDIT_MANIFEST empty or unreadable — re-run heatmap"
	default:
		return fmt.Sprintf("%d not gathered, %d blocker(s)", s.auditIncomplete, s.blockers)
	}
}

// readFile is a best-effort read: a missing consolidated doc reads as empty,
// which the state cascade correctly treats as "not covered".
func readFile(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}
