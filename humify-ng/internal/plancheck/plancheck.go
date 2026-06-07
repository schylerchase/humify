// Package plancheck defines the plan-checker's output schema: the adversarial,
// read-only verdict on one area's PLAN.md. Each issue carries a mandatory
// severity; the convergence loop continues while an area has any blocker or
// warning, and stops (accepts the plan) when it has none. Info issues are
// advisory and never block. A check that fails validation is treated as no
// verdict rather than silently trusted — same fail-loud stance as fragments.
package plancheck

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Issue is one problem the checker found with a plan.
type Issue struct {
	Severity string `json:"severity"` // blocker | warning | info
	Title    string `json:"title"`
	Detail   string `json:"detail"`
}

// Check is one area's plan-check verdict.
type Check struct {
	AreaID string  `json:"area_id"`
	Issues []Issue `json:"issues"`
}

// ValidSeverity reports whether s is an accepted severity level.
func ValidSeverity(s string) bool {
	return s == "blocker" || s == "warning" || s == "info"
}

// Load reads and parses a plan-check file.
func Load(path string) (Check, error) {
	var c Check
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parse %s: %w", path, err)
	}
	return c, nil
}

// Validate enforces a non-empty area id and a valid, single-line severity/title
// on every issue (newlines are banned for the same reason fragments ban them:
// they could forge structure in any machine-parsed roll-up of plan checks).
func (c Check) Validate() error {
	if c.AreaID == "" {
		return fmt.Errorf("plan-check has empty area_id")
	}
	for i, is := range c.Issues {
		if !ValidSeverity(is.Severity) {
			return fmt.Errorf("%s issue %d: invalid severity %q", c.AreaID, i, is.Severity)
		}
		if strings.ContainsAny(is.Title, "\n\r") {
			return fmt.Errorf("%s issue %d: title contains a newline", c.AreaID, i)
		}
	}
	return nil
}

// BlockingCount returns the number of issues that hold a plan back from being
// accepted: blockers and warnings. Info issues do not block.
func (c Check) BlockingCount() int {
	n := 0
	for _, is := range c.Issues {
		if is.Severity == "blocker" || is.Severity == "warning" {
			n++
		}
	}
	return n
}
