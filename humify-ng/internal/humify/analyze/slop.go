package analyze

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// inspect runs every detector over one source file and returns its findings.
// Detectors read the SCANNED lines (strings blanked, comments separated), so a
// declaration, catch, or marker that appears inside a string literal is never
// flagged — a slop detector that cries wolf is itself slop. Evidence is taken from
// the raw line so it reads naturally.
func inspect(path, lang string, infos []lineInfo, raw []string, m Metrics, cfg Config) []Finding {
	var out []Finding
	out = append(out, metricFindings(path, m, cfg)...)
	out = append(out, nameFindings(path, lang, infos, raw)...)
	out = append(out, contentFindings(path, lang, infos, raw)...)
	return out
}

// metricFindings flags giant files, long functions, and deep nesting from metrics.
func metricFindings(path string, m Metrics, cfg Config) []Finding {
	var out []Finding
	if m.LOC > cfg.MaxFileLines {
		out = append(out, Finding{
			Category: "maintainability", Signal: "giant_file", File: path, Line: 1,
			Severity: sev(m.LOC, cfg.MaxFileLines, 2), Risk: "medium",
			Evidence: fmt.Sprintf("%d lines (threshold %d)", m.LOC, cfg.MaxFileLines),
			Detail:   "Large files usually mix several responsibilities; split by concern so each unit is independently readable and testable.",
		})
	}
	if m.LongestFunc > cfg.MaxFunctionLines {
		out = append(out, Finding{
			Category: "maintainability", Signal: "long_function", File: path, Line: m.LongestLine,
			Severity: sev(m.LongestFunc, cfg.MaxFunctionLines, 2), Risk: "medium",
			Evidence: fmt.Sprintf("function spans ~%d lines (threshold %d)", m.LongestFunc, cfg.MaxFunctionLines),
			Detail:   "A function this long likely does many unrelated steps; extract cohesive helpers with intention-revealing names.",
		})
	}
	if m.MaxNesting > cfg.MaxNestingDepth {
		out = append(out, Finding{
			Category: "readability", Signal: "deep_nesting", File: path, Line: m.DeepestLine,
			Severity: sev(m.MaxNesting, cfg.MaxNestingDepth, 2), Risk: "low",
			Evidence: fmt.Sprintf("nesting depth %d (threshold %d)", m.MaxNesting, cfg.MaxNestingDepth),
			Detail:   "Deep nesting hides control flow; use early returns or guard clauses to flatten it.",
		})
	}
	return out
}

// vagueNames are identifiers that, used as a declared symbol, signal an unnamed
// responsibility — the classic shape of machine-generated filler.
// "result" and "item" are deliberately absent: in most languages they are
// idiomatic, package-qualified names (scan.Result, plan.Item), not slop.
var vagueNames = map[string]bool{
	"data": true, "thing": true, "things": true,
	"manager": true, "processor": true, "handler": true, "helper": true,
	"helpers": true, "util": true, "utils": true, "foo": true, "bar": true,
	"baz": true, "stuff": true, "misc": true, "dostuff": true, "dosomething": true,
	"obj": true, "temp": true, "tmp": true,
}

var declRe = regexp.MustCompile(`^\s*(?:export\s+)?(?:public\s+|private\s+|protected\s+|static\s+|async\s+)*(?:func|function|def|class|type|interface|struct)\s+([A-Za-z_$][\w$]*)`)

// nameFindings flags vaguely-named declarations. It deliberately does NOT flag
// file names: idiomatic module names (utils, helper, misc) are not slop, and the
// evidence/line for a filename finding is misleading.
func nameFindings(path, lang string, infos []lineInfo, raw []string) []Finding {
	var out []Finding
	for i, in := range infos {
		mtch := declRe.FindStringSubmatch(in.code)
		if mtch == nil || !vagueNames[strings.ToLower(mtch[1])] {
			continue
		}
		out = append(out, Finding{
			Category: "readability", Signal: "vague_name", File: path, Line: i + 1,
			Severity: "info", Risk: "low", Evidence: strings.TrimSpace(raw[i]),
			Detail:   fmt.Sprintf("The name %q does not say what it is or does; rename it for the concept it represents.", mtch[1]),
		})
	}
	return out
}

var (
	// A real leftover marker (TODO/FIXME/XXX/HACK) either leads the comment text
	// or is tag-punctuated — the keyword immediately followed by a colon or open
	// paren. A bare mention of the word mid-sentence is prose, not a marker.
	todoLeadRe    = regexp.MustCompile(`^(TODO|FIXME|XXX|HACK)\b`)
	todoTagRe     = regexp.MustCompile(`\b(TODO|FIXME|XXX|HACK)\s*[:(]`)
	emptyCatchRe  = regexp.MustCompile(`catch\s*(?:\([^)]*\))?\s*\{\s*\}`)
	catchOpenRe   = regexp.MustCompile(`catch\s*(?:\([^)]*\))?\s*\{\s*$`)
	bareExceptRe  = regexp.MustCompile(`^\s*except\s*:\s*$`)
	broadExceptRe = regexp.MustCompile(`^\s*except\s+(?:Exception|BaseException)\b`)
	goErrIfRe     = regexp.MustCompile(`^\s*if\s+err\s*!=\s*nil\s*\{\s*$`)
	identRe       = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
	suppressRe    = regexp.MustCompile(`(?i)\b(?:noqa|nolint|eslint-disable)\b`)
	dividerRunRe  = regexp.MustCompile(`[-=#*~]{3,}`)
)

// contentFindings scans the scanned lines for swallowed errors, broad catches,
// leftover TODO markers (in comments), and comments that merely restate the code.
func contentFindings(path, lang string, infos []lineInfo, raw []string) []Finding {
	code := codeOf(infos)
	clike := family(lang) == "brace"
	var out []Finding
	for i, in := range infos {
		ev := strings.TrimSpace(raw[i])
		if loc := todoMarker(in.comment); loc != "" {
			out = append(out, mark("maintainability", "todo_marker", path, i+1, "info", "low", ev,
				"Leftover "+loc+" marker — resolve it or convert it into a tracked task; unfinished markers often mark machine-stubbed code."))
		}
		if clike {
			emptyCatch := emptyCatchRe.MatchString(in.code) ||
				(catchOpenRe.MatchString(in.code) && nextIsEmptyClose(code, i)) ||
				(lang == "go" && goErrIfRe.MatchString(in.code) && nextIsEmptyClose(code, i))
			if emptyCatch && !intentional(infos, raw, lang, i) {
				out = append(out, swallowed(path, i+1, ev))
			}
		} else {
			// broad == a bare `except:` or `except Exception/BaseException`. A narrow
			// typed except (e.g. json.JSONDecodeError) is deliberate handling, not slop.
			broad := bareExceptRe.MatchString(in.code) || broadExceptRe.MatchString(in.code)
			switch {
			case broad && nextIsPass(code, i):
				// Empty broad catch → swallowed (the more severe); never also broad_catch.
				if !intentional(infos, raw, lang, i) {
					out = append(out, swallowed(path, i+1, ev))
				}
			case broad:
				// Broad catch with a real body → broad_catch (unless explicitly suppressed).
				if !suppressedAt(raw, lang, i) {
					out = append(out, mark("correctness", "broad_catch", path, i+1, "warning", "medium", ev,
						"Catching everything hides the errors you did not anticipate; catch the specific exception you can handle."))
				}
			}
		}
		if strings.TrimSpace(in.code) == "" && in.comment != "" && noisyComment(in.comment, code, i) {
			out = append(out, mark("readability", "noisy_comment", path, i+1, "info", "low", ev,
				"This comment restates the code it precedes; delete it or explain the why, not the what."))
		}
	}
	return out
}

// todoMarker returns the marker keyword (TODO/FIXME/XXX/HACK) when the comment
// carries a real leftover marker — one that leads the comment text (after
// whitespace and any block-comment glyph) or is tag-punctuated (keyword then a
// colon or open paren) — or "" when the keyword only appears in prose. comment
// is the text after the line-comment leader, so it may retain leading whitespace.
func todoMarker(comment string) string {
	body := strings.TrimLeft(comment, " \t*-")
	if m := todoLeadRe.FindStringSubmatch(body); m != nil {
		return m[1]
	}
	if m := todoTagRe.FindStringSubmatch(comment); m != nil {
		return m[1]
	}
	return ""
}

// swallowed builds the standard swallowed-error finding.
func swallowed(path string, line int, evidence string) Finding {
	return mark("correctness", "swallowed_error", path, line, "major", "high", evidence,
		"An error is caught and discarded; handle it, wrap it with context, or at minimum log it — silent failure is the hardest bug to find.")
}

// intentional reports whether the catch/except opened at line i is deliberate: it
// carries an explanatory comment, or a lint-suppression marker. Such a swallow is
// documented intent, not a hidden bug, so Humify does not flag it. For indentation
// languages the doc may sit on the body (`pass`) line, so i+1 is also considered.
func intentional(infos []lineInfo, raw []string, lang string, i int) bool {
	for _, k := range catchLines(lang, i) {
		if k >= 0 && k < len(infos) && strings.TrimSpace(infos[k].comment) != "" {
			return true
		}
	}
	return suppressedAt(raw, lang, i)
}

// suppressedAt reports whether the catch at line i (or its body line, for indent
// languages) carries an explicit lint-suppression marker.
func suppressedAt(raw []string, lang string, i int) bool {
	for _, k := range catchLines(lang, i) {
		if k >= 0 && k < len(raw) && suppressRe.MatchString(raw[k]) {
			return true
		}
	}
	return false
}

// catchLines returns the line indices that may document a catch at line i: just i
// for brace languages, i and i+1 for indentation languages (the `pass` body line).
func catchLines(lang string, i int) []int {
	if family(lang) == "indent" {
		return []int{i, i + 1}
	}
	return []int{i}
}

// mark constructs a Finding with the common fields filled.
func mark(category, signal, path string, line int, severity, risk, evidence, detail string) Finding {
	return Finding{
		Category: category, Signal: signal, File: path, Line: line,
		Severity: severity, Risk: risk, Evidence: clip(evidence, 140), Detail: detail,
	}
}

// nextIsEmptyClose reports whether the block opened on line i is empty — the next
// non-blank code line is a bare "}". It deliberately does NOT match "} else {" or
// "} catch", so a handled error is not mistaken for a swallowed one.
func nextIsEmptyClose(code []string, i int) bool {
	if j := nextNonBlank(code, i+1); j >= 0 {
		return strings.TrimSpace(code[j]) == "}"
	}
	return false
}

// nextIsPass reports whether the next non-blank code line is a lone `pass`.
func nextIsPass(code []string, i int) bool {
	if j := nextNonBlank(code, i+1); j >= 0 {
		return strings.TrimSpace(code[j]) == "pass"
	}
	return false
}

// nextNonBlank returns the index of the next code line with content, or -1.
func nextNonBlank(code []string, from int) int {
	for j := from; j < len(code); j++ {
		if strings.TrimSpace(code[j]) != "" {
			return j
		}
	}
	return -1
}

// noisyComment reports whether a comment is a bare restatement of the next code
// line's identifier. It is deliberately conservative: a real codebase's standalone
// comments are overwhelmingly section labels and dividers (// State, // SQL Servers,
// // ── Group card ──), which are navigation aids, not noise. So it fires ONLY for a
// single, non-capitalized token (e.g. "// getName" over `func getName()`) — skipping
// dividers, multi-word labels, and Title-Case labels — and requires that token to
// equal a whole identifier on the next code line.
func noisyComment(comment string, code []string, i int) bool {
	text := strings.TrimSpace(comment)
	if text == "" || isDivider(text) {
		return false
	}
	if len(strings.Fields(text)) != 1 {
		return false // multi-word comments are labels/prose, not bare restatements
	}
	if r := []rune(text)[0]; unicode.IsUpper(r) {
		return false // a Capitalized label (// State, // Tags) is documentation
	}
	core := reduce(text)
	if len(core) < 4 || len(core) > 40 {
		return false
	}
	j := nextNonBlank(code, i+1)
	if j < 0 {
		return false
	}
	for _, tok := range identRe.FindAllString(code[j], -1) {
		if reduce(tok) == core {
			return true
		}
	}
	return false
}

// isDivider reports whether a comment is a visual section divider (box-drawing
// glyphs, or a run of structural punctuation like --- or ===), never noise.
func isDivider(s string) bool {
	for _, r := range s {
		if r >= 0x2500 && r <= 0x257F { // box-drawing block
			return true
		}
	}
	return dividerRunRe.MatchString(s)
}

// reduce lowercases a string and keeps only [a-z0-9], so "get Name()" and
// "getName" compare equal.
func reduce(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// splitLines splits content into lines, dropping the empty element a trailing
// newline would otherwise add.
func splitLines(content string) []string {
	lines := strings.Split(content, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

// clip truncates s to max runes, adding an ellipsis when it cuts.
func clip(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
