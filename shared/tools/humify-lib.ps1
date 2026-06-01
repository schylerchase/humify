# Shared helpers for the PowerShell Humify tooling (mirror of shared/tools/humify_lib.py).
#
# Dot-source this file (`. (Join-Path $PSScriptRoot "humify-lib.ps1")`) from the evaluators
# and the self-test so Windows (pwsh) and macOS/Linux (python3) get identical behavior.
# The calibration rubric itself lives in humify-fixtures.json / humify-plan-fixtures.json,
# which BOTH the .ps1 and .py tools read, so there is a single source of truth.
#
# This file defines functions only — it has no side effects when sourced.

function Read-TextFile {
  param([string]$Path)

  if (-not (Test-Path -LiteralPath $Path)) {
    return $null
  }

  return [System.IO.File]::ReadAllText((Resolve-Path -LiteralPath $Path))
}

function Test-ContainsAny {
  # True if any needle appears in text (case-insensitive substring).
  param(
    [AllowNull()][string]$Text,
    [string[]]$Needles
  )

  if ([string]::IsNullOrWhiteSpace($Text)) {
    return $false
  }

  foreach ($needle in $Needles) {
    if ($Text.IndexOf($needle, [StringComparison]::OrdinalIgnoreCase) -ge 0) {
      return $true
    }
  }

  return $false
}

function Test-AllNeedles {
  # True if every needle appears in text.
  param(
    [AllowNull()][string]$Text,
    [string[]]$Needles
  )

  foreach ($needle in $Needles) {
    if (-not (Test-ContainsAny -Text $Text -Needles @($needle))) {
      return $false
    }
  }

  return $true
}

function Test-Regex {
  param(
    [AllowNull()][string]$Text,
    [string]$Pattern
  )

  if ([string]::IsNullOrWhiteSpace($Text)) {
    return $false
  }

  return [regex]::IsMatch($Text, $Pattern, [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)
}

function Get-RowCells {
  # Normalized (markdown/quote-stripped, lower-cased) cells of a table row.
  param([string]$Line)

  $trim = [char[]]@('`', '*', '_', '"', "'", ' ')
  return @($Line.Split([char]'|') | ForEach-Object {
    $_.Trim().Trim($trim).Trim().ToLowerInvariant()
  })
}

function Get-Classification {
  # Return the audit's stated classification verdict (mirror detect_classification).
  # Ordered most- to least-specific so a label that is merely *mentioned* (ruled out)
  # cannot hijack the result. Character classes are single, non-overlapping quantifiers
  # (no "\**[ \t]*" adjacency) so a long whitespace run cannot cause catastrophic
  # backtracking (ReDoS).
  param(
    [AllowNull()][string]$Text,
    [string[]]$Classifications
  )

  if ([string]::IsNullOrEmpty($Text)) {
    return $null
  }

  $verdictLabels = @("overall", "classification", "verdict", "final", "result")
  $byLower = @{}
  foreach ($c in $Classifications) { $byLower[$c.ToLowerInvariant()] = $c }
  $lines = [regex]::Split($Text, '\r?\n')

  # 1. Verdict row: first non-empty cell is a verdict label, and a later cell on
  #    that row names a classification.
  foreach ($line in $lines) {
    if ($line.IndexOf('|') -lt 0) { continue }
    $cells = Get-RowCells $line
    $nonEmpty = @($cells | Where-Object { $_ -ne '' })
    if ($nonEmpty.Count -eq 0) { continue }
    $labelWords = @([regex]::Matches($nonEmpty[0], '[a-z]+') | ForEach-Object { $_.Value })
    $isVerdict = $false
    foreach ($v in $verdictLabels) {
      if ($labelWords -contains $v) { $isVerdict = $true; break }
    }
    if ($isVerdict) {
      foreach ($cell in $cells) {
        if ($byLower.ContainsKey($cell)) { return $byLower[$cell] }
      }
    }
  }

  # 2. Prose verdict line(s): explicit "<verdict label> <sep> <classification>".
  #    When several are stated (multi-section audit), return the highest-priority
  #    classification in rubric order, matching the legacy first-phrase ordering so
  #    the result does not depend on which line comes first.
  $alt = ($Classifications | ForEach-Object { [regex]::Escape($_) }) -join '|'
  $prosePat = '(?im)^[ \t|*+-]*(?:overall[ \t]+classification|final[ \t]+classification|overall|final|classification|verdict|result)[ \t*_`]*[:=|-]+[ \t*_`]*(' + $alt + ')\b'
  $hits = @{}
  foreach ($m in [regex]::Matches($Text, $prosePat)) {
    $hits[$m.Groups[1].Value.ToLowerInvariant()] = $true
  }
  if ($hits.Count -gt 0) {
    foreach ($c in $Classifications) {
      if ($hits.ContainsKey($c.ToLowerInvariant())) { return $c }
    }
  }

  # 3. Any table cell equal to a classification (legacy).
  foreach ($line in $lines) {
    if ($line.IndexOf('|') -lt 0) { continue }
    foreach ($cell in (Get-RowCells $line)) {
      if ($byLower.ContainsKey($cell)) { return $byLower[$cell] }
    }
  }

  # 4. First classification phrase appearing anywhere (legacy).
  $low = $Text.ToLowerInvariant()
  foreach ($c in $Classifications) {
    if ($low.Contains($c.ToLowerInvariant())) { return $c }
  }

  return $null
}

function Get-Confidence {
  # Extract the stated machine-shaped confidence value (mirror detect_confidence).
  param(
    [AllowNull()][string]$Text,
    [string[]]$Confidences
  )

  if ([string]::IsNullOrWhiteSpace($Text)) {
    return $null
  }

  $m = [regex]::Match($Text, 'machine[- ]?shaped\s+confidence\s*(?:is|:|=|\||-|–|—)?\s*[`*_"'']*\s*(Not applicable|None|Low|Medium|High)\b', [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)
  if (-not $m.Success) {
    return $null
  }

  $val = $m.Groups[1].Value.Trim()
  foreach ($c in $Confidences) {
    if ($c -ieq $val) { return $c }
  }

  return $null
}

function Test-HasFinding {
  # True if the audit contains at least one finding header like "## H001".
  param([AllowNull()][string]$Text)
  return (Test-Regex -Text $Text -Pattern "##\s+H\d+")
}

function Test-FieldAtLineStart {
  # True if $Label (e.g. "File:") begins a line. Leading whitespace, table pipes,
  # list bullets and markdown emphasis are folded into one non-overlapping class so a
  # long run cannot trigger catastrophic backtracking. Anchoring at the line start
  # prevents substring false positives (profile:->File:, outlines:->Lines:, prefix:->Fix:).
  param(
    [AllowNull()][string]$Text,
    [string]$Label
  )

  if ([string]::IsNullOrWhiteSpace($Text)) {
    return $false
  }

  $pat = '(?im)^[ \t|*+-]*' + [regex]::Escape($Label)
  return [regex]::IsMatch($Text, $pat)
}

function Test-HasAuditEvidence {
  # True if a finding carries the required evidence fields, each as a line label.
  param([AllowNull()][string]$Text)

  if ([string]::IsNullOrEmpty($Text)) {
    return $false
  }

  $hasLine = (Test-FieldAtLineStart -Text $Text -Label "Line:") -or (Test-FieldAtLineStart -Text $Text -Label "Lines:")
  return (
    (Test-FieldAtLineStart -Text $Text -Label "File:") -and
    $hasLine -and
    (Test-FieldAtLineStart -Text $Text -Label "Symptom:") -and
    (Test-FieldAtLineStart -Text $Text -Label "Causal chain:") -and
    (Test-FieldAtLineStart -Text $Text -Label "Fix:")
  )
}

function Test-UnsafeGeneratedClaim {
  # True if the audit makes a bare positive AI/machine-generated ORIGIN claim with no
  # negation cue before it in the sentence (mirror unsafe_generated_claim).
  param([AllowNull()][string]$Text)

  if ([string]::IsNullOrWhiteSpace($Text)) {
    return $false
  }

  foreach ($sentence in [regex]::Split($Text, '(?<=[.!?])\s+|\n+')) {
    $cm = [regex]::Match($sentence, '\b(is|was|appears to be|likely)\s+(ai|machine)[ -]?generated\b', [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)
    if ($cm.Success) {
      $before = $sentence.Substring(0, $cm.Index)
      if (-not [regex]::IsMatch($before, '\b(no|not|n''t|never|without|cannot|rather than|ruled?\s+out|no\s+evidence)\b', [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)) {
        return $true
      }
    }
  }

  return $false
}

function Get-AuditScore {
  # Map passed-check count to a 0-3 score; any unsafe failure forces 0 (audit grid = 4 checks).
  param(
    [bool[]]$Checks,
    [bool]$UnsafeFailure
  )

  if ($UnsafeFailure) {
    return 0
  }

  $passed = 0
  foreach ($check in $Checks) {
    if ($check) { $passed++ }
  }

  if ($passed -ge 4) { return 3 }
  if ($passed -eq 3) { return 2 }
  if ($passed -eq 2) { return 1 }
  return 0
}

function Get-PlanScore {
  # Plan grid is 5 checks: 5 -> 3, 4 -> 2, >=2 -> 1, else 0.
  param([bool[]]$Checks)

  $passed = 0
  foreach ($check in $Checks) {
    if ($check) { $passed++ }
  }

  if ($passed -ge 5) { return 3 }
  if ($passed -eq 4) { return 2 }
  if ($passed -ge 2) { return 1 }
  return 0
}

function Get-Readiness {
  # Honest readiness band (mirror readiness_label). An identity self-test (actual dir ==
  # expected dir) is forced to the maximum, so it must NOT be reported as real-repo accuracy.
  param(
    [int]$TotalScore,
    [int]$MaxScore,
    [ValidateSet("audit", "plan")][string]$Kind,
    [bool]$IdentitySelfTest
  )

  if ($IdentitySelfTest) {
    return "Evaluator wiring OK (identity self-test — not a measure of real-repo accuracy)"
  }

  $ratio = if ($MaxScore -gt 0) { $TotalScore / $MaxScore } else { 0 }
  if ($Kind -eq "plan") {
    if ($ratio -ge 0.86) { return "Plan calibration ready" }
    if ($ratio -ge 0.67) { return "Usable with human review" }
    if ($ratio -ge 0.40) { return "Needs planning guidance tuning" }
    return "Not reliable yet"
  }

  if ($ratio -ge 0.86) { return "Ready to use on a real repo" }
  if ($ratio -ge 0.67) { return "Usable with human review" }
  if ($ratio -ge 0.40) { return "Needs framework tuning" }
  return "Not reliable yet"
}

function Test-SameDir {
  param([string]$A, [string]$B)

  try {
    $ra = (Resolve-Path -LiteralPath $A -ErrorAction Stop).Path
    $rb = (Resolve-Path -LiteralPath $B -ErrorAction Stop).Path
    return ([System.IO.Path]::GetFullPath($ra).TrimEnd('\', '/') -ieq [System.IO.Path]::GetFullPath($rb).TrimEnd('\', '/'))
  } catch {
    return $false
  }
}

function Get-PlanChecks {
  # The five plan criteria, mirroring humify_evaluate_plans.py plan_checks().
  param(
    [AllowNull()][string]$Text,
    $ShapeNeedles,
    [string]$UnitHeaderPattern,
    $UnitFieldNeedles,
    $TestsFirstNeedles,
    $RequiredNeedles,
    $FirstSliceNeedles
  )

  return @(
    (Test-AllNeedles -Text $Text -Needles @($ShapeNeedles)),
    (Test-Regex -Text $Text -Pattern $UnitHeaderPattern),
    (Test-AllNeedles -Text $Text -Needles @($UnitFieldNeedles)),
    ((Test-ContainsAny -Text $Text -Needles @($RequiredNeedles)) -and (Test-ContainsAny -Text $Text -Needles @($FirstSliceNeedles))),
    (Test-ContainsAny -Text $Text -Needles @($TestsFirstNeedles))
  )
}
