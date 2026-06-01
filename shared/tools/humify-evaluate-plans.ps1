[CmdletBinding()]
param(
  [string]$ActualPlanDir = (Join-Path $PSScriptRoot "..\actual-plans"),
  [string]$ExpectedPlanDir = (Join-Path $PSScriptRoot "..\expected-plans"),
  [string]$OutputPath = (Join-Path $PSScriptRoot "..\..\.humify-runs\HUMIFY-PLAN-SCORE.md"),
  [int]$ReadyThreshold = 0,
  [switch]$FailBelowThreshold
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# Cross-platform parity: mirrors shared/tools/humify_evaluate_plans.py. Shared helpers live
# in humify-lib.ps1. The rubric lives in humify-plan-fixtures.json (single source of truth).
# The ExpectedPlanDir is load-bearing: each criterion is credited only when the actual plan
# AND the expected baseline both establish it, so a missing/hollowed expected file lowers the
# score instead of silently passing. See shared/tools/humify-selftest.ps1 / humify_selftest.py.

. (Join-Path $PSScriptRoot "humify-lib.ps1")

$rubricPath = Join-Path $PSScriptRoot "humify-plan-fixtures.json"
$rubric = (Get-Content -LiteralPath $rubricPath -Raw) | ConvertFrom-Json
$shapeNeedles = @($rubric.shapeNeedles)
$unitHeaderPattern = [string]$rubric.unitHeaderPattern
$unitFieldNeedles = @($rubric.unitFieldNeedles)
$testsFirstNeedles = @($rubric.testsFirstNeedles)
$plans = @($rubric.plans)
$maxPer = if ($rubric.PSObject.Properties['maxPerPlan']) { [int]$rubric.maxPerPlan } else { 3 }
$readyRatio = if ($rubric.PSObject.Properties['readyRatio']) { [double]$rubric.readyRatio } else { 0.86 }

$results = @()
$warnings = New-Object System.Collections.Generic.List[string]
$maxScore = $plans.Count * $maxPer
if ($ReadyThreshold -le 0) {
  $ReadyThreshold = [int][Math]::Ceiling($maxScore * $readyRatio)
}

$labels = @("shape", "unit_header", "unit_fields", "scenario_and_first_slice", "tests_first")

foreach ($plan in $plans) {
  $actualPath = Join-Path $ActualPlanDir $plan.file
  $expectedPath = Join-Path $ExpectedPlanDir $plan.file
  $actualText = Read-TextFile -Path $actualPath
  $expectedText = Read-TextFile -Path $expectedPath

  if ($null -eq $actualText) {
    $warnings.Add("MISSING actual: $actualPath")
    $results += [pscustomobject]@{
      Plan = $plan.id
      Score = 0
      Status = "Missing actual"
      Notes = "Create $actualPath"
      ActualPath = $actualPath
      ExpectedPath = $expectedPath
    }
    continue
  }

  $notes = New-Object System.Collections.Generic.List[string]
  if ($null -eq $expectedText) {
    $warnings.Add("MISSING expected (cannot calibrate): $expectedPath")
    $notes.Add("expected_file_missing=true")
  }

  $act = Get-PlanChecks -Text $actualText -ShapeNeedles $shapeNeedles -UnitHeaderPattern $unitHeaderPattern -UnitFieldNeedles $unitFieldNeedles -TestsFirstNeedles $testsFirstNeedles -RequiredNeedles $plan.requiredNeedles -FirstSliceNeedles $plan.firstSliceNeedles
  if ($null -ne $expectedText) {
    $exp = Get-PlanChecks -Text $expectedText -ShapeNeedles $shapeNeedles -UnitHeaderPattern $unitHeaderPattern -UnitFieldNeedles $unitFieldNeedles -TestsFirstNeedles $testsFirstNeedles -RequiredNeedles $plan.requiredNeedles -FirstSliceNeedles $plan.firstSliceNeedles
  } else {
    $exp = @($false, $false, $false, $false, $false)
  }

  $checks = @()
  for ($i = 0; $i -lt $act.Count; $i++) {
    $checks += ($act[$i] -and $exp[$i])
  }

  $score = Get-PlanScore -Checks $checks
  $status = if ($score -eq 3) { "Pass" } elseif ($score -ge 1) { "Partial" } else { "Fail" }

  for ($i = 0; $i -lt $labels.Count; $i++) {
    $notes.Add("$($labels[$i])=$($checks[$i])(exp=$($exp[$i]),act=$($act[$i]))")
  }

  $results += [pscustomobject]@{
    Plan = $plan.id
    Score = $score
    Status = $status
    Notes = ($notes -join "; ")
    ActualPath = $actualPath
    ExpectedPath = $expectedPath
  }
}

$totalScore = ($results | Measure-Object -Property Score -Sum).Sum
if ($null -eq $totalScore) {
  $totalScore = 0
}

$identity = Test-SameDir -A $ActualPlanDir -B $ExpectedPlanDir
$readiness = Get-Readiness -TotalScore $totalScore -MaxScore $maxScore -Kind "plan" -IdentitySelfTest $identity
$timestamp = (Get-Date).ToString("yyyy-MM-ddTHH:mm:ssK")

$lines = New-Object System.Collections.Generic.List[string]
$lines.Add("# Humify Plan Score")
$lines.Add("")
$lines.Add("Generated: $timestamp")
$lines.Add("")
$lines.Add("Actual plan dir: ``$ActualPlanDir``")
$lines.Add("Expected plan dir: ``$ExpectedPlanDir``")
$lines.Add("")
$lines.Add("## Summary")
$lines.Add("")
$lines.Add("Total score: **$totalScore / $maxScore**")
$lines.Add("")
$lines.Add("Readiness: **$readiness**")
$lines.Add("")
$lines.Add("| Plan | Score | Status | Notes |")
$lines.Add("| --- | ---: | --- | --- |")

foreach ($result in $results) {
  $notes = $result.Notes.Replace("|", "\|")
  $lines.Add("| $($result.Plan) | $($result.Score) / 3 | $($result.Status) | $notes |")
}

$lines.Add("")
$lines.Add("## Interpretation")
$lines.Add("")
$lines.Add("- 86-100%: Plan calibration ready.")
$lines.Add("- 67-85%: Usable with human review.")
$lines.Add("- 40-66%: Needs planning guidance tuning.")
$lines.Add("- 0-39%: Not reliable yet.")
$lines.Add("")
$lines.Add("Note: an identity self-test (actual dir == expected dir) is forced to the maximum score and only proves evaluator wiring, not real-repo accuracy.")

$outputParent = Split-Path -Parent $OutputPath
if (-not [string]::IsNullOrWhiteSpace($outputParent) -and -not (Test-Path -LiteralPath $outputParent)) {
  New-Item -ItemType Directory -Path $outputParent | Out-Null
}

[System.IO.File]::WriteAllLines($OutputPath, $lines)

foreach ($w in $warnings) {
  Write-Warning $w
}
Write-Output "Humify plan score: $totalScore / $maxScore - $readiness"
Write-Output "Report: $OutputPath"

if ($FailBelowThreshold -and $totalScore -lt $ReadyThreshold) {
  exit 1
}
