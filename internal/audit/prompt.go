package audit

import (
	"fmt"
	"path"
	"strings"

	"github.com/schylerryan/humify/internal/textutil"
)

// RenderPrompt builds the auditor prompt for one area. The auditor is a
// read-only judgment worker: it reads exactly the area's files and writes one
// fragment JSON. Everything deterministic (which files, where the fragment
// goes, the gather, the merge) is owned by the binary — the prompt only asks
// for judgment, in the exact machine-readable shape consolidate already parses.
//
// The contract is deliberately strict because the fragment feeds a
// machine-parsed merge: mandatory severity on every finding, and no newlines in
// title/file (the consolidate stage rejects such fragments to stop a forged
// "### BLOCKERS (N)" header reaching AUDIT.md, so a violating auditor's whole
// fragment is thrown away).
func RenderPrompt(j Job, target string) string {
	// Display paths are always forward-slash for the prompt. The area's own files
	// arrive forward-slashed (scan normalizes them), but target may be native
	// (e.g. a Windows "C:\src"); normalize it so path.Join never mixes separators.
	target = textutil.ToForwardSlash(target)
	var b strings.Builder
	fmt.Fprintf(&b, "# Humify auditor — area %s\n\n", j.AreaID)
	b.WriteString("## Stance\n")
	b.WriteString("You are a code auditor with a FORCE stance: assume this area contains " +
		"human-hostile code until the evidence says otherwise. Your job is to find what makes " +
		"this code hard for a human to read, reason about, and safely change — god functions, " +
		"dead or duplicated code, misleading names, hidden/global mutable state, deep nesting, " +
		"silent error swallowing, leaky abstractions, and undocumented coupling to other areas. " +
		"Report only real, located hazards. Do not invent findings to fill a quota; an area with " +
		"no genuine hazard yields an empty findings list, and that is a valid result.\n\n")

	b.WriteString("## Assignment\n")
	fmt.Fprintf(&b, "- Area: `%s` (kind: %s, root: `%s`, ~%d LOC)\n", j.AreaID, j.Kind, j.Root, j.LOC)
	fmt.Fprintf(&b, "- Target codebase root: `%s`\n", target)
	b.WriteString("- Read ONLY these files (paths are relative to the target root):\n")
	for _, f := range j.Files {
		fmt.Fprintf(&b, "    - `%s`\n", textutil.ToForwardSlash(path.Join(target, f)))
	}
	if len(j.Files) == 0 {
		fmt.Fprintf(&b, "    - (no files listed; read everything under `%s`)\n", textutil.ToForwardSlash(path.Join(target, j.Root)))
	}
	b.WriteString("\n")

	b.WriteString("## Output — write exactly one file\n")
	fmt.Fprintf(&b, "Write your fragment to `%s` (relative to the humify project root) and nothing else. "+
		"Do NOT modify any source file. Return a one-line confirmation when done.\n\n", j.FragmentPath)
	b.WriteString("The file MUST be JSON of this exact shape:\n\n")
	b.WriteString("```json\n")
	fmt.Fprintf(&b, `{
  "area_id": %q,
  "findings": [
    {
      "id": "%s-1",
      "title": "short, single-line hazard name",
      "severity": "blocker | warning | info",
      "file": "%s",
      "line": 42,
      "detail": "what is wrong, why it is human-hostile, and what a fix would look like",
      "refs": ["other-area-id-this-couples-to"]
    }
  ]
}`, j.AreaID, j.AreaID, firstFile(j))
	b.WriteString("\n```\n\n")

	b.WriteString("## Rules\n")
	b.WriteString("- `severity` is mandatory on every finding and must be exactly `blocker`, `warning`, or `info`.\n")
	b.WriteString("    - `blocker`: actively dangerous or a correctness/security hazard a human would likely trip on.\n")
	b.WriteString("    - `warning`: real maintainability hazard that should be fixed but is not urgent.\n")
	b.WriteString("    - `info`: minor smell or note.\n")
	b.WriteString("- `title` and `file` MUST be single-line: no newlines or carriage returns (a fragment that " +
		"violates this is rejected whole).\n")
	b.WriteString("- `file` is a path relative to the target root; `line` is a 1-based line number.\n")
	b.WriteString("- `id` must be unique within this fragment (e.g. `" + j.AreaID + "-1`, `-2`, ...).\n")
	b.WriteString("- `refs` lists OTHER area ids this finding couples to (for cross-area dedup); omit or `[]` if none.\n")
	b.WriteString("- Stay inside your assigned files. If a hazard's root cause lives in another area, note it " +
		"via `refs` rather than auditing that area yourself.\n")
	return b.String()
}

// firstFile returns a representative file path for the schema example so the
// auditor sees the expected relative-path shape, not an abstract placeholder.
func firstFile(j Job) string {
	if len(j.Files) > 0 {
		return j.Files[0]
	}
	return j.Root
}
