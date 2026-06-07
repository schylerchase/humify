// Package textutil holds small string helpers shared across stages. Its reason
// to exist is OneLine: every place that embeds agent- or codebase-derived text
// into a machine-parsed document (AUDIT.md, CONFLICTS.md) or an agent prompt
// must flatten embedded newlines, or a crafted finding/issue could forge an
// extra structural line (a fake "### BLOCKERS (N)" header, a fake plan section).
// One definition keeps that guarantee consistent everywhere.
package textutil

import "strings"

var lineFlattener = strings.NewReplacer("\n", " ", "\r", " ")

// OneLine collapses any embedded newlines/carriage returns to spaces so the
// result can never span more than the line it is rendered on.
func OneLine(s string) string {
	return lineFlattener.Replace(s)
}
