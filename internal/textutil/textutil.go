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

// ToForwardSlash replaces every backslash with a forward slash, unconditionally
// and independent of the host OS. Unlike filepath.ToSlash — which rewrites only
// the OS-native separator and is therefore a no-op on Unix — this normalizes a
// Windows-style path such as `C:\src` to `C:/src` even when running on
// macOS/Linux. Use it for display paths embedded in prompts: those artifacts
// must render identically on every host, so the normalization cannot depend on
// the separator of the machine the binary happens to run on.
func ToForwardSlash(s string) string {
	return strings.ReplaceAll(s, "\\", "/")
}
