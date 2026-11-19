#Requires -Version 5.1

<#
.SYNOPSIS
  NOVER 恢复：校验备份与 manifest → 停止应用 → pg_restore → 还原 private-assets 卷 → 启动并验证。

.DESCRIPTION
  破坏性操作，必须显式传入 -Confirm 才会执行：

    # 1) 先演练（只校验并打印计划，不执行任何破坏性操作）
    .\scripts\ops\restore.ps1 -DatabaseDump .\backups\nover-20261119-134500.dump `
        -AssetsArchive .\backups\private-assets-20261119-134500.tar.gz -DryRun

    # 2) 确认后执行（会停止 api/web/console，并覆盖数据库与卷内容）
    .\scripts\ops\restore.ps1 -DatabaseDump .\backups\nover-20261119-134500.dump `
        -AssetsArchive .\backups\private-assets-20261119-134500.tar.gz -Confirm

  兼容 Windows PowerShell 5.1 / PowerShell 7 / Linux pwsh；文件保持 UTF-8 BOM + LF。

  安全顺序：文件与 manifest 校验在和任何 docker 变更之前完成；失败时尽量把服务重新拉起。
  退出码：0 成功；1 校验/执行失败；2 缺少 -Confirm 或参数拒绝。
#>
[CmdletBinding()]
param(
  [Parameter(Mandatory = $true, Position = 0)][string]$DatabaseDump,
  [Parameter(Mandatory = $true, Position = 1)][string]$AssetsArchive,
  [string]$Manifest,
  [string]$ComposeFile,
  [string]$ProjectName,
  [string]$DatabaseContainer = 'db',
  [string]$VolumeName,
  [string]$DatabaseUser,
  [string]$DatabaseName,
  [string]$HelperImage = 'alpine:3.20',
  [string]$ReadyUrl = 'http://127.0.0.1:48080/readyz',
  [int]$ReadyTimeoutSeconds = 90,
  [switch]$Confirm,
  [switch]$DryRun,
  [switch]$SkipManifestCheck,
  [switch]$SkipReadyCheck
)

$ErrorActionPreference = 'Stop'
$script:ToolVersion = '0.4.0'
$script:ScriptName = 'scripts/ops/restore.ps1'
$script:servicesStopped = $false

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

function ConvertTo-DockerMountPath([string]$Path) {
  return $Path.Replace('\', '/')
}

# ---------------------------------------------------------------------------
# 进程执行（PowerShell 5.1 兼容）
# ---------------------------------------------------------------------------
function ConvertTo-ProcessArgument([string]$Value) {
  if ($null -eq $Value) { return '""' }
  if ($Value.Length -eq 0) { return '""' }
  if ($Value -notmatch '[\s"]') { return $Value }

  $builder = New-Object System.Text.StringBuilder
  [void]$builder.Append('"')
  $backslashes = 0
  foreach ($character in $Value.ToCharArray()) {
    if ($character -eq [char]'\') {
      $backslashes++
      continue
    }
    if ($character -eq [char]'"') {
      [void]$builder.Append(('\' * (($backslashes * 2) + 1)))
      [void]$builder.Append('"')
      $backslashes = 0
      continue
    }
    if ($backslashes -gt 0) {
      [void]$builder.Append(('\' * $backslashes))
      $backslashes = 0
    }
    [void]$builder.Append($character)
  }
  if ($backslashes -gt 0) {
    [void]$builder.Append(('\' * ($backslashes * 2)))
  }
  [void]$builder.Append('"')
  return $builder.ToString()
}

function Format-CommandLine([string]$FilePath, [string[]]$Arguments) {
  $parts = New-Object System.Collections.ArrayList
  [void]$parts.Add((ConvertTo-ProcessArgument $FilePath))
  foreach ($argument in @($Arguments)) {
    [void]$parts.Add((ConvertTo-ProcessArgument ([string]$argument)))
  }
  return ($parts -join ' ')
}

function Invoke-Process {
  param(
    [Parameter(Mandatory = $true)][string]$FilePath,
    [string[]]$Arguments = @(),
    [string]$WorkingDirectory,
    [string]$StdoutFile,
    [string]$StdinFile,
    [switch]$CaptureStdout,
    [switch]$AllowFailure,
    [string]$Description
  )

  if (-not [string]::IsNullOrWhiteSpace($Description)) {
    Write-Info ('执行：{0}' -f (Format-CommandLine $FilePath $Arguments))
  }

  $psi = New-Object System.Diagnostics.ProcessStartInfo
  $psi.FileName = $FilePath
  $psi.Arguments = ((@($Arguments) | ForEach-Object { ConvertTo-ProcessArgument ([string]$_) }) -join ' ')
  $psi.UseShellExecute = $false
  $psi.CreateNoWindow = $true
  $psi.RedirectStandardError = $true
  if (-not [string]::IsNullOrWhiteSpace($WorkingDirectory)) {
    $psi.WorkingDirectory = $WorkingDirectory
  }
  $captureOutput = $CaptureStdout -or (-not [string]::IsNullOrWhiteSpace($StdoutFile))
  if ($captureOutput) { $psi.RedirectStandardOutput = $true }
  if (-not [string]::IsNullOrWhiteSpace($StdinFile)) { $psi.RedirectStandardInput = $true }

  $process = New-Object System.Diagnostics.Process
  $process.StartInfo = $psi
  $started = $false
  $exitCode = -1
  $stdoutText = ''
  $stderrText = ''

  try {
    $started = $process.Start()
    if (-not $started) { throw ('无法启动进程：{0}' -f $FilePath) }

    $stderrTask = $process.StandardError.ReadToEndAsync()

    if (-not [string]::IsNullOrWhiteSpace($StdinFile)) {
      $inputStream = [System.IO.File]::OpenRead($StdinFile)
      try {
        $inputStream.CopyTo($process.StandardInput.BaseStream)
        $process.StandardInput.BaseStream.Flush()
      } finally {
        $inputStream.Dispose()
      }
      try { $process.StandardInput.Close() } catch { }
    }

    if (-not [string]::IsNullOrWhiteSpace($StdoutFile)) {
      $outputStream = [System.IO.File]::Create($StdoutFile)
      try {
        $process.StandardOutput.BaseStream.CopyTo($outputStream)
      } finally {
        $outputStream.Dispose()
      }
    } elseif ($captureOutput) {
      $stdoutText = $process.StandardOutput.ReadToEnd()
    }

    $process.WaitForExit()
    $exitCode = $process.ExitCode
    try { $stderrText = [string]$stderrTask.Result } catch { $stderrText = '' }
  } catch {
    if ($started -and -not $process.HasExited) {
      try { $process.Kill() } catch { }
    }
    throw
  } finally {
    $process.Dispose()
  }

  $result = [pscustomobject]@{
    ExitCode = $exitCode
    StdOut   = $stdoutText
    StdErr   = $stderrText
  }

  if ($exitCode -ne 0 -and -not $AllowFailure) {
    $detail = $stderrText
    if ([string]::IsNullOrWhiteSpace($detail)) { $detail = $stdoutText }
    if ([string]::IsNullOrWhiteSpace($detail)) { $detail = '（无输出）' }
    throw ('命令失败（退出码 {0}）：{1}{2}{3}' -f $exitCode, $FilePath, [Environment]::NewLine, $detail.Trim())
  }
  return $result
}

function Assert-DockerAvailable {
  if (-not (Get-Command 'docker' -ErrorAction SilentlyContinue)) {
    throw '未找到 docker 命令；请安装 Docker Desktop / Docker Engine，并确认 docker 在 PATH 中。'
  }
  $probe = Invoke-Process -FilePath 'docker' -Arguments @('compose', 'version') -CaptureStdout -AllowFailure
  if ($probe.ExitCode -ne 0) {
    throw ('docker compose 不可用；请确认 Docker Compose v2（docker compose version）。{0}' -f $probe.StdErr.Trim())
  }
}

function Get-FileSha256([string]$Path) {
  return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
}

function Test-DatabaseDumpFormat([string]$Path) {
  $stream = [System.IO.File]::OpenRead($Path)
  try {
    $buffer = New-Object byte[] 5
    $read = $stream.Read($buffer, 0, 5)
  } finally {
    $stream.Dispose()
  }
  if ($read -lt 5) { throw ('数据库备份文件过短，疑似损坏：{0}' -f $Path) }
  $magic = [System.Text.Encoding]::ASCII.GetString($buffer)
  if ($magic -ne 'PGDMP') {
    throw ('数据库备份不是 pg_dump 自定义格式（文件头为 "{0}"）：{1}；请使用 backup.ps1 生成的 .dump。' -f $magic, $Path)
  }
}

function Test-AssetsArchiveFormat([string]$Path) {
  $stream = [System.IO.File]::OpenRead($Path)
  try {
    $buffer = New-Object byte[] 2
    $read = $stream.Read($buffer, 0, 2)
  } finally {
    $stream.Dispose()
  }
  if ($read -lt 2) { throw ('资产归档文件过短，疑似损坏：{0}' -f $Path) }
  if ($buffer[0] -ne 0x1F -or $buffer[1] -ne 0x8B) {
    Write-WarnLine ('资产归档不是 gzip 头（0x1F 0x8B），仍按 manifest 继续；请确认这是 backup.ps1 生成的 .tar.gz。')
  }
}

function Find-BackupManifest([string]$DumpPath, [string]$ArchivePath) {
  $dir = Split-Path -Parent $DumpPath
  $candidates = @(Get-ChildItem -LiteralPath $dir -Filter 'manifest-*.json' -File -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending)
  $dumpName = [System.IO.Path]::GetFileName($DumpPath)
  $archiveName = [System.IO.Path]::GetFileName($ArchivePath)
  foreach ($candidate in $candidates) {
    try {
      $parsed = Get-Content -LiteralPath $candidate.FullName -Raw -Encoding UTF8 | ConvertFrom-Json
    } catch {
      continue
    }
    if ($null -eq $parsed.database -or $null -eq $parsed.assets) { continue }
    if ([string]$parsed.database.file -eq $dumpName -and [string]$parsed.assets.file -eq $archiveName) {
      return $candidate.FullName
    }
  }
  return $null
}

function Assert-FileMatchesManifest([string]$Path, [object]$Entry, [string]$Label) {
  if ($null -eq $Entry) { throw ('manifest 缺少 {0} 条目。' -f $Label) }
  $expectedSha = ''
  if ($null -ne $Entry.PSObject.Properties['sha256']) {
    $expectedSha = ([string]$Entry.sha256).ToLowerInvariant()
  }
  if ([string]::IsNullOrWhiteSpace($expectedSha)) { throw ('manifest 缺少 {0}.sha256。' -f $Label) }

  $expectedBytes = -1L
  if ($null -ne $Entry.PSObject.Properties['bytes']) {
    $parsed = 0L
    if ([long]::TryParse([string]$Entry.bytes, [ref]$parsed)) { $expectedBytes = $parsed }
  }

  $actualBytes = (Get-Item -LiteralPath $Path).Length
  $actualSha = Get-FileSha256 $Path
  if ($expectedBytes -ge 0 -and $expectedBytes -ne $actualBytes) {
    throw ('{0} 大小不匹配：manifest 记录 {1} 字节，实际 {2} 字节。' -f $Label, $expectedBytes, $actualBytes)
  }
  if ($actualSha -ne $expectedSha) {
    throw ('{0} SHA-256 不匹配：实际 {1}，manifest 记录 {2}。文件可能损坏或被替换，已中止恢复。' -f $Label, $actualSha, $expectedSha)
  }
  Write-Ok ('{0} 校验通过（{1} 字节，SHA-256 {2}）' -f $Label, $actualBytes, $actualSha)
}

function Wait-DatabaseReady([string]$ComposePath, [string]$Container, [string]$User, [string]$Name, [int]$TimeoutSeconds = 60) {
  $composeDir = Split-Path -Parent $ComposePath
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  while ((Get-Date) -lt $deadline) {
    $probe = Invoke-Process -FilePath 'docker' -Arguments @('compose', '-f', $ComposePath, 'exec', '-T', $Container, 'pg_isready', '-U', $User, '-d', $Name) -WorkingDirectory $composeDir -CaptureStdout -AllowFailure
    if ($probe.ExitCode -eq 0) { return }
    Start-Sleep -Seconds 2
  }
  throw ('数据库容器 {0} 在 {1} 秒内未就绪；请检查 docker compose logs {0}。' -f $Container, $TimeoutSeconds)
}

function Wait-ApiReady([string]$Url, [int]$TimeoutSeconds) {
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  while ((Get-Date) -lt $deadline) {
    try {
      $response = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 5
      if ($response.StatusCode -eq 200) { return $true }
    } catch { }
    Start-Sleep -Seconds 2
  }
  return $false
}

function Write-VerificationChecklist([string]$ReadyAddress, [string]$ManifestPath, [object]$ManifestObject) {
  $apiBase = $ReadyAddress
  if ($apiBase -match '/readyz/?$') {
    $apiBase = [regex]::Replace($apiBase, '/readyz/?$', '')
  }
  Write-Host ''
  Write-Host '================ 恢复后验证清单 ================' -ForegroundColor Cyan
  Write-Host '  [ ] 1. 容器状态正常：docker compose ps'
  Write-Host ('  [ ] 2. 数据库就绪：curl.exe -fsS {0}' -f $ReadyAddress)
  Write-Host ('  [ ] 3. 版本元数据：curl.exe -fsS {0}/api/v1/meta' -f $apiBase)
  Write-Host '  [ ] 4. 控制台登录（默认 /ops-7f3c），确认账号、商品、兑换码与审计记录可见'
  Write-Host '  [ ] 5. 公开目录（默认 http://localhost:45173）可打开，商品与图片显示正常'
  Write-Host '  [ ] 6. 兑换 → 交付 → 下载票据全链路抽查；下载文件 SHA-256 与页面一致'
  Write-Host '  [ ] 7. 收益/审计数据与备份时间点一致，无 draft 文件泄漏到交付接口'
  Write-Host '  [ ] 8. 观察日志：docker compose logs api --tail 100 无迁移或权限错误'
  Write-Host '  [ ] 9. 记录本次恢复时间、manifest 路径与执行人'
  if ($null -ne $ManifestObject) {
    Write-Host ''
    Write-Host ('  manifest：{0}' -f $ManifestPath)
    Write-Host ('  备份时间：{0}' -f [string]$ManifestObject.created_at)
    Write-Host ('  项目/卷：{0} / {1}' -f [string]$ManifestObject.compose.project, [string]$ManifestObject.assets.volume)
    Write-Host ('  数据库：{0}' -f [string]$ManifestObject.database.name)
  }
}

# ---------------------------------------------------------------------------
# 主流程
# ---------------------------------------------------------------------------
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$workingDirectory = (Get-Location).Path

try {
  # 1. 输入文件与 compose 定位（此阶段不触碰 docker）
  $dumpPath = Resolve-FullPath $DatabaseDump $workingDirectory
  $archivePath = Resolve-FullPath $AssetsArchive $workingDirectory
  foreach ($input in @(
      [pscustomobject]@{ Path = $dumpPath; Label = '数据库备份' },
      [pscustomobject]@{ Path = $archivePath; Label = '资产归档' }
    )) {
    if (-not (Test-Path -LiteralPath $input.Path -PathType Leaf)) {
      throw ('{0} 不存在：{1}' -f $input.Label, $input.Path)
    }
  }

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
  if ([string]::IsNullOrWhiteSpace($ProjectName)) { $ProjectName = Get-ComposeProjectName $composePath }
  if ([string]::IsNullOrWhiteSpace($VolumeName)) { $VolumeName = '{0}_private-assets' -f $ProjectName }
  if ([string]::IsNullOrWhiteSpace($DatabaseUser)) { $DatabaseUser = Get-EnvOrDefault $envMap 'POSTGRES_USER' 'nover' }
  if ([string]::IsNullOrWhiteSpace($DatabaseName)) { $DatabaseName = Get-EnvOrDefault $envMap 'POSTGRES_DB' 'nover' }

  Write-Host ('NOVER 恢复（{0}）' -f $script:ToolVersion) -ForegroundColor Cyan
  Write-Info ('仓库根目录：{0}' -f $repoRoot)
  Write-Info ('数据库备份：{0}' -f $dumpPath)
  Write-Info ('资产归档：  {0}' -f $archivePath)
  Write-Info ('compose：   {0}' -f $composePath)
  Write-Info ('目标数据库：{0} / {1}（容器 {2}）' -f $DatabaseUser, $DatabaseName, $DatabaseContainer)
  Write-Info ('目标资产卷：{0}' -f $VolumeName)
  if ($DryRun) { Write-WarnLine 'DryRun 模式：只校验并打印计划，不执行任何破坏性操作。' }

  # 2. 格式与 manifest 校验（先于任何 docker 变更）
  Write-Step '校验备份文件'
  Test-DatabaseDumpFormat $dumpPath
  Test-AssetsArchiveFormat $archivePath
  Write-Ok '数据库备份为 pg_dump 自定义格式（PGDMP）'

  $manifestPath = $null
  if (-not [string]::IsNullOrWhiteSpace($Manifest)) {
    $manifestPath = Resolve-FullPath $Manifest $workingDirectory
    if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
      throw ('manifest 不存在：{0}' -f $manifestPath)
    }
  } else {
    $manifestPath = Find-BackupManifest $dumpPath $archivePath
  }

  $manifestObject = $null
  if ([string]::IsNullOrWhiteSpace($manifestPath)) {
    if (-not $SkipManifestCheck) {
      throw ('未找到与备份匹配的 manifest-*.json；请用 -Manifest 指定，或在确认风险后使用 -SkipManifestCheck。')
    }
    Write-WarnLine '未找到 manifest，且已指定 -SkipManifestCheck：跳过 SHA-256 校验（不推荐）。'
  } else {
    Write-Step ('校验 manifest：{0}' -f $manifestPath)
    $manifestObject = Get-Content -LiteralPath $manifestPath -Raw -Encoding UTF8 | ConvertFrom-Json
    if ($null -eq $manifestObject.database -or $null -eq $manifestObject.assets) {
      throw ('manifest 结构不完整（缺少 database/assets）：{0}' -f $manifestPath)
    }
    Assert-FileMatchesManifest $dumpPath $manifestObject.database '数据库备份'
    Assert-FileMatchesManifest $archivePath $manifestObject.assets '资产归档'
    $manifestVolume = [string]$manifestObject.assets.volume
    if (-not [string]::IsNullOrWhiteSpace($manifestVolume) -and $manifestVolume -ne $VolumeName) {
      Write-WarnLine ('manifest 记录的卷名为 {0}，本次目标卷为 {1}；请确认这是预期行为。' -f $manifestVolume, $VolumeName)
    }
  }

  # 3. 计划步骤（DryRun 与实际执行共用同一份定义）
  $composeArgs = [string[]]@('compose', '-f', $composePath)
  $volumeStepArguments = [string[]]@(
    'run', '--rm',
    '-v', ('{0}:/data' -f $VolumeName),
    '-v', ('{0}:/backup/assets.tar.gz:ro' -f (ConvertTo-DockerMountPath $archivePath)),
    $HelperImage,
    'sh', '-c', 'set -e; find /data -mindepth 1 -maxdepth 1 -exec rm -rf {} +; tar xzf /backup/assets.tar.gz -C /data'
  )

  $steps = @(
    [pscustomobject]@{
      Label         = '确保数据库容器运行'
      Arguments     = [string[]]($composeArgs + @('up', '-d', $DatabaseContainer))
      StdinFile     = $null
      WaitDatabase  = $true
    },
    [pscustomobject]@{
      Label         = '停止应用服务 api / web / console'
      Arguments     = [string[]]($composeArgs + @('stop', 'api', 'web', 'console'))
      StdinFile     = $null
      WaitDatabase  = $false
    },
    [pscustomobject]@{
      Label         = '恢复数据库（pg_restore --clean --if-exists，标准输入为备份文件）'
      Arguments     = [string[]]($composeArgs + @('exec', '-T', $DatabaseContainer, 'pg_restore', '-U', $DatabaseUser, '-d', $DatabaseName, '--clean', '--if-exists'))
      StdinFile     = $dumpPath
      WaitDatabase  = $false
    },
    [pscustomobject]@{
      Label         = '清空并解包 private-assets 卷'
      Arguments     = $volumeStepArguments
      StdinFile     = $null
      WaitDatabase  = $false
    },
    [pscustomobject]@{
      Label         = '启动全部服务'
      Arguments     = [string[]]($composeArgs + @('up', '-d'))
      StdinFile     = $null
      WaitDatabase  = $false
    }
  )

  # 4. DryRun：打印计划后退出，不调用 docker
  if ($DryRun) {
    Write-Step 'DryRun：计划操作（不会执行）'
    foreach ($step in $steps) {
      Write-Info ('- {0}' -f $step.Label)
      if (-not [string]::IsNullOrWhiteSpace([string]$step.StdinFile)) {
        Write-Host ('       {0} < "{1}"' -f (Format-CommandLine 'docker' $step.Arguments), $step.StdinFile) -ForegroundColor DarkGray
      } else {
        Write-Host ('       {0}' -f (Format-CommandLine 'docker' $step.Arguments)) -ForegroundColor DarkGray
      }
      if ($step.WaitDatabase) {
        Write-Host ('       （随后轮询 pg_isready，最多 60 秒）') -ForegroundColor DarkGray
      }
    }
    if (-not $SkipReadyCheck) {
      Write-Info ('- 等待 API 就绪：{0}（最多 {1} 秒）' -f $ReadyUrl, $ReadyTimeoutSeconds)
    }
    Write-Step 'DryRun 完成：未停止服务、未改动数据库、未改动卷'
    Write-Info ('确认执行请在原命令上追加 -Confirm。')
    exit 0
  }

  # 5. 必须显式 -Confirm 才执行破坏性操作
  if (-not $Confirm) {
    Write-Host ''
    Write-Host '拒绝执行：恢复会覆盖数据库并清空 private-assets 卷，必须显式传入 -Confirm。' -ForegroundColor Red
    Write-Info '建议先执行 -DryRun 演练并确认 manifest 校验结果。'
    Write-Info ('确认后执行：{0} -DatabaseDump "{1}" -AssetsArchive "{2}" -Confirm' -f $script:ScriptName, $dumpPath, $archivePath)
    exit 2
  }

  # 6. 破坏性执行
  Write-Host ''
  Write-Host '⚠ 即将执行破坏性恢复：' -ForegroundColor Red
  Write-Host ('   数据库 {0}/{1} 将被 pg_restore --clean 覆盖；卷 {2} 将被清空后解包。' -f $DatabaseUser, $DatabaseName, $VolumeName) -ForegroundColor Yellow
  Write-Host '   建议先停止外部写入，并确认已有可回滚的当前备份。' -ForegroundColor Yellow

  Assert-DockerAvailable

  foreach ($step in $steps) {
    Write-Step $step.Label
    if ($step.Label -like '停止应用服务*') { $script:servicesStopped = $true }
    Invoke-Process -FilePath 'docker' -Arguments ([string[]]$step.Arguments) -WorkingDirectory $composeDir -StdinFile $step.StdinFile | Out-Null
    if ($step.WaitDatabase) {
      Write-Info '等待数据库就绪（pg_isready）...'
      Wait-DatabaseReady $composePath $DatabaseContainer $DatabaseUser $DatabaseName 60
      Write-Ok '数据库已就绪'
    }
  }

  if (-not $SkipReadyCheck) {
    Write-Step ('等待 API 就绪：{0}' -f $ReadyUrl)
    if (-not (Wait-ApiReady $ReadyUrl $ReadyTimeoutSeconds)) {
      throw ('API 在 {0} 秒内未就绪：{1}；请检查 docker compose logs api --tail 100。' -f $ReadyTimeoutSeconds, $ReadyUrl)
    }
    Write-Ok 'API /readyz 返回 200'
  } else {
    Write-WarnLine '已指定 -SkipReadyCheck：跳过 /readyz 探测，请按下方清单人工验证。'
  }

  Write-VerificationChecklist $ReadyUrl $manifestPath $manifestObject
  Write-Host ''
  Write-Host '恢复完成。请逐项完成上面的验证清单，并把结果记录到运维日志。' -ForegroundColor Green
  exit 0
} catch {
  $failureMessage = $_.Exception.Message
  Write-ErrorLine $failureMessage
  Write-Host ''
  if ($script:servicesStopped) {
    Write-WarnLine '恢复未完成，尝试重新启动服务（数据可能已部分恢复）。'
    try {
      $recovery = Invoke-Process -FilePath 'docker' -Arguments ([string[]]@('compose', '-f', $composePath, 'up', '-d')) -WorkingDirectory $composeDir -AllowFailure
      if ($recovery.ExitCode -ne 0) {
        Write-ErrorLine ('重新启动服务失败；请手动执行 docker compose up -d。{0}' -f $recovery.StdErr.Trim())
      } else {
        Write-Info '服务已重新启动，但恢复未完成；请检查日志并评估是否需要重新执行恢复。'
      }
    } catch {
      Write-ErrorLine ('重新启动服务失败；请手动执行 docker compose up -d。{0}' -f $_.Exception.Message)
    }
  } else {
    Write-Info '尚未停止服务，系统应保持恢复前的运行状态。'
  }
  Write-Host '排查建议：核对 manifest 与文件、确认卷名与数据库账号，必要时先 -DryRun 再重试。' -ForegroundColor Yellow
  exit 1
}
