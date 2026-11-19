<#
.SYNOPSIS
  NOVER 本地一键验证：检查工具、Go vet/test、双端 npm 构建，可选端到端冒烟。
.DESCRIPTION
  在仓库根目录运行：
    .\scripts\verify.ps1            # 全量（需要 Docker 与已启动的服务栈）
    .\scripts\verify.ps1 -SkipE2E   # 跳过冒烟（无需 Docker）
  Windows PowerShell 5.1 可用：
    powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify.ps1 -SkipE2E
  兼容 Windows PowerShell 5.1、PowerShell 7 和 Linux（pwsh -File）。

  可选增强（默认行为不变）：
    -ReportPath <path>  把本次各步骤结果（名称/状态/耗时/详情/时间/仓库版本）汇总为 JSON 报告；
                        父目录必须已存在，否则直接失败（脚本不会自动创建目录）。
    -Integration        若设置了 TEST_DATABASE_URL，在 Go 单元测试后追加
                        go test -tags=integration ./...（server 目录）；
                        未设置则跳过并记录原因；失败计入总结果。

  示例：
    .\scripts\verify.ps1 -SkipE2E -ReportPath .\reports\verify.json
    $env:TEST_DATABASE_URL = 'postgres://nover:...@127.0.0.1:5433/nover_test?sslmode=disable'
    .\scripts\verify.ps1 -SkipE2E -Integration -ReportPath .\reports\verify.json
#>
[CmdletBinding()]
param(
  [switch]$SkipE2E,
  [switch]$Integration,
  [string]$ReportPath
)

$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$serverDir = Join-Path $repoRoot 'server'
$frontendApps = @('web', 'console')
$npmCommand = if ($env:OS -eq 'Windows_NT') { 'npm.cmd' } else { 'npm' }
$curlCommand = if ($env:OS -eq 'Windows_NT') { 'curl.exe' } else { 'curl' }
$readyUrl = 'http://127.0.0.1:48080/readyz'

# -ReportPath：父目录必须已存在（脚本不自动创建目录），否则直接失败。
$reportFullPath = $null
if (-not [string]::IsNullOrWhiteSpace($ReportPath)) {
  $candidate = $ReportPath
  if (-not [System.IO.Path]::IsPathRooted($candidate)) {
    $candidate = Join-Path (Get-Location).Path $candidate
  }
  $reportFullPath = [System.IO.Path]::GetFullPath($candidate)
  $reportParent = Split-Path -Parent $reportFullPath
  if ([string]::IsNullOrWhiteSpace($reportParent) -or -not (Test-Path -LiteralPath $reportParent -PathType Container)) {
    Write-Host ('[失败] 报告父目录不存在：{0}；请先创建目录（脚本不会自动创建）。' -f $reportParent) -ForegroundColor Red
    exit 1
  }
  if (Test-Path -LiteralPath $reportFullPath -PathType Container) {
    Write-Host ('[失败] -ReportPath 指向目录而不是文件：{0}' -f $reportFullPath) -ForegroundColor Red
    exit 1
  }
}

$results = New-Object System.Collections.ArrayList

function Add-Result([string]$Name, [string]$Status, [double]$Seconds, [string]$Detail = '') {
  [void]$script:results.Add([pscustomobject]@{
    Name    = $Name
    Status  = $Status
    Seconds = $Seconds
    Detail  = $Detail
  })
}

function Test-Tool([string]$Command) {
  return [bool](Get-Command $Command -ErrorAction SilentlyContinue)
}

function Invoke-Native {
  param(
    [string]$WorkingDirectory,
    [string]$FilePath,
    [string[]]$Arguments
  )
  Push-Location -LiteralPath $WorkingDirectory
  try {
    & $FilePath @Arguments
    $exitCode = $LASTEXITCODE
  } finally {
    Pop-Location
  }
  if ($exitCode -ne 0) {
    throw ('命令失败：{0} {1}（退出码 {2}）' -f $FilePath, ($Arguments -join ' '), $exitCode)
  }
}

function Invoke-Step {
  param([string]$Name, [scriptblock]$Body)
  Write-Host ''
  Write-Host ('==> {0}' -f $Name) -ForegroundColor Cyan
  $timer = [System.Diagnostics.Stopwatch]::StartNew()
  try {
    & $Body
    $timer.Stop()
    Write-Host ('[通过] {0}（{1:N1}s）' -f $Name, $timer.Elapsed.TotalSeconds) -ForegroundColor Green
    Add-Result $Name '通过' $timer.Elapsed.TotalSeconds
  } catch {
    $timer.Stop()
    Write-Host ('[失败] {0}：{1}' -f $Name, $_.Exception.Message) -ForegroundColor Red
    Add-Result $Name '失败' $timer.Elapsed.TotalSeconds $_.Exception.Message
  }
}

function Add-Skipped([string]$Name, [string]$Reason) {
  Write-Host ('[跳过] {0}：{1}' -f $Name, $Reason) -ForegroundColor Yellow
  Add-Result $Name '跳过' 0 $Reason
}

function Resolve-PowerShellExe {
  $fileName = if ($env:OS -eq 'Windows_NT') {
    if ($PSVersionTable.PSEdition -eq 'Core') { 'pwsh.exe' } else { 'powershell.exe' }
  } else {
    'pwsh'
  }
  $candidate = Join-Path $PSHOME $fileName
  if (Test-Path -LiteralPath $candidate) { return $candidate }
  throw ('无法定位当前 PowerShell 可执行文件（$PSHOME 中没有 {0}）。' -f $fileName)
}

function Get-RepoVersion([string]$Root) {
  # 仓库版本用于报告：优先 .env 的 APP_VERSION（构建元数据），回退 dev。
  $envPath = Join-Path $Root '.env'
  if (-not (Test-Path -LiteralPath $envPath -PathType Leaf)) { return 'dev' }
  foreach ($line in [System.IO.File]::ReadAllLines($envPath, [System.Text.Encoding]::UTF8)) {
    $trimmed = $line.Trim()
    if ($trimmed.Length -eq 0) { continue }
    if ($trimmed.StartsWith('#')) { continue }
    $index = $trimmed.IndexOf('=')
    if ($index -lt 1) { continue }
    if ($trimmed.Substring(0, $index).Trim() -ne 'APP_VERSION') { continue }
    $value = $trimmed.Substring($index + 1).Trim().Trim('"').Trim("'")
    if (-not [string]::IsNullOrWhiteSpace($value)) { return $value }
  }
  return 'dev'
}

function Get-GitCommitShort([string]$Root) {
  if (-not (Get-Command 'git' -ErrorAction SilentlyContinue)) { return 'unknown' }
  $output = & git -C $Root rev-parse --short=12 HEAD 2>$null
  if ($LASTEXITCODE -ne 0) { return 'unknown' }
  $first = @($output) | Select-Object -First 1
  if ($null -eq $first) { return 'unknown' }
  return ([string]$first).Trim()
}

Write-Host 'NOVER 本地验证' -ForegroundColor Cyan
Write-Host ('  仓库：{0}' -f $repoRoot)
Write-Host ('  冒烟：{0}' -f $(if ($SkipE2E) { '跳过（-SkipE2E）' } else { '运行（需要 Docker 与已启动服务）' }))
if ($null -ne $reportFullPath) {
  Write-Host ('  报告：{0}' -f $reportFullPath)
}
if ($Integration) {
  $integrationDbState = if ([string]::IsNullOrWhiteSpace([string]$env:TEST_DATABASE_URL)) { '未设置 TEST_DATABASE_URL（将跳过）' } else { 'TEST_DATABASE_URL 已设置' }
  Write-Host ('  集成测试：执行 go test -tags=integration（{0}）' -f $integrationDbState)
}

# ---------------------------------------------------------------------------
# 1. 工具检查
# ---------------------------------------------------------------------------
Write-Host ''
Write-Host '==> 检查本地工具' -ForegroundColor Cyan
$goAvailable = Test-Tool 'go'
$nodeAvailable = Test-Tool 'node'
$npmAvailable = Test-Tool $npmCommand
$dockerAvailable = Test-Tool 'docker'
$curlAvailable = Test-Tool $curlCommand

$toolChecks = @(
  @{ Name = 'go'; Available = $goAvailable; Required = $true },
  @{ Name = 'node'; Available = $nodeAvailable; Required = $true },
  @{ Name = $npmCommand; Available = $npmAvailable; Required = $true },
  @{ Name = 'docker'; Available = $dockerAvailable; Required = (-not $SkipE2E) },
  @{ Name = $curlCommand; Available = $curlAvailable; Required = (-not $SkipE2E) }
)
$missingTools = @()
foreach ($check in $toolChecks) {
  if ($check.Available) {
    Write-Host ('  [OK]     {0}' -f $check.Name) -ForegroundColor Green
  } elseif ($check.Required) {
    Write-Host ('  [缺失]   {0}' -f $check.Name) -ForegroundColor Red
    $missingTools += $check.Name
  } else {
    Write-Host ('  [可选]   {0}（已跳过，当前 -SkipE2E 不需要）' -f $check.Name) -ForegroundColor Yellow
  }
}
if ($missingTools.Count -gt 0) {
  Add-Result '检查本地工具' '失败' 0 ('缺少：' + ($missingTools -join ', '))
} else {
  Add-Result '检查本地工具' '通过' 0
}

# ---------------------------------------------------------------------------
# 2. Go 后端：vet + test
# ---------------------------------------------------------------------------
if ($goAvailable) {
  Invoke-Step 'Go vet（server）' { Invoke-Native $serverDir 'go' @('vet', './...') }
  Invoke-Step 'Go test（server）' { Invoke-Native $serverDir 'go' @('test', './...') }
} else {
  Add-Skipped 'Go vet（server）' '未找到 go'
  Add-Skipped 'Go test（server）' '未找到 go'
}

# 2.1 集成测试（仅 -Integration）：需要 TEST_DATABASE_URL 指向可丢弃数据库
if ($Integration) {
  $integrationDatabaseUrl = [string]$env:TEST_DATABASE_URL
  if (-not $goAvailable) {
    Add-Skipped 'Go test（integration）' '未找到 go'
  } elseif ([string]::IsNullOrWhiteSpace($integrationDatabaseUrl)) {
    Add-Skipped 'Go test（integration）' '未设置 TEST_DATABASE_URL；-Integration 已跳过（设置后重跑即可）'
  } else {
    Invoke-Step 'Go test（integration）' { Invoke-Native $serverDir 'go' @('test', '-tags=integration', './...') }
  }
}

# ---------------------------------------------------------------------------
# 3. 前端：web / console 构建
# ---------------------------------------------------------------------------
foreach ($app in $frontendApps) {
  if (-not ($nodeAvailable -and $npmAvailable)) {
    Add-Skipped ('前端构建：{0}' -f $app) '未找到 node 或 npm'
    continue
  }
  $appDir = Join-Path $repoRoot $app
  Invoke-Step ('前端安装依赖：{0}' -f $app) {
    if (Test-Path -LiteralPath (Join-Path $appDir 'node_modules')) {
      Write-Host '  已存在 node_modules，跳过 npm ci。'
    } else {
      Invoke-Native $appDir $npmCommand @('ci')
    }
  }
  Invoke-Step ('前端构建：{0}' -f $app) { Invoke-Native $appDir $npmCommand @('run', 'build') }
}

# ---------------------------------------------------------------------------
# 4. 端到端冒烟（默认运行，-SkipE2E 跳过）
# ---------------------------------------------------------------------------
if ($SkipE2E) {
  Add-Skipped '冒烟测试（端到端）' '已指定 -SkipE2E'
} else {
  Invoke-Step '冒烟测试（端到端）' {
    if (-not (Test-Path -LiteralPath (Join-Path $repoRoot '.env'))) {
      throw '缺少 .env；请先执行 Copy-Item .env.example .env，再执行 docker compose up -d --build。'
    }
    if (-not $curlAvailable) { throw ('未找到 {0}，无法探测 API 就绪状态。' -f $curlCommand) }

    Push-Location -LiteralPath $repoRoot
    try {
      & $curlCommand '-fsS' '--max-time' '3' $readyUrl | Out-Null
      $readyExit = $LASTEXITCODE
    } finally {
      Pop-Location
    }
    if ($readyExit -ne 0) {
      throw ('API 未就绪；请先执行 docker compose up -d --build，并确认 {0} 可访问。' -f $readyUrl)
    }

    $powerShellExe = Resolve-PowerShellExe
    $arguments = @('-NoProfile')
    if ($env:OS -eq 'Windows_NT') { $arguments += @('-ExecutionPolicy', 'Bypass') }
    $arguments += @('-File', (Join-Path $repoRoot 'scripts/qa/run-all.ps1'))

    Push-Location -LiteralPath $repoRoot
    try {
      & $powerShellExe @arguments
      $smokeExit = $LASTEXITCODE
    } finally {
      Pop-Location
    }
    if ($smokeExit -ne 0) { throw ('冒烟测试失败（退出码 {0}）。' -f $smokeExit) }
  }
}

# ---------------------------------------------------------------------------
# 5. 汇总
# ---------------------------------------------------------------------------
Write-Host ''
Write-Host '================ 验证汇总 ================' -ForegroundColor Cyan
$failed = 0
$skipped = 0
$passed = 0
foreach ($result in $results) {
  if ($result.Status -eq '通过') {
    $color = 'Green'; $passed++
  } elseif ($result.Status -eq '失败') {
    $color = 'Red'; $failed++
  } else {
    $color = 'Yellow'; $skipped++
  }
  Write-Host ('  [{0}] {1}（{2:N1}s）{3}' -f $result.Status, $result.Name, $result.Seconds, $result.Detail) -ForegroundColor $color
}

# ---------------------------------------------------------------------------
# 5.1 JSON 报告（仅 -ReportPath）
# ---------------------------------------------------------------------------
if ($null -ne $reportFullPath) {
  $reportStatus = if ($failed -gt 0) { '失败' } elseif ($skipped -gt 0) { '通过（含跳过）' } else { '通过' }
  $reportRows = @()
  foreach ($result in $results) {
    $reportRows += [ordered]@{
      name    = $result.Name
      status  = $result.Status
      seconds = [math]::Round([double]$result.Seconds, 3)
      detail  = [string]$result.Detail
    }
  }
  $report = [ordered]@{
    report_version   = 1
    tool             = 'scripts/verify.ps1'
    created_at       = [System.DateTime]::UtcNow.ToString('o', [System.Globalization.CultureInfo]::InvariantCulture)
    created_at_local = [System.DateTime]::Now.ToString('o', [System.Globalization.CultureInfo]::InvariantCulture)
    host             = [System.Environment]::MachineName
    platform         = [ordered]@{
      os         = $env:OS
      ps_version = $PSVersionTable.PSVersion.ToString()
      ps_edition = [string]$PSVersionTable.PSEdition
    }
    repo             = [ordered]@{
      root       = $repoRoot
      version    = (Get-RepoVersion $repoRoot)
      git_commit = (Get-GitCommitShort $repoRoot)
    }
    options          = [ordered]@{
      skip_e2e    = [bool]$SkipE2E
      integration = [bool]$Integration
    }
    summary          = [ordered]@{
      status  = $reportStatus
      passed  = $passed
      failed  = $failed
      skipped = $skipped
      total   = $results.Count
    }
    results          = @($reportRows)
  }
  try {
    $reportJson = ($report | ConvertTo-Json -Depth 8) -replace "`r`n", "`n"
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($reportFullPath, ($reportJson + "`n"), $utf8NoBom)
    Write-Host ''
    Write-Host ('报告已写入：{0}' -f $reportFullPath) -ForegroundColor Cyan
  } catch {
    Write-Host ''
    Write-Host ('[失败] 写入报告失败：{0}' -f $_.Exception.Message) -ForegroundColor Red
    exit 1
  }
}

Write-Host ''
if ($failed -gt 0) {
  Write-Host ('结果：{0} 项失败，{1} 项通过，{2} 项跳过。' -f $failed, $passed, $skipped) -ForegroundColor Red
  exit 1
}
Write-Host ('结果：全部通过（{0} 项通过，{1} 项跳过）。' -f $passed, $skipped) -ForegroundColor Green
exit 0
