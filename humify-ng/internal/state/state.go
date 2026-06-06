// Package state derives an area's pipeline status purely from on-disk
// artifacts. Nothing is persisted as authoritative status — the file
// topology IS the state, recomputed on every read. This is a port of the
// GSD roadmap.cjs disk-status cascade, with one added terminal-of-concern:
// AuditIncomplete, which fires when audit fragments exist but the
// consolidated AUDIT.md never gathered them (the failure mode that stranded
// 25 fragments on the azure_mapper run).
package state

import (
	"os"
	"strings"
)

// Status is an area's highest reached pipeline stage.
type Status string

const (
	NoDirectory     Status = "no_directory"
	Empty           Status = "empty"
	Mapped          Status = "mapped"
	AuditIncomplete Status = "audit-incomplete"
	Audited         Status = "audited"
	Planned         Status = "planned"
	Executed        Status = "executed"
	Patched         Status = "patched"
)

// Area is the derived state of one .humify/areas/<id> directory.
type Area struct {
	ID        string `json:"id"`
	Status    Status `json:"status"`
	Fragments int    `json:"fragments"`
	Plans     int    `json:"plans"`
	Summaries int    `json:"summaries"`
	HasMap    bool   `json:"has_map"`
}

type counts struct {
	hasMap    bool
	fragments int
	plans     int
	summaries int
}

// Derive computes an area's status from its directory contents and its
// membership in the consolidated AUDIT.md / PATCHLOG.md documents (passed in
// as already-read text so callers read each consolidated doc only once).
func Derive(areaDir, areaID, auditDoc, patchlogDoc string) Area {
	if !isDir(areaDir) {
		return Area{ID: areaID, Status: NoDirectory}
	}
	c := scan(areaDir)
	return Area{
		ID:        areaID,
		Status:    classify(c, areaID, auditDoc, patchlogDoc),
		Fragments: c.fragments,
		Plans:     c.plans,
		Summaries: c.summaries,
		HasMap:    c.hasMap,
	}
}

// classify is the highest-stage-wins cascade. Later stages imply earlier
// ones, so the first matching branch from the top is the answer.
func classify(c counts, areaID, auditDoc, patchlogDoc string) Status {
	switch {
	case covers(patchlogDoc, areaID):
		return Patched
	case c.plans > 0 && c.summaries >= c.plans:
		return Executed
	case c.plans > 0:
		return Planned
	case c.fragments > 0 && covers(auditDoc, areaID):
		return Audited
	case c.fragments > 0:
		return AuditIncomplete // fragments on disk, but AUDIT.md never gathered them
	case c.hasMap:
		return Mapped
	default:
		return Empty
	}
}

func scan(areaDir string) counts {
	var c counts
	entries, err := os.ReadDir(areaDir)
	if err != nil {
		return c
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		classifyFile(e.Name(), &c)
	}
	return c
}

func classifyFile(name string, c *counts) {
	switch {
	case strings.HasSuffix(name, "-MAP.md"):
		c.hasMap = true
	case strings.Contains(name, "AUDIT-fragment") && strings.HasSuffix(name, ".json"):
		c.fragments++
	case strings.HasSuffix(name, "-PLAN.md"):
		c.plans++
	case strings.HasSuffix(name, "-SUMMARY.md"):
		c.summaries++
	}
}

// covers reports whether a consolidated doc DECLARES coverage of areaID — a
// list item or heading whose sole content is the id (e.g. "- 01-core" or
// "## 01-core"). It deliberately does NOT match an id embedded in prose such as
// a finding's file path ("01-core/handler.go") or title. Without this anchor a
// COVERED area's finding text could name a PENDING area's id and falsely flip
// that pending area to audited — a fail-open that defeats drift detection.
func covers(doc, areaID string) bool {
	if doc == "" || areaID == "" {
		return false
	}
	for _, line := range strings.Split(doc, "\n") {
		if lineNamesArea(line, areaID) {
			return true
		}
	}
	return false
}

func lineNamesArea(line, areaID string) bool {
	s := strings.TrimSpace(line)
	for _, marker := range []string{"- ", "* ", "## ", "# "} {
		s = strings.TrimPrefix(s, marker)
	}
	s = strings.TrimSpace(s)
	// Area ids never contain spaces, so a structural heading the renderer wrote
	// ("## Areas consolidated") can never be mistaken for a coverage declaration.
	return s == areaID && !strings.ContainsRune(s, ' ')
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
