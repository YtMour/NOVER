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
#>
[CmdletBinding()]
param(
  [switch]$SkipE2E
)

$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$serverDir = Join-Path $repoRoot 'server'
$frontendApps = @('web', 'console')
$npmCommand = if ($env:OS -eq 'Windows_NT') { 'npm.cmd' } else { 'npm' }
$curlCommand = if ($env:OS -eq 'Windows_NT') { 'curl.exe' } else { 'curl' }
$readyUrl = 'http://127.0.0.1:48080/readyz'

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

Write-Host 'NOVER 本地验证' -ForegroundColor Cyan
Write-Host ('  仓库：{0}' -f $repoRoot)
Write-Host ('  冒烟：{0}' -f $(if ($SkipE2E) { '跳过（-SkipE2E）' } else { '运行（需要 Docker 与已启动服务）' }))

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

Write-Host ''
if ($failed -gt 0) {
  Write-Host ('结果：{0} 项失败，{1} 项通过，{2} 项跳过。' -f $failed, $passed, $skipped) -ForegroundColor Red
  exit 1
}
Write-Host ('结果：全部通过（{0} 项通过，{1} 项跳过）。' -f $passed, $skipped) -ForegroundColor Green
exit 0
