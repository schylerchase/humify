package exec

import (
	"encoding/json"
	"fmt"
	"os"
)

// Verify is the verifier's verdict on one merged slice: an adversarial
// filesystem fact-check and behaviour-preservation judgement. Passed=false with
// a non-empty Failed list means the merged change does not do what its SUMMARY
// claims, or changed behaviour it should have preserved.
type Verify struct {
	AreaID        string   `json:"area_id"`
	ClaimsChecked int      `json:"claims_checked"`
	Passed        bool     `json:"passed"`
	Failed        []string `json:"failed"`
}

// LoadVerify reads and parses a verifier verdict file.
func LoadVerify(path string) (Verify, error) {
	var v Verify
	b, err := os.ReadFile(path)
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return v, fmt.Errorf("parse %s: %w", path, err)
	}
	return v, nil
}

// Validate enforces a non-empty area id and internal consistency: a failed
// verdict must list at least one failure, and a passed verdict must list none.
func (v Verify) Validate() error {
	if v.AreaID == "" {
		return fmt.Errorf("verify has empty area_id")
	}
	if !v.Passed && len(v.Failed) == 0 {
		return fmt.Errorf("%s: failed verdict must list at least one failure", v.AreaID)
	}
	if v.Passed && len(v.Failed) > 0 {
		return fmt.Errorf("%s: passed verdict must not list failures", v.AreaID)
	}
	return nil
}
