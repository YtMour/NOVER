#Requires -Version 5.1

<#
.SYNOPSIS
  NOVER 本地升级与回滚：记录当前镜像 → 备份 → 构建并启动 → 等待 /readyz → 版本/迁移摘要；-Rollback 用 :previous 镜像标签回滚。

.DESCRIPTION
  在仓库根目录执行（Windows PowerShell 5.1 / PowerShell 7 / Linux pwsh 均可）：

    # 1) 演练：只打印预检结果与将执行的命令，不修改镜像、容器与数据
    .\scripts\ops\upgrade.ps1 -DryRun

    # 2) 正式升级（默认先备份，再 docker compose build + up -d）
    .\scripts\ops\upgrade.ps1

    # 3) 只重建后端 api（不改动 web / console）
    .\scripts\ops\upgrade.ps1 -BackendOnly

    # 4) 回滚到升级前的 :previous 镜像（只 retag + up -d，不 build）
    .\scripts\ops\upgrade.ps1 -Rollback

  升级步骤：
    1. 预检：仓库根、compose 文件、.env、docker、git HEAD 与工作区状态（dirty 只警告）。
    2. 记录当前 api/web/console 容器镜像，写入 <OutputDir>/upgrade-<timestamp>.json，
       并把镜像打上 nover-api:previous / nover-web:previous / nover-console:previous 标签（仅记录，不删除镜像）。
    3. 备份：默认调用 scripts/ops/backup.ps1 -OutputDir <OutputDir>；-SkipBackup 跳过。
    4. 构建并启动：默认 docker compose build + docker compose up -d；
       -BackendOnly 时 docker compose up -d --build api。
    5. 等待 /readyz（最多 -TimeoutSeconds 秒），读取 /api/v1/meta 与运营 system/status
       （system/status 需要 .env 中管理员账号登录，失败只警告），输出升级摘要。
    6. -Rollback：把 :previous 标签指回升级记录里的镜像引用，docker compose up -d（不 build），
       等待就绪并输出回滚摘要；没有任何 :previous 镜像时退出码 2。

  安全边界：不执行 docker compose down / rm，不删除卷，不删除镜像，不覆盖历史备份；任何失败返回非零。
  -DryRun 只打印预检结果与计划命令，不调用 docker、不写文件。
  文件保持 UTF-8 BOM + LF（Windows PowerShell 5.1 中文兼容）。

  退出码：
    0 成功（或 -DryRun 完成）
    1 预检 / 备份 / 构建 / 启动 / 就绪检查失败
    2 -Rollback 找不到可用的 :previous 镜像
#>
[CmdletBinding()]
param(
  [switch]$DryRun,
  [switch]$SkipBackup,
  [switch]$BackendOnly,
  [switch]$Rollback,
  [ValidateRange(1, 3600)][int]$TimeoutSeconds = 180,
  [string]$OutputDir = './backups',
  [string]$ComposeFile
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$script:ToolVersion = '0.5.0'
$script:ScriptName = 'scripts/ops/upgrade.ps1'

# ---------------------------------------------------------------------------
# 通用输出
# ---------------------------------------------------------------------------
function Write-Step([string]$Message) {
  Write-Host ''
  Write-Host ('==> {0}' -f $Message) -ForegroundColor Cyan
}

function Write-Info([string]$Message) {
  Write-Host ('    {0}' -f $Message)
}

function Write-Ok([string]$Message) {
  Write-Host ('    [OK] {0}' -f $Message) -ForegroundColor Green
}

function Write-WarnLine([string]$Message) {
  Write-Host ('    [警告] {0}' -f $Message) -ForegroundColor Yellow
}

function Write-ErrorLine([string]$Message) {
  Write-Host ('    [错误] {0}' -f $Message) -ForegroundColor Red
}

function Get-UtcNowString {
  return [System.DateTime]::UtcNow.ToString('o', [System.Globalization.CultureInfo]::InvariantCulture)
}

function Get-LocalNowString {
  return [System.DateTime]::Now.ToString('o', [System.Globalization.CultureInfo]::InvariantCulture)
}

# ---------------------------------------------------------------------------
# 路径 / 环境 / 参数解析
# ---------------------------------------------------------------------------
function Resolve-FullPath([string]$Path, [string]$BasePath) {
  if ([string]::IsNullOrWhiteSpace($Path)) { throw '路径不能为空。' }
  $candidate = $Path
  if (-not [System.IO.Path]::IsPathRooted($candidate)) {
    $candidate = Join-Path $BasePath $candidate
  }
  return [System.IO.Path]::GetFullPath($candidate)
}

function Read-DotEnv([string]$Path) {
  $map = @{}
  if (-not (Test-Path -LiteralPath $Path)) { return $map }
  foreach ($line in [System.IO.File]::ReadAllLines($Path, [System.Text.Encoding]::UTF8)) {
    $trimmed = $line.Trim()
    if ($trimmed.Length -eq 0) { continue }
    if ($trimmed.StartsWith('#')) { continue }
    $index = $trimmed.IndexOf('=')
    if ($index -lt 1) { continue }
    $key = $trimmed.Substring(0, $index).Trim()
    $value = $trimmed.Substring($index + 1).Trim()
    if ($value.Length -ge 2) {
      $doubleQuoted = $value.StartsWith('"') -and $value.EndsWith('"')
      $singleQuoted = $value.StartsWith("'") -and $value.EndsWith("'")
      if ($doubleQuoted -or $singleQuoted) {
        $value = $value.Substring(1, $value.Length - 2)
      }
    }
    $map[$key] = $value
  }
  return $map
}

function Get-EnvOrDefault([hashtable]$Map, [string]$Key, [string]$Default) {
  if ($Map.ContainsKey($Key)) {
    $value = [string]$Map[$Key]
    if (-not [string]::IsNullOrWhiteSpace($value)) { return $value.Trim() }
  }
  return $Default
}

function Get-ComposeProjectName([string]$ComposePath) {
  $composeDir = Split-Path -Parent $ComposePath
  $leaf = Split-Path -Leaf $composeDir
  $name = $leaf.ToLowerInvariant()
  $name = [regex]::Replace($name, '[^a-z0-9_-]', '')
  $name = [regex]::Replace($name, '^[^a-z0-9]+', '')
  if ([string]::IsNullOrWhiteSpace($name)) { return 'nover' }
  return $name
}

function Get-NormalizedImageRef([string]$Reference) {
  if ([string]::IsNullOrWhiteSpace($Reference)) { return $Reference }
  if ($Reference.Contains('@')) { return $Reference }
  $segments = $Reference -split '/'
  $last = $segments[$segments.Length - 1]
  if ($last.Contains(':')) { return $Reference }
  return ($Reference + ':latest')
}

# ---------------------------------------------------------------------------
# 进程执行（PowerShell 5.1 兼容）
# ---------------------------------------------------------------------------
function Format-CommandLine([string]$FilePath, [string[]]$Arguments) {
  $parts = New-Object System.Collections.ArrayList
  [void]$parts.Add($FilePath)
  foreach ($argument in @($Arguments)) {
    $text = [string]$argument
    if ($text -match '[\s"]') {
      $text = '"' + ($text -replace '"', '\"') + '"'
    }
    [void]$parts.Add($text)
  }
  return ($parts -join ' ')
}

function Invoke-Native {
  param(
    [Parameter(Mandatory = $true)][string]$FilePath,
    [string[]]$Arguments = @(),
    [string]$WorkingDirectory,
    [switch]$Capture,
    [switch]$AllowFailure
  )

  $previousLocation = $null
  if (-not [string]::IsNullOrWhiteSpace($WorkingDirectory)) {
    $previousLocation = (Get-Location).Path
    Set-Location -LiteralPath $WorkingDirectory
  }
  try {
    if ($Capture) {
      $buffer = @(& $FilePath @Arguments 2>&1)
      $exitCode = $LASTEXITCODE
      $text = (@($buffer) | ForEach-Object { [string]$_ }) -join "`n"
    } else {
      Write-Info ('执行：{0}' -f (Format-CommandLine $FilePath $Arguments))
      & $FilePath @Arguments
      $exitCode = $LASTEXITCODE
      $text = ''
    }
  } finally {
    if (-not [string]::IsNullOrWhiteSpace($previousLocation)) {
      Set-Location -LiteralPath $previousLocation
    }
  }

  $result = [pscustomobject]@{ ExitCode = $exitCode; Output = $text }
  if ($exitCode -ne 0 -and -not $AllowFailure) {
    $detail = $text.Trim()
    if ([string]::IsNullOrWhiteSpace($detail)) { $detail = '（无输出）' }
    throw ('命令失败（退出码 {0}）：{1}{2}{3}' -f $exitCode, (Format-CommandLine $FilePath $Arguments), [Environment]::NewLine, $detail)
  }
  return $result
}

function Assert-DockerAvailable {
  if (-not (Get-Command 'docker' -ErrorAction SilentlyContinue)) {
    throw '未找到 docker 命令；请安装 Docker Desktop / Docker Engine，并确认 docker 在 PATH 中。'
  }
  $probe = Invoke-Native 'docker' @('compose', 'version') -Capture -AllowFailure
  if ($probe.ExitCode -ne 0) {
    throw ('docker compose 不可用；请确认 Docker Compose v2（docker compose version）。{0}' -f $probe.Output.Trim())
  }
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

# ---------------------------------------------------------------------------
# Git / 镜像 / HTTP 辅助
# ---------------------------------------------------------------------------
function Get-GitInfo([string]$RepoRootPath) {
  $info = [ordered]@{
    available   = $false
    head        = 'unknown'
    branch      = 'unknown'
    dirty       = $false
    dirty_files = 0
  }
  if (-not (Get-Command 'git' -ErrorAction SilentlyContinue)) { return $info }
  $head = Invoke-Native 'git' @('-C', $RepoRootPath, 'rev-parse', '--short=12', 'HEAD') -Capture -AllowFailure
  if ($head.ExitCode -eq 0) {
    $info.head = $head.Output.Trim()
    $info.available = $true
  }
  $branch = Invoke-Native 'git' @('-C', $RepoRootPath, 'rev-parse', '--abbrev-ref', 'HEAD') -Capture -AllowFailure
  if ($branch.ExitCode -eq 0) { $info.branch = $branch.Output.Trim() }
  $status = Invoke-Native 'git' @('-C', $RepoRootPath, 'status', '--porcelain') -Capture -AllowFailure
  if ($status.ExitCode -eq 0) {
    $changed = @($status.Output -split "`n" | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    $info.dirty_files = $changed.Count
    $info.dirty = ($changed.Count -gt 0)
  }
  return $info
}

function Get-CurrentImages {
  param(
    [string]$ComposePath,
    [string]$ProjectName,
    [string[]]$ServiceNames
  )
  $images = New-Object System.Collections.ArrayList
  foreach ($service in $ServiceNames) {
    $containerId = ''
    $imageId = ''
    $imageRef = ''

    $psResult = Invoke-Native 'docker' @('compose', '-f', $ComposePath, 'ps', '-q', $service) -Capture -AllowFailure
    if ($psResult.ExitCode -eq 0) {
      $lines = @($psResult.Output -split "`n" | ForEach-Object { $_.Trim() } | Where-Object { $_ -ne '' })
      if ($lines.Count -gt 0) { $containerId = $lines[0] }
    }

    if (-not [string]::IsNullOrWhiteSpace($containerId)) {
      $inspectId = Invoke-Native 'docker' @('inspect', '--format', '{{.Image}}', $containerId) -Capture -AllowFailure
      if ($inspectId.ExitCode -eq 0) { $imageId = $inspectId.Output.Trim() }
      $inspectRef = Invoke-Native 'docker' @('inspect', '--format', '{{.Config.Image}}', $containerId) -Capture -AllowFailure
      if ($inspectRef.ExitCode -eq 0) { $imageRef = Get-NormalizedImageRef $inspectRef.Output.Trim() }
    } else {
      Write-WarnLine ('服务 {0} 没有运行中的容器，无法从容器记录镜像。' -f $service)
    }

    $fallbackRef = '{0}-{1}:latest' -f $ProjectName, $service
    if ([string]::IsNullOrWhiteSpace($imageId)) {
      $inspectImage = Invoke-Native 'docker' @('image', 'inspect', '--format', '{{.Id}}', $fallbackRef) -Capture -AllowFailure
      if ($inspectImage.ExitCode -eq 0) {
        $imageId = $inspectImage.Output.Trim()
        if ([string]::IsNullOrWhiteSpace($imageRef)) { $imageRef = $fallbackRef }
      }
    }
    if ([string]::IsNullOrWhiteSpace($imageRef)) { $imageRef = $fallbackRef }

    [void]$images.Add([ordered]@{
        service      = $service
        container_id = $containerId
        image_id     = $imageId
        image_ref    = $imageRef
        previous_tag = ('nover-{0}:previous' -f $service)
        tagged       = $false
      })
  }
  return @($images)
}

function Get-JsonOrNull {
  param([string]$Url, [hashtable]$Headers = @{})
  try {
    if ($Headers.Count -gt 0) {
      return Invoke-RestMethod -Uri $Url -Method Get -Headers $Headers -TimeoutSec 10
    }
    return Invoke-RestMethod -Uri $Url -Method Get -TimeoutSec 10
  } catch {
    Write-WarnLine ('读取 {0} 失败：{1}' -f $Url, $_.Exception.Message)
    return $null
  }
}

function Get-OperatorSystemStatus {
  param(
    [string]$ApiBase,
    [string]$OperatorPath,
    [hashtable]$EnvMap
  )
  $out = [ordered]@{
    queried          = $false
    login_user       = $null
    error            = $null
    app_version      = $null
    app_commit       = $null
    environment      = $null
    database_ready   = $null
    migrations       = [ordered]@{ applied = $null; pending = $null; latest = $null }
    schema_ready     = $null
    storage_writable = $null
  }

  $user = Get-EnvOrDefault $EnvMap 'ADMIN_USERNAME' ''
  $password = Get-EnvOrDefault $EnvMap 'ADMIN_PASSWORD' ''
  if ([string]::IsNullOrWhiteSpace($user) -or [string]::IsNullOrWhiteSpace($password)) {
    $out.error = '未在 .env 中配置 ADMIN_USERNAME / ADMIN_PASSWORD，跳过运营 system/status 查询。'
    return $out
  }

  $base = $ApiBase.TrimEnd('/')
  $loginUrl = '{0}/api/v1/{1}/login' -f $base, $OperatorPath
  $statusUrl = '{0}/api/v1/{1}/system/status' -f $base, $OperatorPath
  try {
    $payload = [ordered]@{ username = $user; password = $password } | ConvertTo-Json -Compress
    $bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($payload)
    $login = Invoke-RestMethod -Uri $loginUrl -Method Post -ContentType 'application/json; charset=utf-8' -Body $bodyBytes -TimeoutSec 10
    if ($null -ne $login.PSObject.Properties['mfa_required'] -and [bool]$login.mfa_required) {
      $out.error = '管理员已启用 MFA，脚本无法自动登录；请手动查看 system/status。'
      return $out
    }
    $token = [string]$login.access_token
    if ([string]::IsNullOrWhiteSpace($token)) {
      $out.error = '登录响应缺少 access_token。'
      return $out
    }
    $status = Invoke-RestMethod -Uri $statusUrl -Method Get -Headers @{ Authorization = ('Bearer ' + $token) } -TimeoutSec 10
    $out.queried = $true
    $out.login_user = $user
    $out.app_version = [string]$status.app.version
    $out.app_commit = [string]$status.app.commit
    $out.environment = [string]$status.app.environment
    $out.database_ready = [bool]$status.database.ready
    $out.migrations.applied = $status.database.migrations.applied
    $out.migrations.pending = $status.database.migrations.pending
    $out.migrations.latest = [string]$status.database.migrations.latest
    $out.schema_ready = [bool]$status.database.schema_ready
    $out.storage_writable = [bool]$status.storage.writable
    return $out
  } catch {
    $out.error = $_.Exception.Message
    return $out
  }
}

function Find-LatestUpgradeRecord([string]$Directory) {
  if (-not (Test-Path -LiteralPath $Directory -PathType Container)) { return $null }
  $candidates = @(Get-ChildItem -LiteralPath $Directory -Filter 'upgrade-*.json' -File -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending)
  foreach ($candidate in $candidates) {
    try {
      $data = Get-Content -LiteralPath $candidate.FullName -Raw -Encoding UTF8 | ConvertFrom-Json
      return [pscustomobject]@{ Path = $candidate.FullName; Data = $data }
    } catch {
      continue
    }
  }
  return $null
}

function Save-JsonRecord([object]$Record, [string]$Path) {
  $json = $Record | ConvertTo-Json -Depth 12
  $json = $json -replace "`r`n", "`n"
  $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllText($Path, ($json + "`n"), $utf8NoBom)
}

function New-OutputDirectory([string]$Path, [switch]$PlanOnly) {
  if (Test-Path -LiteralPath $Path -PathType Container) { return }
  if ($PlanOnly) {
    Write-Info ('将创建输出目录：{0}' -f $Path)
  } else {
    New-Item -ItemType Directory -Path $Path -Force | Out-Null
    Write-Info ('已创建输出目录：{0}' -f $Path)
  }
}

# ---------------------------------------------------------------------------
# 就绪探测 / 摘要输出
# ---------------------------------------------------------------------------
function Test-Endpoint([string]$Url) {
  try {
    $response = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 5
    return ($null -ne $response -and $response.StatusCode -eq 200)
  } catch {
    return $false
  }
}

function Wait-Endpoint([string]$Url, [int]$TimeoutSeconds) {
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  while ((Get-Date) -lt $deadline) {
    if (Test-Endpoint $Url) { return $true }
    Start-Sleep -Seconds 2
  }
  return (Test-Endpoint $Url)
}

function Get-VersionLabel([object]$Meta) {
  if ($null -eq $Meta) { return '（未获取）' }
  $version = [string]$Meta.version
  if ([string]::IsNullOrWhiteSpace($version)) { $version = 'unknown' }
  $commit = [string]$Meta.commit
  if (-not [string]::IsNullOrWhiteSpace($commit) -and $commit -ne 'unknown') {
    return ('{0}（{1}）' -f $version, $commit)
  }
  return $version
}

function Write-UpgradeSummary {
  param(
    [string]$Mode,
    [object]$BeforeMeta,
    [object]$AfterMeta,
    [object]$Status,
    [string]$RecordPath,
    [bool]$Ready
  )
  Write-Host ''
  Write-Host '================ 升级摘要 ================' -ForegroundColor Cyan
  Write-Host ('  模式：      {0}' -f $Mode)
  Write-Host ('  就绪状态：  {0}' -f $(if ($Ready) { '/readyz 200' } else { '未通过' }))
  Write-Host ('  旧版本：    {0}' -f (Get-VersionLabel $BeforeMeta))
  Write-Host ('  新版本：    {0}' -f (Get-VersionLabel $AfterMeta))
  if ($null -ne $Status -and $Status.queried) {
    Write-Host ('  数据库：    ready={0}，schema_ready={1}，存储可写={2}，环境={3}' -f $Status.database_ready, $Status.schema_ready, $Status.storage_writable, $Status.environment)
    Write-Host ('  迁移：      applied={0}，pending={1}，latest={2}' -f $Status.migrations.applied, $Status.migrations.pending, $Status.migrations.latest)
  } else {
    $reason = '未查询'
    if ($null -ne $Status -and -not [string]::IsNullOrWhiteSpace([string]$Status.error)) { $reason = $Status.error }
    Write-Host ('  迁移：      未读取（{0}）' -f $reason) -ForegroundColor Yellow
    if ($null -ne $AfterMeta -and $null -ne $AfterMeta.PSObject.Properties['schema_version']) {
      Write-Host ('  数据库 schema 版本（/api/v1/meta）：{0}' -f [string]$AfterMeta.schema_version)
    }
  }
  if (-not [string]::IsNullOrWhiteSpace($RecordPath)) {
    Write-Host ('  记录文件：  {0}' -f $RecordPath)
  }
  Write-Host ''
  Write-Host '回滚命令：.\scripts\ops\upgrade.ps1 -Rollback' -ForegroundColor Yellow
  Write-Host '提示：升级完成后请按 scripts/ops/README.md 的清单抽查控制台、公开目录与交付链路。' -ForegroundColor Yellow
}

function Write-RollbackSummary {
  param(
    [object]$BeforeMeta,
    [object]$AfterMeta,
    [object]$Status,
    [object[]]$Targets,
    [string]$RecordPath
  )
  Write-Host ''
  Write-Host '================ 回滚摘要 ================' -ForegroundColor Cyan
  foreach ($target in @($Targets)) {
    if ($target.exists) {
      Write-Host ('  {0}：{1} -> {2}' -f $target.service, $target.previous_tag, $target.target_ref)
    } else {
      Write-Host ('  {0}：未找到 {1}，已跳过' -f $target.service, $target.previous_tag) -ForegroundColor Yellow
    }
  }
  Write-Host ('  回滚前版本：{0}' -f (Get-VersionLabel $BeforeMeta))
  Write-Host ('  回滚后版本：{0}' -f (Get-VersionLabel $AfterMeta))
  if ($null -ne $Status -and $Status.queried) {
    Write-Host ('  数据库：    ready={0}，schema_ready={1}' -f $Status.database_ready, $Status.schema_ready)
    Write-Host ('  迁移：      applied={0}，pending={1}，latest={2}' -f $Status.migrations.applied, $Status.migrations.pending, $Status.migrations.latest)
  } elseif ($null -ne $Status -and -not [string]::IsNullOrWhiteSpace([string]$Status.error)) {
    Write-Host ('  迁移：      未读取（{0}）' -f $Status.error) -ForegroundColor Yellow
  }
  if (-not [string]::IsNullOrWhiteSpace($RecordPath)) {
    Write-Host ('  记录文件：  {0}' -f $RecordPath)
  }
  Write-Host ''
  Write-Host '回滚只恢复容器镜像，不回退数据库迁移与数据；如需回退数据请使用 scripts/ops/restore.ps1。' -ForegroundColor Yellow
}

# ---------------------------------------------------------------------------
# 主流程
# ---------------------------------------------------------------------------
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$workingDirectory = (Get-Location).Path
$createdRecords = New-Object System.Collections.ArrayList

try {
  # 0. 基础路径与参数
  $composePath = if ([string]::IsNullOrWhiteSpace($ComposeFile)) {
    Join-Path $repoRoot 'docker-compose.yml'
  } else {
    Resolve-FullPath $ComposeFile $workingDirectory
  }
  if (-not (Test-Path -LiteralPath $composePath -PathType Leaf)) {
    throw ('未找到 compose 文件：{0}' -f $composePath)
  }
  $composeDir = Split-Path -Parent $composePath
  $envPath = Join-Path $composeDir '.env'
  $envMap = Read-DotEnv $envPath
  $projectName = Get-ComposeProjectName $composePath
  $appVersion = Get-EnvOrDefault $envMap 'APP_VERSION' 'dev'
  $apiPort = Get-EnvOrDefault $envMap 'API_HOST_PORT' '48080'
  $operatorPath = (Get-EnvOrDefault $envMap 'OPERATOR_API_PATH' 'cp-7f3c').Trim('/')
  if ([string]::IsNullOrWhiteSpace($operatorPath)) { $operatorPath = 'cp-7f3c' }
  $apiBase = 'http://127.0.0.1:{0}' -f $apiPort
  $readyUrl = $apiBase + '/readyz'
  $outputPath = Resolve-FullPath $OutputDir $workingDirectory
  $stamp = [System.DateTime]::Now.ToString('yyyyMMdd-HHmmss', [System.Globalization.CultureInfo]::InvariantCulture)
  $services = if ($BackendOnly) { @('api') } else { @('api', 'web', 'console') }
  $composeArgs = [string[]]@('compose', '-f', $composePath)

  Write-Host ('NOVER 本地升级/回滚（{0}）' -f $script:ToolVersion) -ForegroundColor Cyan

  # 1. 预检
  Write-Step '1/6 预检'
  Write-Info ('仓库根目录：{0}' -f $repoRoot)
  Write-Info ('compose 文件：{0}' -f $composePath)
  Write-Info ('项目名：    {0}' -f $projectName)
  Write-Info ('输出目录：  {0}' -f $outputPath)
  Write-Info ('API 基址：  {0}' -f $apiBase)
  Write-Info ('目标服务：  {0}' -f ($services -join ', '))
  Write-Info ('APP_VERSION（.env）：{0}' -f $appVersion)

  $envExists = Test-Path -LiteralPath $envPath -PathType Leaf
  if ($envExists) {
    Write-Ok ('环境文件：{0}' -f $envPath)
  } else {
    Write-WarnLine ('未找到 {0}；docker compose 可能缺少 POSTGRES_PASSWORD / OPERATOR_TOKEN_KEY 等必填变量。' -f $envPath)
  }

  $dockerAvailable = [bool](Get-Command 'docker' -ErrorAction SilentlyContinue)
  if ($dockerAvailable) {
    Write-Ok 'docker 命令可用（实际执行前会校验 docker compose version）'
  } else {
    Write-WarnLine '未找到 docker 命令；实际执行升级/回滚会失败。'
  }

  $gitInfo = Get-GitInfo $repoRoot
  if ($gitInfo.available) {
    Write-Info ('git：{0} @ {1}' -f $gitInfo.branch, $gitInfo.head)
  } else {
    Write-WarnLine '未能读取 git 信息（未安装 git 或不是 git 仓库）。'
  }
  if ($gitInfo.dirty) {
    Write-WarnLine ('工作区有 {0} 个未提交改动；构建出的镜像会包含当前工作区内容，升级记录只记录 HEAD。' -f $gitInfo.dirty_files)
  }
  if ($Rollback -and $SkipBackup) {
    Write-WarnLine '-Rollback 模式不会备份，-SkipBackup 无额外影响。'
  }
  if ($DryRun) {
    Write-WarnLine 'DryRun 模式：只打印预检结果与计划命令，不调用 docker、不写文件。'
  }

  # 2. DryRun：打印计划后退出
  if ($DryRun) {
    if ($Rollback) {
      Write-Step 'DryRun：回滚计划（不会执行）'
      Write-Info '1) 检查 :previous 镜像是否存在（只读）：'
      foreach ($service in $services) {
        Write-Host ('       docker image inspect nover-{0}:previous' -f $service) -ForegroundColor DarkGray
      }
      Write-Info '2) 把 :previous 标签指回升级记录中的镜像引用（只打标签，不删除镜像）：'
      foreach ($service in $services) {
        $targetRef = '{0}-{1}:latest' -f $projectName, $service
        $previousRecord = Find-LatestUpgradeRecord $outputPath
        if ($null -ne $previousRecord) {
          foreach ($image in @($previousRecord.Data.images)) {
            if ([string]$image.service -eq $service -and -not [string]::IsNullOrWhiteSpace([string]$image.image_ref)) {
              $targetRef = Get-NormalizedImageRef ([string]$image.image_ref)
              break
            }
          }
        }
        Write-Host ('       docker tag nover-{0}:previous {1}' -f $service, $targetRef) -ForegroundColor DarkGray
      }
      Write-Info '3) 启动服务（不构建）：'
      if ($BackendOnly) {
        Write-Host ('       {0}' -f (Format-CommandLine 'docker' ([string[]]($composeArgs + @('up', '-d', 'api'))))) -ForegroundColor DarkGray
      } else {
        Write-Host ('       {0}' -f (Format-CommandLine 'docker' ([string[]]($composeArgs + @('up', '-d'))))) -ForegroundColor DarkGray
      }
      Write-Info ('4) 等待 API 就绪：GET {0} 返回 200（最多 {1} 秒）' -f $readyUrl, $TimeoutSeconds)
      Write-Info ('5) 读取版本与迁移状态：GET {0}/api/v1/meta、POST {1}/api/v1/{2}/login、GET {1}/api/v1/{2}/system/status' -f $apiBase, $apiBase, $operatorPath)
      Write-Info ('6) 写入回滚记录：{0}' -f (Join-Path $outputPath ('rollback-{0}.json' -f $stamp)))
      Write-Step 'DryRun 完成：未修改镜像、容器与数据'
      Write-Info '实际回滚请去掉 -DryRun（注意：只回退镜像，不回退数据库迁移）。'
      exit 0
    }

    Write-Step 'DryRun：升级计划（不会执行）'
    Write-Info '1) 记录当前镜像并打 :previous 标签（只读 + 仅新增标签，不删除镜像）：'
    foreach ($service in $services) {
      Write-Host ('       {0} ps -q {1}' -f (Format-CommandLine 'docker' $composeArgs), $service) -ForegroundColor DarkGray
      Write-Host '       docker inspect --format {{.Image}} <container-id>' -ForegroundColor DarkGray
      Write-Host '       docker inspect --format {{.Config.Image}} <container-id>' -ForegroundColor DarkGray
      Write-Host ('       docker tag <image-id> nover-{0}:previous' -f $service) -ForegroundColor DarkGray
    }
    Write-Info ('   升级记录：{0}' -f (Join-Path $outputPath ('upgrade-{0}.json' -f $stamp)))
    Write-Info '2) 备份：'
    if ($SkipBackup) {
      Write-Host ('       已指定 -SkipBackup：跳过 scripts/ops/backup.ps1') -ForegroundColor DarkGray
    } else {
      $powerShellExe = Resolve-PowerShellExe
      $backupScript = Join-Path $PSScriptRoot 'backup.ps1'
      $backupArguments = [string[]]@('-NoProfile')
      if ($env:OS -eq 'Windows_NT') { $backupArguments += @('-ExecutionPolicy', 'Bypass') }
      $backupArguments += @('-File', $backupScript, '-OutputDir', $outputPath, '-ComposeFile', $composePath)
      Write-Host ('       {0}' -f (Format-CommandLine $powerShellExe $backupArguments)) -ForegroundColor DarkGray
    }
    Write-Info '3) 构建并启动：'
    if ($BackendOnly) {
      Write-Host ('       {0}' -f (Format-CommandLine 'docker' ([string[]]($composeArgs + @('up', '-d', '--build', 'api'))))) -ForegroundColor DarkGray
    } else {
      Write-Host ('       {0}' -f (Format-CommandLine 'docker' ([string[]]($composeArgs + @('build'))))) -ForegroundColor DarkGray
      Write-Host ('       {0}' -f (Format-CommandLine 'docker' ([string[]]($composeArgs + @('up', '-d'))))) -ForegroundColor DarkGray
    }
    Write-Info ('4) 等待 API 就绪：GET {0} 返回 200（最多 {1} 秒）' -f $readyUrl, $TimeoutSeconds)
    Write-Info ('5) 读取版本与迁移状态：GET {0}/api/v1/meta、POST {0}/api/v1/{1}/login、GET {0}/api/v1/{1}/system/status' -f $apiBase, $operatorPath)
    Write-Step 'DryRun 完成：未修改镜像、容器与数据'
    Write-Info '实际升级请去掉 -DryRun；建议先确认备份输出目录所在磁盘空间充足。'
    exit 0
  }

  # =========================================================================
  # 回滚
  # =========================================================================
  if ($Rollback) {
    Assert-DockerAvailable
    $mode = 'rollback'

    Write-Step '2/6 检查 :previous 镜像'
    $previousRecord = Find-LatestUpgradeRecord $outputPath
    $previousRecordPath = $null
    if ($null -ne $previousRecord) {
      $previousRecordPath = $previousRecord.Path
      Write-Ok ('已找到最近一次升级记录：{0}' -f $previousRecordPath)
    } else {
      Write-WarnLine ('输出目录中没有 upgrade-*.json；只能按约定镜像名回滚（{0}-<service>:latest）。' -f $projectName)
    }

    $targets = New-Object System.Collections.ArrayList
    foreach ($service in $services) {
      $previousTag = 'nover-{0}:previous' -f $service
      $probe = Invoke-Native 'docker' @('image', 'inspect', $previousTag) -Capture -AllowFailure
      $exists = ($probe.ExitCode -eq 0)
      $targetRef = ''
      if ($null -ne $previousRecord) {
        foreach ($image in @($previousRecord.Data.images)) {
          if ([string]$image.service -eq $service) {
            $targetRef = Get-NormalizedImageRef ([string]$image.image_ref)
            break
          }
        }
      }
      if ([string]::IsNullOrWhiteSpace($targetRef) -or $targetRef -eq $previousTag) {
        $targetRef = '{0}-{1}:latest' -f $projectName, $service
      }
      [void]$targets.Add([ordered]@{
          service      = $service
          previous_tag = $previousTag
          exists       = $exists
          target_ref   = $targetRef
        })
      if ($exists) {
        Write-Ok ('{0} -> {1}' -f $previousTag, $targetRef)
      } else {
        Write-WarnLine ('未找到 {0}（该服务将保持当前镜像）。' -f $previousTag)
      }
    }

    $available = @($targets | Where-Object { $_.exists })
    if ($available.Count -eq 0) {
      Write-ErrorLine ('未找到任何 :previous 镜像（尝试：{0}）。' -f (($targets | ForEach-Object { $_.previous_tag }) -join ', '))
      Write-Info '说明：升级脚本升级前会给当前镜像打 :previous 标签；当前 host 上没有该标签时无法自动回滚。'
      Write-Info '可改用 scripts/ops/restore.ps1 从备份恢复数据，并手动把镜像切到旧版本。'
      exit 2
    }

    Write-Step '3/6 记录回滚计划并打标签'
    $beforeMeta = Get-JsonOrNull ($apiBase + '/api/v1/meta')
    $record = [ordered]@{
      rollback_version = 1
      tool             = $script:ScriptName
      tool_version     = $script:ToolVersion
      mode             = $mode
      result           = 'in_progress'
      started_at       = (Get-UtcNowString)
      started_at_local = (Get-LocalNowString)
      finished_at      = $null
      host             = [System.Environment]::MachineName
      repo             = [ordered]@{
        root               = $repoRoot
        git_commit         = $gitInfo.head
        git_branch         = $gitInfo.branch
        working_tree_dirty = $gitInfo.dirty
        dirty_files        = $gitInfo.dirty_files
      }
      compose          = [ordered]@{ file = $composePath; project = $projectName }
      backend_only     = [bool]$BackendOnly
      services         = $services
      source_record    = $previousRecordPath
      images           = @($targets)
      before           = [ordered]@{ meta = $beforeMeta }
      after            = [ordered]@{ meta = $null; system_status = $null }
      health           = [ordered]@{ ready = $false; url = $readyUrl }
      error            = $null
    }
    New-OutputDirectory $outputPath
    $recordPath = Join-Path $outputPath ('rollback-{0}.json' -f $stamp)
    Save-JsonRecord $record $recordPath
    [void]$createdRecords.Add($recordPath)
    Write-Ok ('回滚记录：{0}' -f $recordPath)

    foreach ($target in $targets) {
      if (-not $target.exists) { continue }
      Invoke-Native 'docker' @('tag', $target.previous_tag, $target.target_ref) | Out-Null
      Write-Ok ('{0} -> {1}' -f $target.previous_tag, $target.target_ref)
    }

    Write-Step '4/6 启动服务（不构建）'
    if ($BackendOnly) {
      Invoke-Native 'docker' ([string[]]($composeArgs + @('up', '-d', 'api'))) | Out-Null
    } else {
      Invoke-Native 'docker' ([string[]]($composeArgs + @('up', '-d'))) | Out-Null
    }

    Write-Step ('5/6 等待 API 就绪：{0}（最多 {1} 秒）' -f $readyUrl, $TimeoutSeconds)
    if (-not (Wait-Endpoint $readyUrl $TimeoutSeconds)) {
      throw ('API 在 {0} 秒内未就绪：{1}；请检查 docker compose ps 与 docker compose logs api --tail 200。' -f $TimeoutSeconds, $readyUrl)
    }
    Write-Ok 'API /readyz 返回 200'
    $record['health']['ready'] = $true

    Write-Step '6/6 读取版本与迁移状态'
    $afterMeta = Get-JsonOrNull ($apiBase + '/api/v1/meta')
    $status = Get-OperatorSystemStatus -ApiBase $apiBase -OperatorPath $operatorPath -EnvMap $envMap
    if ($status.queried) {
      Write-Ok ('迁移状态：applied={0}，pending={1}，latest={2}' -f $status.migrations.applied, $status.migrations.pending, $status.migrations.latest)
    } else {
      Write-WarnLine ('system/status 未查询成功：{0}' -f $status.error)
    }
    $record['after']['meta'] = $afterMeta
    $record['after']['system_status'] = $status
    $record['result'] = 'success'
    $record['finished_at'] = (Get-UtcNowString)
    Save-JsonRecord $record $recordPath

    Write-RollbackSummary -BeforeMeta $beforeMeta -AfterMeta $afterMeta -Status $status -Targets @($targets) -RecordPath $recordPath
    Write-Host '回滚完成。请按 scripts/ops/README.md 的验证清单抽查控制台、公开目录与交付链路。' -ForegroundColor Green
    exit 0
  }

  # =========================================================================
  # 升级
  # =========================================================================
  Assert-DockerAvailable
  $mode = if ($BackendOnly) { 'upgrade-backend' } else { 'upgrade-full' }

  Write-Step ('2/6 记录当前镜像并打 :previous 标签（{0}）' -f ($services -join ', '))
  $images = Get-CurrentImages -ComposePath $composePath -ProjectName $projectName -ServiceNames $services
  $beforeMeta = Get-JsonOrNull ($apiBase + '/api/v1/meta')
  $record = [ordered]@{
    upgrade_version  = 1
    tool             = $script:ScriptName
    tool_version     = $script:ToolVersion
    mode             = $mode
    result           = 'in_progress'
    started_at       = (Get-UtcNowString)
    started_at_local = (Get-LocalNowString)
    finished_at      = $null
    host             = [System.Environment]::MachineName
    repo             = [ordered]@{
      root               = $repoRoot
      git_commit         = $gitInfo.head
      git_branch         = $gitInfo.branch
      working_tree_dirty = $gitInfo.dirty
      dirty_files        = $gitInfo.dirty_files
    }
    compose          = [ordered]@{ file = $composePath; project = $projectName }
    backend_only     = [bool]$BackendOnly
    services         = $services
    app_version      = $appVersion
    output_dir       = $outputPath
    backup           = [ordered]@{
      requested  = (-not $SkipBackup)
      skipped    = [bool]$SkipBackup
      script     = 'scripts/ops/backup.ps1'
      output_dir = $outputPath
    }
    images           = @($images)
    before           = [ordered]@{ meta = $beforeMeta }
    after            = [ordered]@{ meta = $null; system_status = $null }
    health           = [ordered]@{ ready = $false; url = $readyUrl }
    error            = $null
  }
  New-OutputDirectory $outputPath
  $recordPath = Join-Path $outputPath ('upgrade-{0}.json' -f $stamp)
  Save-JsonRecord $record $recordPath
  [void]$createdRecords.Add($recordPath)
  Write-Ok ('升级记录：{0}' -f $recordPath)

  foreach ($image in $images) {
    if ([string]::IsNullOrWhiteSpace([string]$image.image_id)) {
      Write-WarnLine ('服务 {0} 没有可记录的镜像，无法为它创建 :previous 标签。' -f $image.service)
      continue
    }
    Invoke-Native 'docker' @('tag', $image.image_id, $image.previous_tag) | Out-Null
    $image.tagged = $true
    Write-Ok ('{0} -> {1}' -f $image.previous_tag, $image.image_ref)
  }
  Save-JsonRecord $record $recordPath

  Write-Step ('3/6 备份（{0}）' -f $(if ($SkipBackup) { '已跳过' } else { 'scripts/ops/backup.ps1' }))
  if ($SkipBackup) {
    Write-WarnLine '已指定 -SkipBackup：跳过备份。升级失败时只能依赖历史备份与 :previous 镜像。'
  } else {
    $backupScript = Join-Path $PSScriptRoot 'backup.ps1'
    if (-not (Test-Path -LiteralPath $backupScript -PathType Leaf)) {
      throw ('未找到备份脚本：{0}' -f $backupScript)
    }
    $powerShellExe = Resolve-PowerShellExe
    $backupArguments = [string[]]@('-NoProfile')
    if ($env:OS -eq 'Windows_NT') { $backupArguments += @('-ExecutionPolicy', 'Bypass') }
    $backupArguments += @('-File', $backupScript, '-OutputDir', $outputPath, '-ComposeFile', $composePath)
    Invoke-Native $powerShellExe $backupArguments | Out-Null
    Write-Ok '备份完成（数据库 + private-assets 卷 + manifest）'
  }

  Write-Step '4/6 构建并启动'
  if ($BackendOnly) {
    Invoke-Native 'docker' ([string[]]($composeArgs + @('up', '-d', '--build', 'api'))) | Out-Null
  } else {
    Invoke-Native 'docker' ([string[]]($composeArgs + @('build'))) | Out-Null
    Invoke-Native 'docker' ([string[]]($composeArgs + @('up', '-d'))) | Out-Null
  }
  Write-Ok '容器已按新镜像启动'

  Write-Step ('5/6 等待 API 就绪：{0}（最多 {1} 秒）' -f $readyUrl, $TimeoutSeconds)
  if (-not (Wait-Endpoint $readyUrl $TimeoutSeconds)) {
    throw ('API 在 {0} 秒内未就绪：{1}；请检查 docker compose ps 与 docker compose logs api --tail 200。' -f $TimeoutSeconds, $readyUrl)
  }
  Write-Ok 'API /readyz 返回 200'
  $record['health']['ready'] = $true

  Write-Step '6/6 读取版本与迁移状态'
  $afterMeta = Get-JsonOrNull ($apiBase + '/api/v1/meta')
  $status = Get-OperatorSystemStatus -ApiBase $apiBase -OperatorPath $operatorPath -EnvMap $envMap
  if ($status.queried) {
    Write-Ok ('迁移状态：applied={0}，pending={1}，latest={2}' -f $status.migrations.applied, $status.migrations.pending, $status.migrations.latest)
  } else {
    Write-WarnLine ('system/status 未查询成功：{0}' -f $status.error)
  }
  $record['after']['meta'] = $afterMeta
  $record['after']['system_status'] = $status
  $record['result'] = 'success'
  $record['finished_at'] = (Get-UtcNowString)
  Save-JsonRecord $record $recordPath

  Write-UpgradeSummary -Mode $mode -BeforeMeta $beforeMeta -AfterMeta $afterMeta -Status $status -RecordPath $recordPath -Ready $true
  Write-Host '升级完成。请按 scripts/ops/README.md 的验证清单抽查控制台、公开目录与交付链路。' -ForegroundColor Green
  exit 0
} catch {
  $failureMessage = $_.Exception.Message
  if ($null -ne $record -and -not [string]::IsNullOrWhiteSpace([string]$recordPath)) {
    try {
      $record['result'] = 'failed'
      $record['error'] = $failureMessage
      $record['finished_at'] = (Get-UtcNowString)
      Save-JsonRecord $record $recordPath
      Write-WarnLine ('失败记录已写入：{0}' -f $recordPath)
    } catch {
      Write-WarnLine ('升级/回滚记录写入失败：{0}' -f $_.Exception.Message)
    }
  }
  Write-ErrorLine $failureMessage
  Write-Host ''
  Write-Host '操作失败：未执行 docker compose down，未删除卷，未删除镜像。' -ForegroundColor Red
  if ($Rollback) {
    Write-Host '排查建议：docker compose ps / docker compose logs api --tail 200；必要时手动检查 :previous 镜像与 compose 文件。' -ForegroundColor Yellow
  } elseif (-not [string]::IsNullOrWhiteSpace([string]$recordPath)) {
    Write-Host '若新版本不可用，可用升级前记录的 :previous 镜像回滚：.\scripts\ops\upgrade.ps1 -Rollback' -ForegroundColor Yellow
  } else {
    Write-Host '排查建议：核对 -ComposeFile / 仓库根目录 / .env 与 docker 状态，先用 -DryRun 打印计划。' -ForegroundColor Yellow
  }
  exit 1
}
