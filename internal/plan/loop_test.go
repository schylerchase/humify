package plan

import (
	"testing"
)

func actionOf(d Decision, areaID string) Action {
	for _, v := range d.Verdicts {
		if v.AreaID == areaID {
			return v.Action
		}
	}
	return ""
}

func TestDecideInitialPlan(t *testing.T) {
	s := New(0)
	s.Reconcile([]string{"01-a", "02-b"})
	d := Decide([]Obs{{AreaID: "01-a"}, {AreaID: "02-b"}}, &s)
	if d.Status != "dispatch" || len(d.PlanAreas) != 2 || len(d.CheckAreas) != 0 {
		t.Fatalf("want 2 plan dispatches, got %+v", d)
	}
	if actionOf(d, "01-a") != ActPlan {
		t.Fatalf("01-a action = %q, want plan", actionOf(d, "01-a"))
	}
}

func TestDecideNeedsCheck(t *testing.T) {
	s := New(0)
	s.Reconcile([]string{"01-a"})
	d := Decide([]Obs{{AreaID: "01-a", HasPlan: true}}, &s)
	if d.Status != "dispatch" || len(d.CheckAreas) != 1 || actionOf(d, "01-a") != ActCheck {
		t.Fatalf("want check dispatch, got %+v", d)
	}
}

func TestDecideConvergedWhenAllAccepted(t *testing.T) {
	s := New(0)
	s.Reconcile([]string{"01-a", "02-b"})
	d := Decide([]Obs{
		{AreaID: "01-a", HasPlan: true, HasCheck: true, Issues: 0},
		{AreaID: "02-b", HasPlan: true, HasCheck: true, Issues: 0},
	}, &s)
	if d.Status != "converged" || len(d.Accepted) != 2 {
		t.Fatalf("want converged with 2 accepted, got %+v", d)
	}
}

// A failing verdict with budget left re-plans and records the issue level; the
// same count next round is a stall and escalates; a lower count keeps going.
func TestDecideReplanStallAndProgress(t *testing.T) {
	s := New(3)
	s.Reconcile([]string{"01-a"})

	// First failing check: 2 issues, no prior replan → re-plan, bump to 1.
	d := Decide([]Obs{{AreaID: "01-a", HasPlan: true, HasCheck: true, Issues: 2}}, &s)
	if actionOf(d, "01-a") != ActReplan {
		t.Fatalf("first failing check should replan, got %q", actionOf(d, "01-a"))
	}
	if s.Areas["01-a"].Replans != 1 || s.Areas["01-a"].LastIssues != 2 {
		t.Fatalf("state after replan = %+v, want {1,2}", s.Areas["01-a"])
	}

	// Same 2 issues again → no improvement → stall → escalate (no further bump).
	d = Decide([]Obs{{AreaID: "01-a", HasPlan: true, HasCheck: true, Issues: 2}}, &s)
	if actionOf(d, "01-a") != ActEscalated || d.Status != "escalated" {
		t.Fatalf("stall should escalate, got action=%q status=%q", actionOf(d, "01-a"), d.Status)
	}
	if s.Areas["01-a"].Replans != 1 {
		t.Fatalf("escalation must not bump replans, got %d", s.Areas["01-a"].Replans)
	}

	// Reset to a progressing trajectory: fewer issues than last → re-plan again.
	s2 := New(3)
	s2.Areas["01-a"] = AreaState{Replans: 1, LastIssues: 2}
	d = Decide([]Obs{{AreaID: "01-a", HasPlan: true, HasCheck: true, Issues: 1}}, &s2)
	if actionOf(d, "01-a") != ActReplan || s2.Areas["01-a"].Replans != 2 {
		t.Fatalf("improving issues should replan and bump, got action=%q state=%+v", actionOf(d, "01-a"), s2.Areas["01-a"])
	}
}

func TestDecideMaxReplansEscalates(t *testing.T) {
	s := New(2)
	s.Areas["01-a"] = AreaState{Replans: 2, LastIssues: 1} // already at cap
	d := Decide([]Obs{{AreaID: "01-a", HasPlan: true, HasCheck: true, Issues: 1}}, &s)
	if actionOf(d, "01-a") != ActEscalated {
		t.Fatalf("at cap should escalate, got %q", actionOf(d, "01-a"))
	}
}

// Work still pending on one area keeps the round dispatchable even if another
// area has escalated; only when nothing is dispatchable does status flip.
func TestDecideDispatchTakesPriorityOverEscalated(t *testing.T) {
	s := New(1)
	s.Areas["01-a"] = AreaState{Replans: 1, LastIssues: 1} // will escalate
	s.Areas["02-b"] = AreaState{}                          // brand new, needs plan
	d := Decide([]Obs{
		{AreaID: "01-a", HasPlan: true, HasCheck: true, Issues: 1},
		{AreaID: "02-b"},
	}, &s)
	if d.Status != "dispatch" {
		t.Fatalf("status = %q, want dispatch (02-b still needs a plan)", d.Status)
	}
	if len(d.Escalated) != 1 || d.Escalated[0].AreaID != "01-a" {
		t.Fatalf("01-a should be recorded escalated, got %+v", d.Escalated)
	}
}

func TestDecideEscalatedWhenNoWorkLeft(t *testing.T) {
	s := New(1)
	s.Areas["01-a"] = AreaState{Replans: 1, LastIssues: 1}
	s.Areas["02-b"] = AreaState{}
	d := Decide([]Obs{
		{AreaID: "01-a", HasPlan: true, HasCheck: true, Issues: 1}, // escalates
		{AreaID: "02-b", HasPlan: true, HasCheck: true, Issues: 0}, // accepted
	}, &s)
	if d.Status != "escalated" {
		t.Fatalf("status = %q, want escalated (nothing dispatchable, one escalated)", d.Status)
	}
}

func TestReconcileAddsAndDrops(t *testing.T) {
	s := New(0)
	s.Areas["old"] = AreaState{Replans: 2}
	s.Reconcile([]string{"01-a", "02-b"})
	if _, ok := s.Areas["old"]; ok {
		t.Fatal("reconcile should drop areas no longer targeted")
	}
	if _, ok := s.Areas["01-a"]; !ok {
		t.Fatal("reconcile should add new targets")
	}
	if s.Areas["01-a"].LastIssues != -1 {
		t.Fatalf("new area LastIssues = %d, want -1", s.Areas["01-a"].LastIssues)
	}
}

func TestStateSaveLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	// Load with no file → fresh state.
	s, err := Load(root)
	if err != nil || s.MaxReplans != DefaultMaxReplans || len(s.Areas) != 0 {
		t.Fatalf("fresh load = %+v err=%v", s, err)
	}
	s.MaxReplans = 5
	s.Areas["01-a"] = AreaState{Replans: 2, LastIssues: 3}
	if err := s.Save(root); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.MaxReplans != 5 || got.Areas["01-a"] != (AreaState{Replans: 2, LastIssues: 3}) {
		t.Fatalf("round-trip = %+v", got)
	}
}
