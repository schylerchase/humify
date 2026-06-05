package consolidate

import (
	"fmt"
	"sort"
	"strings"
)

// RenderAudit produces AUDIT.md. Only COVERED areas are listed (so `status`
// flips them to audited); pending areas are intentionally absent and surface in
// CONFLICTS.md instead, so an incomplete consolidation never reads as complete.
func RenderAudit(target string, r Result) string {
	var b strings.Builder
	b.WriteString("# Humify Audit (consolidated)\n\n")
	if target != "" {
		fmt.Fprintf(&b, "Target: `%s`\n\n", target)
	}
	total := len(r.Covered) + len(r.Pending)
	fmt.Fprintf(&b, "Coverage: %d/%d areas consolidated", len(r.Covered), total)
	if len(r.Pending) > 0 {
		fmt.Fprintf(&b, " (%d pending — see CONFLICTS.md)", len(r.Pending))
	}
	b.WriteString(".\n\n## Areas consolidated\n\n")
	for _, a := range r.Covered {
		fmt.Fprintf(&b, "- %s\n", a)
	}
	for _, sev := range []string{"blocker", "warning", "info"} {
		writeFindings(&b, sev, r.Findings)
	}
	return b.String()
}

func writeFindings(b *strings.Builder, sev string, findings []Merged) {
	var rows []string
	for _, m := range findings {
		if m.Severity == sev {
			rows = append(rows, findingRow(m))
		}
	}
	if len(rows) == 0 {
		return
	}
	sort.Strings(rows)
	fmt.Fprintf(b, "\n## %s findings (%d)\n\n", titleCase(sev), len(rows))
	for _, row := range rows {
		b.WriteString(row)
	}
}

func findingRow(m Merged) string {
	loc := m.File
	if m.Line > 0 {
		loc = fmt.Sprintf("%s:%d", m.File, m.Line)
	}
	return fmt.Sprintf("- [%s] %s — %s (source: %s)\n",
		m.Sources[0], loc, m.Title, strings.Join(m.Sources, ", "))
}

// RenderConflicts produces CONFLICTS.md with fixed-format bucket counts so the
// gate is machine-parseable without reading prose.
func RenderConflicts(r Result) string {
	var b strings.Builder
	b.WriteString("## Conflict Detection Report\n")
	writeBucket(&b, "BLOCKERS", "blocker", r.Conflicts)
	writeBucket(&b, "WARNINGS", "warning", r.Conflicts)
	writeBucket(&b, "INFO", "info", r.Conflicts)
	fmt.Fprintf(&b, "\nSTATUS: %s\n", r.Status)
	return b.String()
}

func writeBucket(b *strings.Builder, heading, bucket string, conflicts []Conflict) {
	var rows []string
	for _, c := range conflicts {
		if c.Bucket == bucket {
			rows = append(rows, conflictRow(c))
		}
	}
	sort.Strings(rows)
	fmt.Fprintf(b, "\n### %s (%d)\n\n", heading, len(rows))
	for _, row := range rows {
		b.WriteString(row)
	}
}

func conflictRow(c Conflict) string {
	tag := strings.ToUpper(c.Bucket)
	if len(c.Sources) > 0 {
		return fmt.Sprintf("[%s] %s: %s [%s]\n", tag, c.Kind, c.Detail, strings.Join(c.Sources, ", "))
	}
	return fmt.Sprintf("[%s] %s: %s\n", tag, c.Kind, c.Detail)
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
