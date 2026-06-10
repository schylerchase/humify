package plancheck

import "testing"

func TestValidateRejectsBadSeverityAndNewline(t *testing.T) {
	cases := []struct {
		name string
		c    Check
		ok   bool
	}{
		{"valid", Check{AreaID: "01-a", Issues: []Issue{{Severity: "blocker", Title: "t"}}}, true},
		{"empty area", Check{Issues: []Issue{{Severity: "info", Title: "t"}}}, false},
		{"bad severity", Check{AreaID: "01-a", Issues: []Issue{{Severity: "fatal", Title: "t"}}}, false},
		{"newline title", Check{AreaID: "01-a", Issues: []Issue{{Severity: "info", Title: "a\nb"}}}, false},
		{"no issues is valid", Check{AreaID: "01-a"}, true},
	}
	for _, c := range cases {
		err := c.c.Validate()
		if (err == nil) != c.ok {
			t.Errorf("%s: Validate() err=%v, want ok=%v", c.name, err, c.ok)
		}
	}
}

// BlockingCount counts blockers and warnings; info issues never block a plan.
func TestBlockingCountIgnoresInfo(t *testing.T) {
	c := Check{AreaID: "01-a", Issues: []Issue{
		{Severity: "blocker", Title: "a"},
		{Severity: "warning", Title: "b"},
		{Severity: "info", Title: "c"},
		{Severity: "info", Title: "d"},
	}}
	if got := c.BlockingCount(); got != 2 {
		t.Fatalf("BlockingCount = %d, want 2 (blocker+warning only)", got)
	}
}
