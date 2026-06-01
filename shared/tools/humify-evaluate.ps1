[CmdletBinding()]
param(
  [string]$ActualDir = (Join-Path $PSScriptRoot "..\actual"),
  [string]$ExpectedDir = (Join-Path $PSScriptRoot "..\expected"),
  [string]$OutputPath = (Join-Path $PSScriptRoot "..\..\.humify-runs\HUMIFY-SCORE.md"),
  [int]$ReadyThreshold = 0,
  [switch]$FailBelowThreshold
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# Cross-platform parity: this mirrors shared/tools/humify_evaluate.py + humify_lib.py.
# Shared parser/scoring helpers live in humify-lib.ps1 (the analogue of humify_lib.py).
# The rubric lives in humify-fixtures.json (single source of truth, read by BOTH tools).
# The ExpectedDir is load-bearing: a fixture's classification, machine-shaped confidence
# and finding-vs-no-finding expectation are DERIVED FROM the matching expected file and
# the actual must AGREE with them. A missing expected file is reported, not silently passed.
# See shared/tools/humify-selftest.ps1 / humify_selftest.py for the falsifiable proof.

. (Join-Path $PSScriptRoot "humify-lib.ps1")

$rubricPath = Join-Path $PSScriptRoot "humify-fixtures.json"
$rubric = (Get-Content -LiteralPath $rubricPath -Raw) | ConvertFrom-Json
$classifications = @($rubric.classifications)
$confidences = @($rubric.confidences)
$fixtures = @($rubric.fixtures)
$maxPer = if ($rubric.PSObject.Properties['maxPerFixture']) { [int]$rubric.maxPerFixture } else { 3 }
$readyRatio = if ($rubric.PSObject.Properties['readyRatio']) { [double]$rubric.readyRatio } else { 0.86 }

$results = @()
$warnings = New-Object System.Collections.Generic.List[string]
$maxScore = $fixtures.Count * $maxPer
if ($ReadyThreshold -le 0) {
  $ReadyThreshold = [int][Math]::Ceiling($maxScore * $readyRatio)
}

foreach ($fixture in $fixtures) {
  $actualPath = Join-Path $ActualDir $fixture.file
  $expectedPath = Join-Path $ExpectedDir $fixture.file
  $actualText = Read-TextFile -Path $actualPath
  $expectedText = Read-TextFile -Path $expectedPath

  if ($null -eq $actualText) {
    $warnings.Add("MISSING actual: $actualPath")
    $results += [pscustomobject]@{
      Fixture = $fixture.id
      Score = 0
      Expected = "(none)"
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

  $expClass = Get-Classification -Text $expectedText -Classifications $classifications
  $expConf = Get-Confidence -Text $expectedText -Confidences $confidences
  $expHasFinding = Test-HasFinding -Text $expectedText

  $actClass = Get-Classification -Text $actualText -Classifications $classifications
  $actConf = Get-Confidence -Text $actualText -Confidences $confidences
  $actHasFinding = Test-HasFinding -Text $actualText

  $classificationOk = ($null -ne $expClass) -and ($expClass -eq $actClass)
  $confidenceOk = ($null -ne $expConf) -and ($expConf -eq $actConf)
  if ($expHasFinding) {
    $findingOk = $actHasFinding -and (Test-HasAuditEvidence -Text $actualText)
  } else {
    $findingOk = ($null -ne $expectedText) -and (-not $actHasFinding)
  }
  $requiredOk = Test-ContainsAny -Text $actualText -Needles @($fixture.requiredNeedles)
  $safeOk = Test-ContainsAny -Text $actualText -Needles @($fixture.safeActionNeedles)

  $genExcl = [bool]($fixture.PSObject.Properties['generatedExclusion'] -and $fixture.generatedExclusion)
  $unsafeClaim = Test-UnsafeGeneratedClaim -Text $actualText
  $unsafeGenerated = $genExcl -and $actHasFinding
  $unsafe = $unsafeClaim -or $unsafeGenerated

  $checks = @($classificationOk, $confidenceOk, $findingOk, ($requiredOk -and $safeOk))
  $score = Get-AuditScore -Checks $checks -UnsafeFailure $unsafe

  $expClassDisplay = if ($null -ne $expClass) { $expClass } else { "(none)" }
  $actClassDisplay = if ($null -ne $actClass) { $actClass } else { "(none)" }
  $expConfDisplay = if ($null -ne $expConf) { $expConf } else { "(none)" }
  $actConfDisplay = if ($null -ne $actConf) { $actConf } else { "(none)" }
  $notes.Add("classification=$classificationOk(exp=$expClassDisplay,act=$actClassDisplay)")
  $notes.Add("confidence=$confidenceOk(exp=$expConfDisplay,act=$actConfDisplay)")
  $notes.Add("finding_or_exclusion=$findingOk")
  $notes.Add("safe_next_action=$($requiredOk -and $safeOk)")
  if ($unsafeClaim) { $notes.Add("unsafe_generated_origin_claim=true") }
  if ($unsafeGenerated) { $notes.Add("generated_file_was_scored=true") }

  $status = if ($score -eq 3) { "Pass" } elseif ($score -ge 1) { "Partial" } else { "Fail" }

  $results += [pscustomobject]@{
    Fixture = $fixture.id
    Score = $score
    Expected = $expClassDisplay
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

$identity = Test-SameDir -A $ActualDir -B $ExpectedDir
$readiness = Get-Readiness -TotalScore $totalScore -MaxScore $maxScore -Kind "audit" -IdentitySelfTest $identity
$timestamp = (Get-Date).ToString("yyyy-MM-ddTHH:mm:ssK")

$lines = New-Object System.Collections.Generic.List[string]
$lines.Add("# Humify Score")
$lines.Add("")
$lines.Add("Generated: $timestamp")
$lines.Add("")
$lines.Add("Actual dir: ``$ActualDir``")
$lines.Add("Expected dir: ``$ExpectedDir``")
$lines.Add("")
$lines.Add("## Summary")
$lines.Add("")
$lines.Add("Total score: **$totalScore / $maxScore**")
$lines.Add("")
$lines.Add("Readiness: **$readiness**")
$lines.Add("")
$lines.Add("| Fixture | Score | Status | Notes |")
$lines.Add("| --- | ---: | --- | --- |")

foreach ($result in $results) {
  $notes = $result.Notes.Replace("|", "\|")
  $lines.Add("| $($result.Fixture) | $($result.Score) / 3 | $($result.Status) | $notes |")
}

$lines.Add("")
$lines.Add("## Interpretation")
$lines.Add("")
$lines.Add("- 86-100%: Ready to use on a real repo.")
$lines.Add("- 67-85%: Usable with human review.")
$lines.Add("- 40-66%: Needs framework tuning.")
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
Write-Output "Humify score: $totalScore / $maxScore - $readiness"
Write-Output "Report: $OutputPath"

if ($FailBelowThreshold -and $totalScore -lt $ReadyThreshold) {
  exit 1
}
