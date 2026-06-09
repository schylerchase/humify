package analyze

import (
	"regexp"
	"strings"
)

// Metrics are the structural facts measured from one source file's text. They are
// heuristic and language-shallow — enough to flag giant functions, deep nesting,
// and comment density without parsing every language — but they are computed over
// code with strings and comments removed, so braces and indentation inside string
// literals or comments never skew them.
type Metrics struct {
	LOC          int     `json:"loc"`
	Code         int     `json:"code"`
	Comment      int     `json:"comment"`
	Blank        int     `json:"blank"`
	CommentRatio float64 `json:"comment_ratio"`
	LongestFunc  int     `json:"longest_func"` // lines in the longest function-like body
	LongestLine  int     `json:"longest_line"` // 1-based start line of that body
	MaxNesting   int     `json:"max_nesting"`  // deepest block nesting
	DeepestLine  int     `json:"deepest_line"` // 1-based line at the deepest nesting
}

// family groups languages by how their structure is measured.
func family(lang string) string {
	switch lang {
	case "py", "rb":
		return "indent"
	default:
		return "brace" // go, js, ts, java, rs, c, cpp, cs, php, sh, ps1, ...
	}
}

// lineCommentToken returns the line-comment marker for a language.
func lineCommentToken(lang string) string {
	switch lang {
	case "py", "rb", "sh", "ps1":
		return "#"
	default:
		return "//"
	}
}

// lineInfo holds, per source line, its code (with string literals blanked and
// comments removed) and any comment text on the line. A small stateful scanner
// produces these so multi-line strings and block comments are handled correctly.
type lineInfo struct {
	code    string
	comment string
}

// Measure computes metrics for a file's text in the given language.
func Measure(content, lang string) Metrics {
	return measureFrom(content, scanLines(content, lang), lang)
}

// measureFrom computes metrics from a pre-scanned file (so callers that already
// scanned — e.g. the slop pass — do not scan twice).
func measureFrom(content string, infos []lineInfo, lang string) Metrics {
	m := Metrics{LOC: len(infos)}
	countKinds(content, infos, &m)
	code := codeOf(infos)
	if family(lang) == "indent" {
		m.LongestFunc, m.LongestLine, m.MaxNesting, m.DeepestLine = indentSpans(code)
	} else {
		m.LongestFunc, m.LongestLine, m.MaxNesting, m.DeepestLine = braceSpans(code)
	}
	if m.LOC > 0 {
		m.CommentRatio = float64(m.Comment) / float64(m.LOC)
	}
	return m
}

// codeOf extracts the code-only line slice.
func codeOf(infos []lineInfo) []string {
	out := make([]string, len(infos))
	for i, in := range infos {
		out[i] = in.code
	}
	return out
}

// countKinds tallies blank, comment, and code lines. A line inside a multi-line
// string counts as code (it is part of a code statement), not a comment.
func countKinds(content string, infos []lineInfo, m *Metrics) {
	raw := splitLines(content)
	for i, in := range infos {
		switch {
		case i < len(raw) && strings.TrimSpace(raw[i]) == "":
			m.Blank++
		case strings.TrimSpace(in.code) != "":
			m.Code++
		case in.comment != "":
			m.Comment++
		default:
			m.Code++
		}
	}
}

// scanLines dispatches to the language-family scanner.
func scanLines(content, lang string) []lineInfo {
	raw := splitLines(content)
	if family(lang) == "indent" {
		return scanIndent(raw, lang)
	}
	return scanBrace(raw, lang)
}

// scanBrace tokenizes brace-family source, tracking block comments and multi-line
// strings across lines so their braces never reach the metrics. Backticks are Go
// raw strings (no escapes) in most languages, but in JS/TS they are template
// literals: `tmpl` enables escape (\`) handling and `${…}` interpolation, whose
// embedded code IS measured (and whose braces must not be miscounted) — without it
// a `String.raw` template would swallow the rest of the file and zero its metrics.
func scanBrace(raw []string, lang string) []lineInfo {
	token := lineCommentToken(lang)
	allowBlock := lang != "sh" && lang != "ps1"
	tmpl := lang == "js" || lang == "ts"
	infos := make([]lineInfo, len(raw))
	inBlock, inRaw, interp := false, false, 0
	for i, line := range raw {
		var code, comment strings.Builder
		for j := 0; j < len(line); {
			rest := line[j:]
			switch {
			case inBlock:
				if k := strings.Index(rest, "*/"); k >= 0 {
					comment.WriteString(rest[:k])
					inBlock, j = false, j+k+2
				} else {
					comment.WriteString(rest)
					j = len(line)
				}
			case inRaw:
				switch {
				case tmpl && rest[0] == '\\': // escaped char inside a template literal
					j += 2
				case rest[0] == '`':
					inRaw, j = false, j+1
				case tmpl && strings.HasPrefix(rest, "${"): // interpolation → code mode
					inRaw, interp, j = false, interp+1, j+2
				default:
					j++ // template/raw text — neither code nor comment
				}
			case interp > 0: // inside ${ ... }: real code, measured; brace-balanced
				switch {
				case rest[0] == '{':
					interp++
					code.WriteByte('{')
					j++
				case rest[0] == '}':
					if interp--; interp == 0 {
						inRaw = true // resume the surrounding template text
					} else {
						code.WriteByte('}')
					}
					j++
				case allowBlock && strings.HasPrefix(rest, "/*"):
					if k := strings.Index(rest[2:], "*/"); k >= 0 {
						comment.WriteString(rest[2 : 2+k])
						j += 2 + k + 2
					} else {
						comment.WriteString(rest[2:])
						inBlock, j = true, len(line)
					}
				case strings.HasPrefix(rest, token):
					comment.WriteString(rest[len(token):])
					j = len(line)
				case rest[0] == '"' || rest[0] == '\'':
					j += skipString(rest)
				case rest[0] == '`':
					inRaw, j = true, j+1 // nested template inside interpolation
				default:
					code.WriteByte(rest[0])
					j++
				}
			case allowBlock && strings.HasPrefix(rest, "/*"):
				if k := strings.Index(rest[2:], "*/"); k >= 0 {
					comment.WriteString(rest[2 : 2+k])
					j += 2 + k + 2
				} else {
					comment.WriteString(rest[2:])
					inBlock, j = true, len(line)
				}
			case strings.HasPrefix(rest, token):
				comment.WriteString(rest[len(token):])
				j = len(line)
			case rest[0] == '"' || rest[0] == '\'':
				j += skipString(rest)
			case rest[0] == '`':
				inRaw, j = true, j+1
			default:
				code.WriteByte(rest[0])
				j++
			}
		}
		infos[i] = lineInfo{code: code.String(), comment: comment.String()}
	}
	return infos
}

// scanIndent tokenizes indentation-family source (Python, Ruby), tracking triple-
// quoted strings across lines so a docstring's contents never count as code or
// skew the indentation unit.
func scanIndent(raw []string, lang string) []lineInfo {
	token := lineCommentToken(lang)
	infos := make([]lineInfo, len(raw))
	inTriple := false
	tripleTok := ""
	for i, line := range raw {
		var code, comment strings.Builder
		for j := 0; j < len(line); {
			rest := line[j:]
			switch {
			case inTriple:
				if k := strings.Index(rest, tripleTok); k >= 0 {
					inTriple, j = false, j+k+len(tripleTok)
				} else {
					j = len(line)
				}
			case strings.HasPrefix(rest, `"""`) || strings.HasPrefix(rest, "'''"):
				tripleTok = rest[:3]
				if k := strings.Index(rest[3:], tripleTok); k >= 0 {
					j += 3 + k + 3
				} else {
					inTriple, j = true, len(line)
				}
			case strings.HasPrefix(rest, token):
				comment.WriteString(rest[len(token):])
				j = len(line)
			case rest[0] == '"' || rest[0] == '\'':
				j += skipString(rest)
			default:
				code.WriteByte(rest[0])
				j++
			}
		}
		infos[i] = lineInfo{code: code.String(), comment: comment.String()}
	}
	return infos
}

// skipString returns how many bytes to advance past a quoted string beginning at
// s[0] (the quote), honoring backslash escapes; an unterminated string consumes
// the rest of the line.
func skipString(s string) int {
	q := s[0]
	for i := 1; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == q {
			return i + 1
		}
	}
	return len(s)
}

// callable reports whether a top-level line opens a function-like body: it carries
// a parameter list or an arrow. This keeps top-level type/object literals (which
// have no "()" or "=>") from being counted as functions.
func callable(line string) bool {
	return (strings.Contains(line, "(") && strings.Contains(line, ")")) || strings.Contains(line, "=>")
}

// braceSpans finds the longest function-like body (in lines, with its start line)
// and the deepest brace nesting (with its line), over code-only lines. A body is a
// depth 0→...→0 span that began on a callable line, so structs and object literals
// are not counted. Returned line numbers are 1-based.
func braceSpans(code []string) (longest, longestLine, maxNesting, deepestLine int) {
	depth, start := 0, -1
	for i, line := range code {
		opens := strings.Count(line, "{")
		closes := strings.Count(line, "}")
		if depth == 0 && start < 0 && opens > closes && callable(line) {
			start = i
		}
		depth += opens - closes
		if depth > maxNesting {
			maxNesting, deepestLine = depth, i+1
		}
		if start >= 0 && depth <= 0 {
			if span := i - start + 1; span > longest {
				longest, longestLine = span, start+1
			}
			start, depth = -1, 0
		}
	}
	return longest, longestLine, maxNesting, deepestLine
}

var (
	defLine       = regexp.MustCompile(`^(\s*)(async\s+)?def\s`)
	blockOpenerRe = regexp.MustCompile(`^(?:if|elif|else|for|while|with|try|except|finally|def|class|match|case|async)\b.*:\s*$`)
)

// indentSpans measures the longest def body (with its start line) and deepest
// block nesting (with its line) over code-only lines for indentation-structured
// languages. Nesting counts enclosing block openers (lines ending in ':'), NOT raw
// indentation, so a wrapped multi-line call argument never inflates depth. Blank
// entries (comments, blanks, string interiors) are skipped. Line numbers are 1-based.
func indentSpans(code []string) (longest, longestLine, maxNesting, deepestLine int) {
	defIndent, defStart, lastBody := -1, -1, -1
	closeDef := func(end int) {
		if defStart >= 0 {
			if span := end - defStart + 1; span > longest {
				longest, longestLine = span, defStart+1
			}
			defIndent, defStart = -1, -1
		}
	}
	var openers []int // indents of the block openers strictly enclosing the line
	for i, line := range code {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := leadingCols(line)
		for len(openers) > 0 && indent <= openers[len(openers)-1] {
			openers = openers[:len(openers)-1] // dedented out of these blocks
		}
		if depth := len(openers) + 1; depth > maxNesting {
			maxNesting, deepestLine = depth, i+1
		}
		if blockOpenerRe.MatchString(strings.TrimSpace(line)) {
			openers = append(openers, indent)
		}
		if defStart >= 0 && indent <= defIndent {
			closeDef(lastBody)
		}
		if m := defLine.FindStringSubmatch(line); m != nil && defStart < 0 {
			defIndent, defStart = len(m[1]), i
		}
		lastBody = i
	}
	closeDef(lastBody)
	return longest, longestLine, maxNesting, deepestLine
}

// leadingCols counts leading indentation columns (a tab counts as one).
func leadingCols(line string) int {
	n := 0
	for _, r := range line {
		if r == ' ' || r == '\t' {
			n++
			continue
		}
		break
	}
	return n
}
