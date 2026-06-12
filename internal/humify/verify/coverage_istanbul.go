package verify

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// istanbulFile is the subset of an Istanbul coverage-final.json entry we read
// (c8/nyc --reporter=json output).
type istanbulFile struct {
	Path         string `json:"path"`
	StatementMap map[string]struct {
		Start struct {
			Line int `json:"line"`
		} `json:"start"`
	} `json:"statementMap"`
	S map[string]int `json:"s"` // statement id -> hit count
}

// parseIstanbul turns a coverage-final.json body into per-file coverage keyed by
// repo-relative path (root stripped). A file is Covered iff any statement hit > 0.
// Entries outside root (dependencies) are ignored.
func parseIstanbul(data []byte, root string) (map[string]FileCoverage, error) {
	var raw map[string]istanbulFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	files := map[string]FileCoverage{}
	for _, f := range raw {
		rel := f.Path
		if r, err := filepath.Rel(root, f.Path); err == nil {
			rel = filepath.ToSlash(r)
		}
		if strings.HasPrefix(rel, "..") {
			continue // outside the repo (a dependency); ignore
		}
		var fc FileCoverage
		for id, hits := range f.S {
			if hits > 0 {
				fc.Covered = true
				if stmt, ok := f.StatementMap[id]; ok {
					fc.Lines = append(fc.Lines, stmt.Start.Line)
				}
			}
		}
		files[rel] = fc
	}
	return files, nil
}
