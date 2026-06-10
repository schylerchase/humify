// Package fragment defines the audit-fragment schema produced (later) by one
// auditor agent per area, plus loading and validation. A fragment that fails
// validation is rejected loudly rather than silently consolidated — the
// auditor contract requires a valid severity on every finding.
package fragment

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Finding is one audit observation about a specific location.
type Finding struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Severity string   `json:"severity"` // blocker | warning | info
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Detail   string   `json:"detail"`
	Refs     []string `json:"refs"` // other area ids this finding cross-references
}

// Fragment is one area's audit output.
type Fragment struct {
	AreaID   string    `json:"area_id"`
	Findings []Finding `json:"findings"`
}

// ValidSeverity reports whether s is an accepted severity level.
func ValidSeverity(s string) bool {
	return s == "blocker" || s == "warning" || s == "info"
}

// SeverityRank orders severities so the strongest can win a merge.
func SeverityRank(s string) int {
	switch s {
	case "blocker":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

// Load reads and parses a fragment file.
func Load(path string) (Fragment, error) {
	var f Fragment
	b, err := os.ReadFile(path)
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return f, fmt.Errorf("parse %s: %w", path, err)
	}
	return f, nil
}

// Validate enforces the mandatory-severity contract: a non-empty area id and,
// for every finding, a title and a valid severity. The first violation is
// returned so a broken fragment is rejected, never silently merged.
func (f Fragment) Validate() error {
	if f.AreaID == "" {
		return fmt.Errorf("fragment has empty area_id")
	}
	for i, fd := range f.Findings {
		if fd.Title == "" {
			return fmt.Errorf("%s finding %d: empty title", f.AreaID, i)
		}
		if !ValidSeverity(fd.Severity) {
			return fmt.Errorf("%s finding %q: invalid severity %q", f.AreaID, fd.Title, fd.Severity)
		}
		// Newlines in auditor-controlled text could forge headers/rows in the
		// machine-parseable AUDIT.md / CONFLICTS.md output. Reject at the input.
		if strings.ContainsAny(fd.Title, "\n\r") || strings.ContainsAny(fd.File, "\n\r") {
			return fmt.Errorf("%s finding %q: title/file contains a newline", f.AreaID, strings.TrimSpace(fd.Title))
		}
	}
	return nil
}
