# NOVER QA 冒烟套件：依次运行两套业务冒烟脚本，任一失败时以非零退出码结束。
#
# 用法（仓库根目录）：
#   .\scripts\qa\run-all.ps1
#   powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\qa\run-all.ps1
#
# 兼容 Windows PowerShell 5.1、PowerShell 7 和 Linux（pwsh -File）。文件保持 UTF-8 BOM + LF。

[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path

$suites = @(
  [pscustomobject]@{
    Name   = '版本权益与下载额度'
    Script = (Join-Path $PSScriptRoot 'version-entitlement-smoke.ps1')
  },
  [pscustomobject]@{
    Name   = '租户隔离与商户交付'
    Script = (Join-Path $PSScriptRoot 'tenant-delivery-smoke.ps1')
  },
  [pscustomobject]@{
    Name   = '运营能力（商品资料/退款/成员/审计）'
    Script = (Join-Path $PSScriptRoot 'operator-capabilities-smoke.ps1')
  }
)

$results = New-Object System.Collections.ArrayList

Write-Host ('NOVER 冒烟套件：共 {0} 项，仓库根目录 {1}' -f @($suites).Count, $repoRoot) -ForegroundColor Cyan

foreach ($suite in $suites) {
  Write-Host ''
  Write-Host ('=== 开始：{0} ===' -f $suite.Name) -ForegroundColor Cyan
  $timer = [System.Diagnostics.Stopwatch]::StartNew()
  $captured = New-Object System.Collections.ArrayList
  $threw = $false
  $errorMessage = ''

  try {
    & $suite.Script 2>&1 | ForEach-Object {
      $line = if ($_ -is [System.Management.Automation.ErrorRecord]) { $_.ToString() } else { "$_" }
      [void]$captured.Add($line)
      Write-Host $line
    }
  } catch {
    $threw = $true
    $errorMessage = $_.Exception.Message
    Write-Host ('[错误] {0}' -f $errorMessage) -ForegroundColor Red
  }
  $timer.Stop()

  $passed = -not $threw
  $detail = ''
  if ($threw) {
    $detail = $errorMessage
  } else {
    # 冒烟脚本以 JSON 输出断言结果；租户脚本失败时会同时抛异常，版本脚本只输出 pass=false。
    $jsonText = ($captured -join "`n")
    $passValue = $null
    try {
      $parsed = $jsonText | ConvertFrom-Json
      if ($parsed.PSObject.Properties.Name -contains 'pass') { $passValue = [bool]$parsed.pass }
    } catch {
      if ($jsonText -match '"pass"\s*:\s*(true|false)') { $passValue = ($matches[1] -eq 'true') }
    }
    if ($null -eq $passValue) {
      $passed = $true
      $detail = '（未返回 pass 字段，按脚本成功处理）'
    } else {
      $passed = $passValue
      if (-not $passed) { $detail = '断言未全部通过' }
    }
  }

  [void]$results.Add([pscustomobject]@{
    Name    = $suite.Name
    Passed  = $passed
    Seconds = $timer.Elapsed.TotalSeconds
    Detail  = $detail
  })
}

Write-Host ''
Write-Host '================ QA 冒烟汇总 ================' -ForegroundColor Cyan
$failed = 0
foreach ($result in $results) {
  $status = if ($result.Passed) { 'PASS' } else { 'FAIL' }
  $color = if ($result.Passed) { 'Green' } else { 'Red' }
  Write-Host ('  [{0}] {1}（{2:N1}s）{3}' -f $status, $result.Name, $result.Seconds, $result.Detail) -ForegroundColor $color
  if (-not $result.Passed) { $failed++ }
}

if ($failed -gt 0) {
  Write-Host ''
  Write-Host ('结果：{0} 项失败 / 共 {1} 项，请检查上方日志。' -f $failed, @($results).Count) -ForegroundColor Red
  exit 1
}

Write-Host ''
Write-Host ('结果：全部 {0} 项通过。' -f @($results).Count) -ForegroundColor Green
exit 0
