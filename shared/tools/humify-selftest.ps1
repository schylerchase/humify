[CmdletBinding()]
param()

# PowerShell Humify evaluator self-test + NEGATIVE CONTROL (mirror of humify_selftest.py).
#
# The identity self-tests (expected vs expected) only prove wiring: by construction they
# score the maximum. This script ADDS a negative control — it feeds deliberately wrong
# audit/plan outputs and asserts the score drops below the readiness threshold and below
# the maximum. That is the proof the evaluator can actually fail, which an identity check
# can never demonstrate. It also unit-checks the parser hardening (verdict-row precedence,
# prose verdict ordering, line-start evidence, ReDoS-safety).
#
# Run:  pwsh -NoProfile -File .\shared\tools\humify-selftest.ps1
# Exit: 0 if all assertions hold, 1 otherwise.

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "humify-lib.ps1")

$toolsDir = $PSScriptRoot
$sharedDir = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot "..")).Path
$expectedDir = Join-Path $sharedDir "expected"
$expectedPlansDir = Join-Path $sharedDir "expected-plans"
$auditScript = Join-Path $toolsDir "humify-evaluate.ps1"
$planScript = Join-Path $toolsDir "humify-evaluate-plans.ps1"

$auditRubric = (Get-Content -LiteralPath (Join-Path $toolsDir "humify-fixtures.json") -Raw) | ConvertFrom-Json
$planRubric = (Get-Content -LiteralPath (Join-Path $toolsDir "humify-plan-fixtures.json") -Raw) | ConvertFrom-Json
$classifications = @($auditRubric.classifications)
$auditRatio = if ($auditRubric.PSObject.Properties['readyRatio']) { [double]$auditRubric.readyRatio } else { 0.86 }
$planRatio = if ($planRubric.PSObject.Properties['readyRatio']) { [double]$planRubric.readyRatio } else { 0.86 }

$JUNK = "# Wrong output`n`nClassification: not a real classification`n`nNothing useful here.`n"
$failures = New-Object System.Collections.Generic.List[string]
$tempDirs = New-Object System.Collections.Generic.List[string]

function Test-Assertion {
  param([string]$Label, [bool]$Condition)
  $tag = if ($Condition) { "PASS" } else { "FAIL" }
  Write-Output "  [$tag] $Label"
  if (-not $Condition) { $failures.Add($Label) }
}

function New-TempDir {
  $d = Join-Path ([System.IO.Path]::GetTempPath()) ("humify-st-" + [guid]::NewGuid().ToString('N'))
  New-Item -ItemType Directory -Path $d | Out-Null
  $script:tempDirs.Add($d)
  return $d
}

function Get-RunScore {
  param([string]$ScriptPath, [hashtable]$Splat)
  $tmpOut = Join-Path ([System.IO.Path]::GetTempPath()) ("humify-st-" + [guid]::NewGuid().ToString('N') + ".md")
  $Splat["OutputPath"] = $tmpOut
  $out = & $ScriptPath @Splat 2>&1 | Out-String
  if ($out -match 'score:\s+(\d+)\s+/\s+(\d+)') {
    return [pscustomobject]@{ Total = [int]$Matches[1]; Max = [int]$Matches[2] }
  }
  throw "evaluator produced no parseable score: $out"
}

function Get-AuditScoreRun {
  param([string]$ActualDir, [string]$ExpectedDir)
  return Get-RunScore -ScriptPath $auditScript -Splat @{ ActualDir = $ActualDir; ExpectedDir = $ExpectedDir }
}

function Get-PlanScoreRun {
  param([string]$ActualPlanDir, [string]$ExpectedPlanDir)
  return Get-RunScore -ScriptPath $planScript -Splat @{ ActualPlanDir = $ActualPlanDir; ExpectedPlanDir = $ExpectedPlanDir }
}

try {
  Write-Output "Humify self-test (PowerShell)"

  Write-Output ""
  Write-Output "1) Identity self-tests (wiring only — must hit the maximum):"
  $ai = Get-AuditScoreRun -ActualDir $expectedDir -ExpectedDir $expectedDir
  Test-Assertion "audit identity == $($ai.Max)/$($ai.Max)" ($ai.Total -eq $ai.Max)
  $pi = Get-PlanScoreRun -ActualPlanDir $expectedPlansDir -ExpectedPlanDir $expectedPlansDir
  Test-Assertion "plan identity == $($pi.Max)/$($pi.Max)" ($pi.Total -eq $pi.Max)

  Write-Output ""
  Write-Output "2) Negative control (deliberately wrong outputs — MUST score low & fail):"
  $aJunk = New-TempDir
  foreach ($f in $auditRubric.fixtures) {
    [System.IO.File]::WriteAllText((Join-Path $aJunk $f.file), $JUNK)
  }
  $na = Get-AuditScoreRun -ActualDir $aJunk -ExpectedDir $expectedDir
  $aThreshold = [int][Math]::Ceiling($na.Max * $auditRatio)
  Test-Assertion "audit junk score $($na.Total) < threshold $aThreshold" ($na.Total -lt $aThreshold)
  Test-Assertion "audit junk score < max (evaluator is falsifiable)" ($na.Total -lt $na.Max)

  $pJunk = New-TempDir
  foreach ($p in $planRubric.plans) {
    [System.IO.File]::WriteAllText((Join-Path $pJunk $p.file), $JUNK)
  }
  $np = Get-PlanScoreRun -ActualPlanDir $pJunk -ExpectedPlanDir $expectedPlansDir
  $pThreshold = [int][Math]::Ceiling($np.Max * $planRatio)
  Test-Assertion "plan junk score $($np.Total) < threshold $pThreshold" ($np.Total -lt $pThreshold)
  Test-Assertion "plan junk score < max (evaluator is falsifiable)" ($np.Total -lt $np.Max)

  Write-Output ""
  Write-Output "3) ExpectedDir is load-bearing (a single flipped classification is detected):"
  $one = New-TempDir
  foreach ($f in $auditRubric.fixtures) {
    Copy-Item -LiteralPath (Join-Path $expectedDir $f.file) -Destination (Join-Path $one $f.file)
  }
  $target = Join-Path $one "machine-shaped-audit.md"
  $flipped = ([System.IO.File]::ReadAllText($target)).Replace("Machine-shaped readability risk", "Clean")
  [System.IO.File]::WriteAllText($target, $flipped)
  $flip = Get-AuditScoreRun -ActualDir $one -ExpectedDir $expectedDir
  Test-Assertion "flipped-classification score $($flip.Total) < max $($flip.Max)" ($flip.Total -lt $flip.Max)

  Write-Output ""
  Write-Output "4) Plan ExpectedDir is load-bearing (hollowing one expected plan is detected):"
  $modExp = New-TempDir
  foreach ($p in $planRubric.plans) {
    Copy-Item -LiteralPath (Join-Path $expectedPlansDir $p.file) -Destination (Join-Path $modExp $p.file)
  }
  [System.IO.File]::WriteAllText((Join-Path $modExp $planRubric.plans[0].file), $JUNK)
  $lb = Get-PlanScoreRun -ActualPlanDir $expectedPlansDir -ExpectedPlanDir $modExp
  Test-Assertion "hollowed-expected-plan score $($lb.Total) < max $($lb.Max)" ($lb.Total -lt $lb.Max)

  Write-Output ""
  Write-Output "5) Parser robustness (latent real-repo hazards the current baselines hide):"
  $hijack = "## Candidates considered`n`n| Candidate | Ruled out |`n| --- | --- |`n| High-risk refactor candidate | yes (has tests) |`n`n## Required classification`n`n| Category | Expected |`n| --- | --- |`n| Overall | Clean |`n"
  Test-Assertion "ruled-out analysis table does not hijack the verdict row" ((Get-Classification -Text $hijack -Classifications $classifications) -eq "Clean")

  $prose = "Considered ``High-risk refactor candidate``; ruled it out.`n`nClassification: Clean`n"
  Test-Assertion "prose verdict line beats a ruled-out mention" ((Get-Classification -Text $prose -Classifications $classifications) -eq "Clean")

  $look = "Performance profile: nominal`nFunction outlines: three`nSymptom: slow`nCausal chain: a -> b`nConfig prefix: app_`n"
  Test-Assertion "evidence check rejects substring look-alikes (profile:/outlines:/prefix:)" ((Test-HasAuditEvidence -Text $look) -eq $false)

  $real = [System.IO.File]::ReadAllText((Join-Path $expectedDir "machine-shaped-audit.md"))
  Test-Assertion "a properly labeled finding still registers as evidence" ((Test-HasAuditEvidence -Text $real) -eq $true)

  $multi = "### a.py`nClassification: Clean`n`n### b.py`nClassification: High-risk refactor candidate`n"
  Test-Assertion "multi-section verdict resolves to worst-of-present, not first-stated" ((Get-Classification -Text $multi -Classifications $classifications) -eq "High-risk refactor candidate")

  $pad = (" " * 1200)
  $sw = [System.Diagnostics.Stopwatch]::StartNew(); [void](Test-HasAuditEvidence -Text ($pad + "x")); $evMs = $sw.ElapsedMilliseconds
  $sw = [System.Diagnostics.Stopwatch]::StartNew(); [void](Get-Classification -Text ("classification:" + $pad) -Classifications $classifications); $clMs = $sw.ElapsedMilliseconds
  Test-Assertion "parsers stay linear on long whitespace (evidence ${evMs}ms, classify ${clMs}ms; must be < 500ms each)" (($evMs -lt 500) -and ($clMs -lt 500))
} finally {
  foreach ($d in $tempDirs) {
    if (Test-Path -LiteralPath $d) { Remove-Item -LiteralPath $d -Recurse -Force -ErrorAction SilentlyContinue }
  }
}

Write-Output ""
if ($failures.Count -gt 0) {
  Write-Output "SELF-TEST FAILED: $($failures.Count) assertion(s) failed: $($failures -join ', ')"
  exit 1
}
Write-Output "SELF-TEST PASSED: identity wiring holds AND the evaluator demonstrably fails on bad input."
exit 0
