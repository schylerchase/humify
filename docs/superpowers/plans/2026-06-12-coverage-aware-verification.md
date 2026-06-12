# Coverage-Aware Verification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make humify honestly distinguish "a test executed this code" (`behavior-verified`) from "this code merely still compiles" (`build-only`) for every change it validates, using real coverage instrumentation.

**Architecture:** A new `verify.Coverage(root)` runs the project's test suite under coverage and returns a `CoverageReport` (file → covered lines), persisted once to `.humify/coverage.json`. The wiring layer (`humify.go`) crosses that report with each applyable plan item's files to set a `Verification` verdict, which rides into `plan.json`, the apply `Result`, and the quarantine `Manifest`. The proven quarantine→verify→rollback path is untouched — coverage is a read-only input, computed once, never recomputed during apply's gate runs. Gating is additive: `build-only` warns and records, never blocks.

**Tech Stack:** Go (zero new dependencies). Coverage adapters shell out to the project's own toolchain — `go test -coverprofile` (Phase 1), `c8`/`nyc` Istanbul JSON (Phase 2).

**Reference:** `docs/superpowers/specs/2026-06-12-humify-coverage-aware-verification-design.md`

**Phasing:** Phase 1 (Tasks 1–7) delivers working coverage-aware verification for **Go** projects — humify can dogfood it on itself. Phase 2 (Tasks 8–10) adds the **JS/TS** adapter, which fixes the originally-motivating gap (Azure-Mapper). Each phase produces working, testable software.

---

## File Structure

- **Create** `internal/humify/verify/coverage.go` — `CoverageReport`/`FileCoverage` types, `Verdict` + `VerdictFor`, the `Provider` interface, Go provider, `Coverage(root)` orchestrator.
- **Create** `internal/humify/verify/coverage_test.go` — verdict truth table, Go coverprofile parser fixtures, orchestrator behavior.
- **Create** `internal/humify/verify/coverage_js.go` — JS/TS provider + Istanbul parser (Phase 2).
- **Create** `internal/humify/verify/coverage_js_test.go` — Istanbul parser fixtures (Phase 2).
- **Modify** `internal/humify/state/state.go` — register `CoverageFile = "coverage.json"`.
- **Modify** `humify.go` — `cmdVerify`/`cmdPlan` produce + persist coverage and decorate plan items with verdicts; honor `--no-coverage`.
- **Modify** `main.go` — parse `--no-coverage` into `options`.
- **Modify** `internal/humify/plan/plan.go` — add `Item.Verification` field (populated by the caller; no new import).
- **Modify** `internal/humify/apply/apply.go` — `Result` + `Manifest` carry `Verification`; message names the verdict.
- **Modify** `internal/humify/render.go` — surface the verdict in apply/plan output.

---

## Task 1: Coverage types + verdict

**Files:**
- Create: `internal/humify/verify/coverage.go`
- Test: `internal/humify/verify/coverage_test.go`

- [ ] **Step 1: Write the failing test (verdict truth table)**

```go
package verify

import "testing"

func report(measured bool, files map[string]FileCoverage) CoverageReport {
	return CoverageReport{Schema: 1, Measured: measured, Files: files}
}

func TestVerdictFor(t *testing.T) {
	covered := map[string]FileCoverage{"a.go": {Covered: true, Lines: []int{3}}, "b.go": {Covered: false}}
	tests := []struct {
		name string
		rep  CoverageReport
		file string
		want Verdict
	}{
		{"measured+covered -> behavior-verified", report(true, covered), "a.go", VerdictBehaviorVerified},
		{"measured+uncovered -> build-only", report(true, covered), "b.go", VerdictBuildOnly},
		{"measured+absent -> build-only", report(true, covered), "z.go", VerdictBuildOnly},
		{"unmeasured -> unmeasured", report(false, nil), "a.go", VerdictUnmeasured},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rep.VerdictFor(tt.file); got != tt.want {
				t.Errorf("VerdictFor(%q) = %q, want %q", tt.file, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/humify/verify/ -run TestVerdictFor`
Expected: FAIL — `undefined: FileCoverage / CoverageReport / Verdict`.

- [ ] **Step 3: Write minimal implementation**

```go
// Package-level additions in internal/humify/verify/coverage.go
package verify

// FileCoverage records whether a file was executed by the test suite and which
// of its lines were hit. Covered is the load-bearing field; Lines is captured for
// later line-level use (dead-function detection) and is not required by v1.
type FileCoverage struct {
	Covered bool  `json:"covered"`
	Lines   []int `json:"lines,omitempty"`
}

// CoverageReport is the per-file coverage of one test run. Measured is false when
// no coverage tooling could run — verdicts then become Unmeasured, never a silent
// pass.
type CoverageReport struct {
	Schema   int                     `json:"schema"`
	Tool     string                  `json:"tool"` // "go" | "c8" | "nyc" | ""
	Measured bool                    `json:"measured"`
	Files    map[string]FileCoverage `json:"files"`
}

// Verdict is the honest strength of verification for one file.
type Verdict string

const (
	VerdictBehaviorVerified Verdict = "behavior-verified"
	VerdictBuildOnly        Verdict = "build-only"
	VerdictUnmeasured       Verdict = "unmeasured"
)

// VerdictFor returns the verification verdict for a repo-relative file path. An
// unmeasured report yields Unmeasured; a measured report yields BehaviorVerified
// iff the file has covered lines, else BuildOnly (the suite did not execute it).
func (r CoverageReport) VerdictFor(file string) Verdict {
	if !r.Measured {
		return VerdictUnmeasured
	}
	if fc, ok := r.Files[file]; ok && fc.Covered {
		return VerdictBehaviorVerified
	}
	return VerdictBuildOnly
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/humify/verify/ -run TestVerdictFor`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/humify/verify/coverage.go internal/humify/verify/coverage_test.go
git commit -m "feat(verify): coverage report types and verification verdict"
```

---

## Task 2: Go coverprofile parser

**Files:**
- Modify: `internal/humify/verify/coverage.go`
- Test: `internal/humify/verify/coverage_test.go`

Go's `-coverprofile` output looks like (paths are import paths, not repo-relative):

```
mode: set
github.com/me/proj/pkg/a.go:3.10,5.2 1 1
github.com/me/proj/pkg/b.go:7.10,9.2 1 0
```

A file is covered iff any of its blocks has count > 0. `modulePath` (from `go.mod`'s `module` line) is stripped to get a repo-relative path.

- [ ] **Step 1: Write the failing test**

```go
func TestParseGoProfile(t *testing.T) {
	profile := "mode: set\n" +
		"github.com/me/proj/pkg/a.go:3.10,5.2 1 1\n" +
		"github.com/me/proj/pkg/a.go:6.2,6.20 1 0\n" +
		"github.com/me/proj/pkg/b.go:7.10,9.2 1 0\n"
	files := parseGoProfile(profile, "github.com/me/proj")
	if !files["pkg/a.go"].Covered {
		t.Errorf("a.go has a hit block (count 1) -> must be Covered; got %+v", files["pkg/a.go"])
	}
	if files["pkg/b.go"].Covered {
		t.Errorf("b.go has only a zero-count block -> must NOT be Covered; got %+v", files["pkg/b.go"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/humify/verify/ -run TestParseGoProfile`
Expected: FAIL — `undefined: parseGoProfile`.

- [ ] **Step 3: Write minimal implementation**

```go
import (
	"bufio"
	"strconv"
	"strings"
)

// parseGoProfile turns a `go test -coverprofile` body into per-file coverage,
// keyed by repo-relative path (modulePath stripped). A file is Covered iff any
// block executed (trailing count > 0).
func parseGoProfile(profile, modulePath string) map[string]FileCoverage {
	files := map[string]FileCoverage{}
	sc := bufio.NewScanner(strings.NewReader(profile))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		// <path>:<sl>.<sc>,<el>.<ec> <numStmts> <count>
		fields := strings.Fields(line)
		if len(fields) != 3 {
			continue
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil {
			continue
		}
		colon := strings.LastIndex(fields[0], ":")
		if colon < 0 {
			continue
		}
		path := strings.TrimPrefix(fields[0][:colon], modulePath+"/")
		rng := fields[0][colon+1:] // "3.10,5.2"
		fc := files[path]
		if count > 0 {
			fc.Covered = true
			if startLine := leadingLine(rng); startLine > 0 {
				fc.Lines = append(fc.Lines, startLine)
			}
		}
		files[path] = fc
	}
	return files
}

// leadingLine returns the start line number from a coverprofile range "sl.sc,el.ec".
func leadingLine(rng string) int {
	dot := strings.IndexByte(rng, '.')
	if dot < 0 {
		return 0
	}
	n, _ := strconv.Atoi(rng[:dot])
	return n
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/humify/verify/ -run TestParseGoProfile`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/humify/verify/coverage.go internal/humify/verify/coverage_test.go
git commit -m "feat(verify): parse go coverprofile into per-file coverage"
```

---

## Task 3: Go provider + Coverage orchestrator

**Files:**
- Modify: `internal/humify/verify/coverage.go`
- Test: `internal/humify/verify/coverage_test.go`

- [ ] **Step 1: Write the failing test (Detect + end-to-end Run on a real tiny module)**

```go
import (
	"os"
	"os/exec"
	"path/filepath"
)

func TestGoProviderRun(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the go toolchain")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	root := t.TempDir()
	write := func(rel, body string) {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module covdemo\n\ngo 1.26\n")
	write("covered.go", "package covdemo\n\nfunc Used() int { return 1 }\n")
	write("uncovered.go", "package covdemo\n\nfunc Unused() int { return 2 }\n")
	write("covered_test.go", "package covdemo\n\nimport \"testing\"\n\nfunc TestUsed(t *testing.T){ if Used()!=1 {t.Fail()} }\n")

	p := goProvider{}
	if !p.Detect(root) {
		t.Fatal("goProvider must Detect a go.mod project")
	}
	rep, err := p.Run(root)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !rep.Measured || rep.Tool != "go" {
		t.Fatalf("expected a measured go report, got %+v", rep)
	}
	if !rep.Files["covered.go"].Covered {
		t.Error("covered.go is exercised by TestUsed -> must be Covered")
	}
	if rep.Files["uncovered.go"].Covered {
		t.Error("uncovered.go is compiled but never run by a test -> must be build-only (not Covered)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/humify/verify/ -run TestGoProviderRun`
Expected: FAIL — `undefined: goProvider`.

- [ ] **Step 3: Write minimal implementation**

```go
import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Provider runs a language's test suite under coverage and reports per-file
// coverage. A provider is used only when Detect is true for the repo.
type Provider interface {
	Name() string
	Detect(root string) bool
	Run(root string) (CoverageReport, error)
}

// providers is the ordered registry. The first whose Detect matches wins.
var providers = []Provider{goProvider{}}

// Coverage produces a coverage report for root by running the first matching
// provider's instrumented test suite. When no provider matches (or it errors),
// it returns an unmeasured report — never an error the caller must handle to stay
// honest; Measured:false is the honest "couldn't measure" signal.
func Coverage(root string) CoverageReport {
	for _, p := range providers {
		if !p.Detect(root) {
			continue
		}
		rep, err := p.Run(root)
		if err != nil {
			return CoverageReport{Schema: state.Schema, Tool: p.Name(), Measured: false, Files: map[string]FileCoverage{}}
		}
		return rep
	}
	return CoverageReport{Schema: state.Schema, Measured: false, Files: map[string]FileCoverage{}}
}

type goProvider struct{}

func (goProvider) Name() string         { return "go" }
func (goProvider) Detect(root string) bool { return exists(filepath.Join(root, "go.mod")) }

func (goProvider) Run(root string) (CoverageReport, error) {
	prof := filepath.Join(root, ".humify-cover.out")
	defer os.Remove(prof)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "-coverprofile="+prof, "./...")
	cmd.Dir = root
	_ = cmd.Run() // a failing/empty test suite still yields a (partial) profile
	data, err := os.ReadFile(prof)
	if err != nil {
		return CoverageReport{}, err
	}
	return CoverageReport{
		Schema:   state.Schema,
		Tool:     "go",
		Measured: true,
		Files:    parseGoProfile(string(data), goModulePath(root)),
	}, nil
}

// goModulePath reads the module path from go.mod, or "" if unreadable.
func goModulePath(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}
```

Add `"github.com/schylerryan/humify/internal/humify/state"` to the imports if not already present.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/humify/verify/ -run TestGoProviderRun`
Expected: PASS (covered.go Covered, uncovered.go not).

- [ ] **Step 5: Commit**

```bash
git add internal/humify/verify/coverage.go internal/humify/verify/coverage_test.go
git commit -m "feat(verify): go coverage provider and Coverage orchestrator"
```

---

## Task 4: Persist coverage.json + `--no-coverage`

**Files:**
- Modify: `internal/humify/state/state.go:20-22` (file name constants)
- Modify: `main.go` (options struct + parseArgs)
- Modify: `humify.go` (`cmdVerify` produces + persists coverage)
- Test: `internal/humify/verify/coverage_test.go` (round-trip), manual CLI check

- [ ] **Step 1: Register the state file**

In `internal/humify/state/state.go`, alongside `AnalysisFile`/`PlanFile`/`ValidationFile`:

```go
	ValidationFile = "validation.json"
	CoverageFile   = "coverage.json"
```

- [ ] **Step 2: Add the `--no-coverage` flag**

In `main.go`, add to the `options` struct:

```go
	noCoverage bool
```

In `parseArgs`, alongside the other `--flag` cases:

```go
	case a == "--no-coverage":
		opts.noCoverage = true
```

- [ ] **Step 3: Produce + persist coverage in `cmdVerify`**

In `humify.go` `cmdVerify`, after the existing `verify.Run(...)` + its `hstate.Save(root, hstate.ValidationFile, v)`, add:

```go
	if !opts.noCoverage {
		cov := verify.Coverage(root)
		_ = hstate.Save(root, hstate.CoverageFile, cov)
	}
```

(`cmdAnalyze`/`cmdPlan` do not run coverage themselves — coverage comes from `verify`. `cmdPlan` reads the cached file in Task 5.)

- [ ] **Step 4: Verify build + a manual round-trip**

Run: `go build ./... && go test ./internal/humify/verify/`
Then a manual dogfood on humify itself:
Run: `go run . verify . && cat .humify/coverage.json | head`
Expected: `coverage.json` exists; `"measured": true`, `"tool": "go"`, and humify's own well-tested files (e.g. `internal/humify/verify/coverage.go`) show `"covered": true`.

- [ ] **Step 5: Commit**

```bash
git add internal/humify/state/state.go main.go humify.go
git commit -m "feat(verify): persist coverage.json from verify; add --no-coverage"
```

---

## Task 5: Verdict on plan items + render

**Files:**
- Modify: `internal/humify/plan/plan.go` (add `Item.Verification`)
- Modify: `humify.go` (`cmdPlan` decorates items from coverage.json)
- Modify: `internal/humify/render.go` (show verdict on applyable items)
- Test: `internal/humify/plan/plan_test.go`

- [ ] **Step 1: Write the failing test (field exists + serializes)**

In `internal/humify/plan/plan_test.go`:

```go
func TestItemCarriesVerification(t *testing.T) {
	it := Item{ID: "HMF-001", Signal: "dead_module", Verification: "build-only"}
	if it.Verification != "build-only" {
		t.Fatalf("Item must carry a Verification verdict; got %q", it.Verification)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/humify/plan/ -run TestItemCarriesVerification`
Expected: FAIL — `unknown field Verification in struct literal`.

- [ ] **Step 3: Add the field**

In `internal/humify/plan/plan.go`, in the `Item` struct:

```go
	Verification string `json:"verification,omitempty"` // behavior-verified|build-only|unmeasured (applyable items)
```

- [ ] **Step 4: Decorate items in the wiring layer**

In `humify.go` `cmdPlan`, after `p := hplan.Build(a)` and before `hstate.Save(root, hstate.PlanFile, p)`:

```go
	var cov verify.CoverageReport
	if hstate.Load(root, hstate.CoverageFile, &cov) == nil {
		for i := range p.Items {
			if !p.Items[i].Applyable {
				continue
			}
			v := cov.WorstVerdict(p.Items[i].Files) // worst across the item's files
			p.Items[i].Verification = string(v)
		}
	}
```

Add `WorstVerdict` to `internal/humify/verify/coverage.go` (a build-only file dominates a behavior-verified one; unmeasured if the report is unmeasured):

```go
// WorstVerdict returns the least-verified verdict across files: if the report is
// unmeasured, Unmeasured; else BuildOnly if ANY file is uncovered, else
// BehaviorVerified. An item is only as trustworthy as its weakest file.
func (r CoverageReport) WorstVerdict(files []string) Verdict {
	if !r.Measured {
		return VerdictUnmeasured
	}
	worst := VerdictBehaviorVerified
	for _, f := range files {
		if r.VerdictFor(f) == VerdictBuildOnly {
			worst = VerdictBuildOnly
		}
	}
	return worst
}
```

(Confirm `hstate.Load(root, name, &v)` exists — `state.go` Task 4 area shows `Save`; `Load` is its read counterpart used by `cmdPlan`'s `loadOrAnalyze`.)

- [ ] **Step 5: Show it in render**

In `internal/humify/render.go`, where applyable plan items are printed, append the verdict when present, e.g.:

```go
	if it.Applyable && it.Verification != "" {
		fmt.Fprintf(w, "    verification: %s\n", it.Verification)
	}
```

- [ ] **Step 6: Run tests + manual check**

Run: `go test ./internal/humify/... && go run . plan .`
Expected: plan tests pass; `plan` output shows `verification: ...` on applyable items.

- [ ] **Step 7: Commit**

```bash
git add internal/humify/plan/plan.go internal/humify/verify/coverage.go humify.go internal/humify/render.go internal/humify/plan/plan_test.go
git commit -m "feat(plan): attach coverage verdict to applyable items"
```

---

## Task 6: Verdict in apply Result + Manifest + message

**Files:**
- Modify: `internal/humify/apply/apply.go` (`Result`, `Manifest`, message)
- Test: `internal/humify/apply/apply_test.go`

- [ ] **Step 1: Write the failing test**

In `internal/humify/apply/apply_test.go` — extend the existing quarantine E2E to assert the verdict propagates. Add a focused test:

```go
func TestApplyRecordsVerificationVerdict(t *testing.T) {
	root, p := buildRepo(t)
	item, _ := p.Find("HMF-001") // the stale_file quarantine
	item.Verification = "build-only"
	// Re-find through Apply by setting the verdict on the plan item it will use:
	for i := range p.Items {
		if p.Items[i].ID == item.ID {
			p.Items[i].Verification = "build-only"
		}
	}
	res, err := Apply(root, p, item.ID, false, true, "", false, time.Now())
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Verification != "build-only" {
		t.Errorf("Result must carry the item's verdict; got %q", res.Verification)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/humify/apply/ -run TestApplyRecordsVerificationVerdict`
Expected: FAIL — `res.Verification undefined`.

- [ ] **Step 3: Add the field + propagate**

In `internal/humify/apply/apply.go`, add to `Result`:

```go
	Verification string `json:"verification,omitempty"`
```

and to `Manifest`:

```go
	Verification string `json:"verification,omitempty"`
```

In `Apply`, set `res.Verification = item.Verification` where `res` is initialized, and copy it into the manifest in `performQuarantine` (set `man.Verification = item.Verification` when the manifest is built). When the verdict is `build-only`, append to the success message: `" (build-only: no test exercised this file)"`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/humify/apply/ -run TestApplyRecordsVerificationVerdict`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/humify/apply/apply.go internal/humify/apply/apply_test.go
git commit -m "feat(apply): record and surface the coverage verdict on quarantine"
```

---

## Task 7: End-to-end + dogfood (Phase 1 done)

**Files:**
- Test: `internal/humify/verify/coverage_test.go`

- [ ] **Step 1: Write a verdict E2E that crosses a real go run with VerdictFor**

```go
func TestCoverageVerdictEndToEndGo(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the go toolchain")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	// (reuse the tiny module construction from TestGoProviderRun via a helper)
	root := newCovDemoModule(t)
	rep := Coverage(root)
	if rep.VerdictFor("covered.go") != VerdictBehaviorVerified {
		t.Error("covered.go should be behavior-verified")
	}
	if rep.VerdictFor("uncovered.go") != VerdictBuildOnly {
		t.Error("uncovered.go should be build-only")
	}
}
```

Extract `newCovDemoModule(t)` from Task 3's test (move the `write(...)` setup into a shared helper in `coverage_test.go`).

- [ ] **Step 2: Run the full suite**

Run: `go test ./...`
Expected: PASS (all packages).

- [ ] **Step 3: Dogfood on humify itself**

Run: `go run . verify . && go run . plan .`
Expected: `verification:` verdicts appear; well-tested files behavior-verified, untested ones build-only.

- [ ] **Step 4: Commit**

```bash
git add internal/humify/verify/coverage_test.go
git commit -m "test(verify): end-to-end go coverage verdict"
```

**>>> Phase 1 complete: coverage-aware verification works for Go projects. Tag-worthy as v0.3.0-go or hold for Phase 2.**

---

## Task 8: JS/TS invocation spike (empirical — NOT a guess)

**Files:**
- Scratch only; produces a documented command, then `internal/humify/verify/coverage_js.go`

The exact `c8`/`nyc` invocation that wraps an arbitrary `npm run test` and emits per-file JSON is an empirical unknown. **Do not pseudo-code it.** Spike it against a real repo first.

- [ ] **Step 1: Determine the working invocation on a real node repo**

On a copy of a node project with a `test` script (e.g. the Azure-Mapper repo), try, in order, and record which produces a parseable `coverage-final.json`:

```bash
npx --no-install c8 --reporter=json --reports-dir=.humify-cov npm test
# fallback if c8 absent:
npx --no-install nyc --reporter=json --report-dir=.humify-cov npm test
```

Record: the exact command, the output file path (`<reports-dir>/coverage-final.json`), and a 30-line sample of its JSON shape (Istanbul format: keys are absolute file paths; each has `path`, `statementMap` {id→{start:{line}}}, and `s` {id→hitCount}).

- [ ] **Step 2: Write the detection rule**

Decide `jsProvider.Detect`: true when `package.json` exists AND a `test` script is declared AND (`node_modules/.bin/c8` or `node_modules/.bin/nyc` exists). If neither tool is installed, Detect is false → report stays `unmeasured` (honest). Document this in the code comment.

- [ ] **Step 3: Commit the spike findings as a doc comment**

Put the recorded command + JSON sample into the header comment of `internal/humify/verify/coverage_js.go` (created in Task 9) so the parser task has ground truth. No code commit yet.

---

## Task 9: JS Istanbul parser

**Files:**
- Create: `internal/humify/verify/coverage_js.go`
- Create: `internal/humify/verify/coverage_js_test.go`

Istanbul `coverage-final.json` shape (from the spike):

```json
{
  "/abs/repo/src/a.js": {
    "path": "/abs/repo/src/a.js",
    "statementMap": { "0": {"start":{"line":3}}, "1": {"start":{"line":4}} },
    "s": { "0": 2, "1": 0 }
  }
}
```

- [ ] **Step 1: Write the failing test**

```go
package verify

import "testing"

func TestParseIstanbul(t *testing.T) {
	data := `{
      "/abs/repo/src/a.js": {"path":"/abs/repo/src/a.js","statementMap":{"0":{"start":{"line":3}},"1":{"start":{"line":4}}},"s":{"0":2,"1":0}},
      "/abs/repo/src/b.js": {"path":"/abs/repo/src/b.js","statementMap":{"0":{"start":{"line":5}}},"s":{"0":0}}
    }`
	files, err := parseIstanbul([]byte(data), "/abs/repo")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !files["src/a.js"].Covered {
		t.Errorf("a.js has a hit statement (s.0=2) -> Covered; got %+v", files["src/a.js"])
	}
	if files["src/b.js"].Covered {
		t.Errorf("b.js has only a zero-hit statement -> not Covered; got %+v", files["src/b.js"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/humify/verify/ -run TestParseIstanbul`
Expected: FAIL — `undefined: parseIstanbul`.

- [ ] **Step 3: Write minimal implementation**

```go
package verify

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// istanbulFile is the subset of an Istanbul coverage-final.json entry we read.
type istanbulFile struct {
	Path         string                 `json:"path"`
	StatementMap map[string]struct {
		Start struct {
			Line int `json:"line"`
		} `json:"start"`
	} `json:"statementMap"`
	S map[string]int `json:"s"` // statement id -> hit count
}

// parseIstanbul turns a coverage-final.json body into per-file coverage keyed by
// repo-relative path (root stripped). A file is Covered iff any statement hit > 0.
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/humify/verify/ -run TestParseIstanbul`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/humify/verify/coverage_js.go internal/humify/verify/coverage_js_test.go
git commit -m "feat(verify): parse istanbul coverage-final.json"
```

---

## Task 10: JS provider wiring + dogfood

**Files:**
- Modify: `internal/humify/verify/coverage_js.go` (provider), `internal/humify/verify/coverage.go` (register in `providers`)

- [ ] **Step 1: Implement `jsProvider` using the spiked command**

```go
// jsProvider runs the project's test script under c8/nyc and parses Istanbul
// JSON. Detect requires a declared test script AND an installed c8/nyc binary;
// otherwise coverage stays unmeasured (honest) rather than guessed.
type jsProvider struct{}

func (jsProvider) Name() string { return "c8" }

func (jsProvider) Detect(root string) bool {
	if !exists(filepath.Join(root, "package.json")) {
		return false
	}
	return exists(filepath.Join(root, "node_modules", ".bin", "c8")) ||
		exists(filepath.Join(root, "node_modules", ".bin", "nyc"))
}

func (jsProvider) Run(root string) (CoverageReport, error) {
	// <exact command + tool determined by Task 8 spike>
	// run under timeout; read <reports-dir>/coverage-final.json; parseIstanbul.
	// On any failure, return (CoverageReport{}, err) so Coverage() degrades to unmeasured.
}
```

Register it: `var providers = []Provider{goProvider{}, jsProvider{}}` in `coverage.go`.

- [ ] **Step 2: Build + unit tests**

Run: `go build ./... && go test ./internal/humify/...`
Expected: PASS.

- [ ] **Step 3: Dogfood on Azure-Mapper (the motivating repo)**

Run `humify verify` then `humify plan` on a copy of Azure-Mapper with c8 installed. Confirm:
- the 8 already-quarantined dead modules would read `build-only` (no tests),
- unit-tested live modules (network-rules, iam-engine, compliance-engine) read `behavior-verified`.

This automatically reproduces the map built by hand during the dogfood session.

- [ ] **Step 4: Commit**

```bash
git add internal/humify/verify/coverage_js.go internal/humify/verify/coverage.go
git commit -m "feat(verify): javascript/typescript coverage provider"
```

**>>> Phase 2 complete: coverage-aware verification covers Go + JS/TS. Ship as v0.3.0.**

---

## Out of scope (follow-on plans)

- Python (`coverage.py`) provider — same `Provider` shape, a third adapter.
- Line-level verdicts feeding **dead-function/export** detection.
- Optional strict gating mode in `humify.config.json` (require behavior-verification to auto-apply).
