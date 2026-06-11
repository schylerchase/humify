package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/schylerryan/humify/internal/consolidate"
	"github.com/schylerryan/humify/internal/handoff"
	"github.com/schylerryan/humify/internal/layout"
	"github.com/schylerryan/humify/internal/output"
)

// cmdConsolidate runs the fan-in engine: gather fragments -> dedup -> detect
// cycles -> verify completeness -> write AUDIT.md + CONFLICTS.md. Exits 2 if any
// area is pending or any blocker exists (the loop must keep auditing).
func untangleConsolidate(opts options) int {
	root := opts.root
	if root == "" {
		if found, ok := layout.FindRoot(opts.path); ok {
			root = found
		} else {
			root = "."
		}
	}
	res, err := consolidate.Run(root)
	if err != nil {
		return fail(opts, consolidateReason(err), exitError, err.Error())
	}
	if err := writeAudit(root, opts.target, res); err != nil {
		return fail(opts, "write_error", exitError, "write failed: "+err.Error())
	}
	return emitConsolidate(opts, root, res)
}

func consolidateReason(err error) string {
	switch {
	case errors.Is(err, consolidate.ErrNoManifest):
		return "no_manifest"
	case errors.Is(err, consolidate.ErrEmptyManifest):
		return "empty_manifest"
	default:
		return "consolidate_error"
	}
}

func writeAudit(root, target string, res consolidate.Result) error {
	if err := os.WriteFile(layout.AuditFile(root), []byte(consolidate.RenderAudit(target, res)), 0o644); err != nil {
		return err
	}
	return os.WriteFile(layout.ConflictsFile(root), []byte(consolidate.RenderConflicts(res)), 0o644)
}

func emitConsolidate(opts options, root string, res consolidate.Result) int {
	ok := res.Blockers == 0
	reason := "ok"
	switch {
	case len(res.Pending) > 0:
		reason = "audit_incomplete"
	case res.Blockers > 0:
		reason = "blocked"
	case res.Warnings > 0:
		reason = "needs_decision"
	}
	code := exitOK
	if !ok {
		code = exitDrift
	}
	if ok {
		saveHandoff(root, handoff.Handoff{Stage: "consolidate", Action: "proceed",
			NextCommand: "humify plan", Note: "AUDIT.md ready — plan the findings"})
	} else {
		saveHandoff(root, handoff.Handoff{Stage: "consolidate", Action: "blocked",
			NextCommand: "humify audit", Note: "pending/blocked fragments — re-audit the affected areas"})
	}
	if opts.json {
		output.EmitJSON(os.Stdout, output.Result{Ok: ok, ReasonCode: reason, Data: res})
		return code
	}
	fmt.Printf("consolidated %d areas (%d pending) -> %s\n",
		len(res.Covered), len(res.Pending), layout.AuditFile(root))
	fmt.Printf("conflicts: %d blockers, %d warnings, %d info | STATUS: %s\n",
		res.Blockers, res.Warnings, res.Infos, res.Status)
	if len(res.Pending) > 0 {
		fmt.Printf("PENDING (%d): %s — fragments missing; these areas stay audit-incomplete\n",
			len(res.Pending), strings.Join(res.Pending, ", "))
	}
	return code
}
