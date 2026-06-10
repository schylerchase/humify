// Package plan drives the plan stage's bounded convergence loop and renders the
// planner and plan-checker prompts. The binary owns the loop's logic and its
// minimal persisted state; an orchestrator only spawns whatever a `humify plan`
// call dispatches, then re-runs the command to advance the loop.
//
// Most of the loop state is DERIVED from disk artifacts each round — does an
// area have a PLAN.md? a PLAN-CHECK.json? how many blocking issues? — in the
// same spirit as `humify status`. The only thing that cannot be derived, and so
// is the only thing persisted, is per-area replan history: how many times an
// area has been re-planned and its prior blocking-issue count, used purely to
// bound the loop (max replans) and detect a stall (issues not decreasing).
package plan

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"humify/internal/layout"
)

// DefaultMaxReplans bounds re-planning per area (the architecture's "max 3").
const DefaultMaxReplans = 3

// Loop round statuses, the single source of truth for Decision.Status.
const (
	StatusDispatch  = "dispatch"  // prompts were written; spawn them and re-run
	StatusConverged = "converged" // every area has an accepted plan
	StatusEscalated = "escalated" // some area exhausted its replan budget or stalled
)

// AreaState is the per-area replan bookkeeping that cannot be derived from disk.
type AreaState struct {
	Replans    int `json:"replans"`
	LastIssues int `json:"last_issues"` // blocking-issue count at the prior replan decision
}

// State is the persisted loop state: the cap plus per-area replan history.
type State struct {
	MaxReplans int                  `json:"max_replans"`
	Areas      map[string]AreaState `json:"areas"`
}

// New returns an empty loop state with the given cap (0 → DefaultMaxReplans).
func New(maxReplans int) State {
	if maxReplans <= 0 {
		maxReplans = DefaultMaxReplans
	}
	return State{MaxReplans: maxReplans, Areas: map[string]AreaState{}}
}

func statePath(root string) string {
	return filepath.Join(layout.TmpDir(root), "plan-loop.json")
}

// Load reads the loop state, returning a fresh New(0) if none exists yet so the
// first `humify plan` call starts cleanly.
func Load(root string) (State, error) {
	b, err := os.ReadFile(statePath(root))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return New(0), nil
		}
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}, err
	}
	if s.Areas == nil {
		s.Areas = map[string]AreaState{}
	}
	if s.MaxReplans <= 0 {
		s.MaxReplans = DefaultMaxReplans
	}
	return s, nil
}

// Save persists the loop state atomically (write-temp-then-rename) so a crash
// mid-write never leaves a half-parsed state file.
func (s State) Save(root string) error {
	if err := os.MkdirAll(layout.TmpDir(root), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	final := statePath(root)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

// Reconcile aligns the persisted state with the current target set: it adds a
// zero entry for any new target and drops state for areas no longer targeted
// (e.g. an area whose findings were all resolved or whose audit changed).
func (s *State) Reconcile(targets []string) {
	want := map[string]bool{}
	for _, id := range targets {
		want[id] = true
		if _, ok := s.Areas[id]; !ok {
			// LastIssues = -1 is a "no prior measurement" sentinel. It is only ever
			// compared once Replans > 0 (the stall check), and the first replan sets
			// a real value before that, so the sentinel is never actually compared —
			// it just documents that no issue count has been recorded yet.
			s.Areas[id] = AreaState{Replans: 0, LastIssues: -1}
		}
	}
	for id := range s.Areas {
		if !want[id] {
			delete(s.Areas, id)
		}
	}
}

// Action is the per-area decision for one loop round.
type Action string

const (
	ActPlan      Action = "plan"      // no PLAN.md yet → dispatch initial planner
	ActCheck     Action = "check"     // PLAN.md present, no verdict → dispatch checker
	ActReplan    Action = "replan"    // failing verdict, budget remains → re-plan with feedback
	ActAccepted  Action = "accepted"  // verdict has zero blocking issues
	ActEscalated Action = "escalated" // failing verdict, budget exhausted or stalled
)

// Obs is one area's disk-derived observation for a round.
type Obs struct {
	AreaID   string
	HasPlan  bool
	HasCheck bool
	Issues   int // blocking issues from the verdict; meaningful only when HasCheck
}

// Verdict pairs an area with its decided action and a human reason.
type Verdict struct {
	AreaID string
	Action Action
	Reason string
}

// Decision is the whole-round outcome the command acts on.
type Decision struct {
	Verdicts   []Verdict
	PlanAreas  []string // dispatch a planner (initial or replan, in id order)
	CheckAreas []string // dispatch a plan-checker
	Replans    []string // subset of PlanAreas that are re-plans (need feedback + stale-artifact cleanup)
	Accepted   []string
	Escalated  []Verdict
	Status     string // "dispatch" | "converged" | "escalated"
}

// Decide computes the round's actions from disk observations, mutating s only to
// record a replan (bump count, remember issue level for stall detection). It is
// otherwise pure: the same (obs, state) always yields the same decision.
//
// Per-area classification, highest-priority first:
//   - no plan          → ActPlan (dispatch initial planner)
//   - plan, no verdict → ActCheck (dispatch checker)
//   - verdict, 0 issues→ ActAccepted (frozen, never re-touched)
//   - verdict, issues  → ActReplan if budget remains and issues are dropping,
//     else ActEscalated (cap reached, or stalled: issues did not decrease)
func Decide(obs []Obs, s *State) Decision {
	sorted := append([]Obs(nil), obs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].AreaID < sorted[j].AreaID })

	var d Decision
	for _, o := range sorted {
		as := s.Areas[o.AreaID]
		switch {
		case !o.HasPlan:
			d.add(o.AreaID, ActPlan, "no plan yet")
			d.PlanAreas = append(d.PlanAreas, o.AreaID)
		case !o.HasCheck:
			d.add(o.AreaID, ActCheck, "plan awaiting check")
			d.CheckAreas = append(d.CheckAreas, o.AreaID)
		case o.Issues == 0:
			d.add(o.AreaID, ActAccepted, "plan accepted (no blocking issues)")
			d.Accepted = append(d.Accepted, o.AreaID)
		case budgetExhausted(as, s.MaxReplans):
			v := verdict(o.AreaID, ActEscalated, "max replans reached")
			d.Verdicts = append(d.Verdicts, v)
			d.Escalated = append(d.Escalated, v)
		case stalled(as, o.Issues):
			v := verdict(o.AreaID, ActEscalated, "stalled: blocking issues not decreasing")
			d.Verdicts = append(d.Verdicts, v)
			d.Escalated = append(d.Escalated, v)
		default:
			as.Replans++
			as.LastIssues = o.Issues
			s.Areas[o.AreaID] = as
			d.add(o.AreaID, ActReplan, "re-planning with checker feedback")
			d.PlanAreas = append(d.PlanAreas, o.AreaID)
			d.Replans = append(d.Replans, o.AreaID)
		}
	}

	switch {
	case len(d.PlanAreas) > 0 || len(d.CheckAreas) > 0:
		d.Status = StatusDispatch
	case len(d.Escalated) > 0:
		d.Status = StatusEscalated
	default:
		d.Status = StatusConverged
	}
	return d
}

func (d *Decision) add(areaID string, a Action, reason string) {
	d.Verdicts = append(d.Verdicts, verdict(areaID, a, reason))
}

func verdict(areaID string, a Action, reason string) Verdict {
	return Verdict{AreaID: areaID, Action: a, Reason: reason}
}

// budgetExhausted reports an area that has used its full replan budget.
func budgetExhausted(as AreaState, maxReplans int) bool {
	return as.Replans >= maxReplans
}

// stalled reports an area whose blocking-issue count failed to decrease across a
// replan — more replanning is unlikely to help.
func stalled(as AreaState, issues int) bool {
	return as.Replans > 0 && issues >= as.LastIssues
}

// Escalated reports whether an area is terminally stuck — budget exhausted OR
// stalled — and should be surfaced for human attention rather than replanned
// again. It is the union of the two ActEscalated branches Decide uses, exposed so
// resume's escalation guard cannot drift from the loop's own decision. Callers
// must only consult it for an area in the escalation-decidable state (a failing
// check present), matching where Decide evaluates it.
func Escalated(as AreaState, maxReplans, issues int) bool {
	return budgetExhausted(as, maxReplans) || stalled(as, issues)
}
