// Package output renders command results as either a structured
// {ok, reason_code, data} JSON envelope (for agents/automation) or a
// human-readable table. Every command speaks both; the binary never
// requires prose parsing to learn an outcome.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"humify/internal/state"
)

// Result is the structured envelope emitted under --json.
type Result struct {
	Ok         bool   `json:"ok"`
	ReasonCode string `json:"reason_code"`
	Data       any    `json:"data,omitempty"`
}

// StatusData is the payload of `humify status`.
type StatusData struct {
	Root     string       `json:"root"`
	Total    int          `json:"total_areas"`
	Patched  int          `json:"patched"`
	Drifted  int          `json:"audit_incomplete"`
	Progress string       `json:"progress"`
	Areas    []state.Area `json:"areas"`
}

// EmitJSON writes an indented JSON envelope.
func EmitJSON(w io.Writer, r Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// EmitStatusTable writes the human-readable status view.
func EmitStatusTable(w io.Writer, d StatusData) {
	fmt.Fprintf(w, "humify project: %s\n\n", d.Root)
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "AREA\tSTATUS\tFRAGS\tPLANS\tSUMMARIES")
	for _, a := range d.Areas {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\n",
			a.ID, a.Status, a.Fragments, a.Plans, a.Summaries)
	}
	tw.Flush()
	fmt.Fprintf(w, "\nprogress: %s patched (%d/%d areas)\n", d.Progress, d.Patched, d.Total)
	if d.Drifted > 0 {
		fmt.Fprintf(w, "DRIFT: %d area(s) audit-incomplete — fragments exist but "+
			"AUDIT.md never gathered them. Run `humify audit` to consolidate.\n", d.Drifted)
	}
}
