package plan

import "testing"

// Escalated must fire on BOTH terminal conditions Decide recognizes. The stall arm
// is the one resume's guard previously missed — an area can stall (issues not
// decreasing) while still under its replan cap, and must still be surfaced.
func TestEscalated(t *testing.T) {
	const max = 3
	cases := []struct {
		name   string
		as     AreaState
		issues int
		want   bool
	}{
		{"budget exhausted", AreaState{Replans: 3, LastIssues: 2}, 5, true},
		{"stalled below cap (issues not decreasing)", AreaState{Replans: 1, LastIssues: 5}, 5, true},
		{"stalled below cap (issues increasing)", AreaState{Replans: 2, LastIssues: 4}, 6, true},
		{"progressing with budget left", AreaState{Replans: 1, LastIssues: 5}, 3, false},
		{"first round, no prior replan", AreaState{Replans: 0, LastIssues: -1}, 5, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Escalated(tc.as, max, tc.issues); got != tc.want {
				t.Fatalf("Escalated(%+v, %d, %d) = %v, want %v", tc.as, max, tc.issues, got, tc.want)
			}
		})
	}
}
