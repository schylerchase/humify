package audit

import (
	"strings"
	"testing"
)

// A Windows-style --target must render forward-slashed in the auditor prompt on
// every host. filepath.ToSlash is a no-op when the OS separator is already `/`
// (macOS/Linux), so it would leave `C:\src` intact and path.Join would emit a
// mixed `C:\src/a.go`; this guards that platform-dependent trap.
func TestAuditPromptNormalizesTarget(t *testing.T) {
	j := Job{AreaID: "01-a", Kind: "dir", Root: "a", Files: []string{"a/b.go"}, FragmentPath: "f"}
	out := RenderPrompt(j, `C:\src`)
	if !strings.Contains(out, "C:/src/a/b.go") {
		t.Fatalf("auditor target not normalized to forward slashes:\n%s", out)
	}
	if strings.Contains(out, `\`) {
		t.Fatalf("auditor prompt leaked a backslash (mixed separators):\n%s", out)
	}
}

// The no-files branch joins target with the area Root for the "read everything
// under" hint; it must be normalized too.
func TestAuditPromptNormalizesTargetNoFiles(t *testing.T) {
	j := Job{AreaID: "01-a", Kind: "dir", Root: "sub/dir", Files: nil, FragmentPath: "f"}
	out := RenderPrompt(j, `C:\src`)
	if !strings.Contains(out, "C:/src/sub/dir") {
		t.Fatalf("no-files target hint not normalized:\n%s", out)
	}
	if strings.Contains(out, `\`) {
		t.Fatalf("auditor prompt leaked a backslash (mixed separators):\n%s", out)
	}
}
