param(
  [switch]$Force,
  [switch]$Restart
)

$serviceName = "ExampleAgent"
$logPath = Join-Path $env:ProgramData "ExampleAgent\repair.log"

Stop-Service -Name $serviceName -Force -ErrorAction SilentlyContinue
Remove-Item -Path (Join-Path $env:ProgramFiles "ExampleAgent\cache") -Recurse -Force -ErrorAction SilentlyContinue
Start-Service -Name $serviceName

"Repair completed" | Out-File -FilePath $logPath -Append

if ($Restart -or $Force) {
  Restart-Computer -Force
}
