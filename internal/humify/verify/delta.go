package verify

// Delta classifies validation kinds across a baseline→post comparison into
// already-failing (cleanly failed before and after), newly-failing (a
// regression — passed before, cleanly failed after), and fixed (cleanly failed
// before, passed after). "Failed" means a clean fail (ExitCode >= 0); an
// indeterminate result (timeout / launch error, ExitCode < 0) is deliberately
// NOT counted as a failure, so a baseline that merely timed out is never
// reported as "already failing" — callers that care about indeterminate kinds
// must inspect post.Commands directly.
//
// It is the shared discriminator for both apply's success-path summary and the
// read-only `verify --baseline` path: ambient failures and real regressions are
// both clean non-zero exits, and only comparing the same checks before and after
// tells them apart.
func Delta(baseline, post Validation) (alreadyFailing, newlyFailing, fixed []string) {
	basePassed := map[string]bool{}
	baseCleanFail := map[string]bool{}
	for _, c := range baseline.Commands {
		if !c.Ran {
			continue
		}
		switch {
		case c.Passed:
			basePassed[c.Kind] = true
		case c.ExitCode >= 0:
			baseCleanFail[c.Kind] = true
		}
	}
	for _, c := range post.Commands {
		if !c.Ran {
			continue
		}
		switch {
		case c.Passed:
			if baseCleanFail[c.Kind] {
				fixed = append(fixed, c.Kind)
			}
		case c.ExitCode >= 0: // clean fail
			if basePassed[c.Kind] {
				newlyFailing = append(newlyFailing, c.Kind)
			} else if baseCleanFail[c.Kind] {
				alreadyFailing = append(alreadyFailing, c.Kind)
			}
		}
	}
	return alreadyFailing, newlyFailing, fixed
}
