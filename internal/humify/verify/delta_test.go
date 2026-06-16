package verify

import "testing"

func cpass(kind string) CmdResult { return CmdResult{Kind: kind, Ran: true, Passed: true} }
func cfail(kind string) CmdResult {
	return CmdResult{Kind: kind, Ran: true, Passed: false, ExitCode: 1}
}
func cindet(kind string) CmdResult {
	return CmdResult{Kind: kind, Ran: true, Passed: false, ExitCode: -1}
}

func val(cmds ...CmdResult) Validation {
	v := Validation{Commands: cmds, Validated: len(cmds) > 0, Passed: true}
	for _, c := range cmds {
		if c.Ran && !c.Passed {
			v.Passed = false
		}
	}
	return v
}

func sameKinds(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestDelta is the full classification table moved verbatim from apply's
// TestComputeDelta — Delta now lives in verify so the read-only verify path can
// reuse the same before/after discriminator the apply gate uses.
func TestDelta(t *testing.T) {
	if a, n, f := Delta(val(cfail("test")), val(cpass("test"))); !sameKinds(f, []string{"test"}) || len(a)+len(n) != 0 {
		t.Errorf("cleanfail→pass should be fixed only; got already=%v newly=%v fixed=%v", a, n, f)
	}
	if a, n, f := Delta(val(cpass("test")), val(cfail("test"))); !sameKinds(n, []string{"test"}) || len(a)+len(f) != 0 {
		t.Errorf("pass→cleanfail should be newly-failing only; got already=%v newly=%v fixed=%v", a, n, f)
	}
	if a, n, f := Delta(val(cfail("test")), val(cfail("test"))); !sameKinds(a, []string{"test"}) || len(n)+len(f) != 0 {
		t.Errorf("cleanfail→cleanfail should be already-failing only; got already=%v newly=%v fixed=%v", a, n, f)
	}
	// The honesty fix: an indeterminate baseline that then passes is NOT "fixed"
	// (it was never known to be failing).
	if a, n, f := Delta(val(cindet("test")), val(cpass("test"))); len(a)+len(n)+len(f) != 0 {
		t.Errorf("indeterminate→pass must classify as nothing; got already=%v newly=%v fixed=%v", a, n, f)
	}
}
