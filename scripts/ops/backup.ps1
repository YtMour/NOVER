#Requires -Version 5.1

<#
.SYNOPSIS
  NOVER 成对备份：PostgreSQL 逻辑备份（pg_dump -Fc）+ private-assets 卷归档 + manifest（含 SHA-256）。

.DESCRIPTION
  在仓库根目录执行（Windows PowerShell 5.1 / PowerShell 7 / Linux pwsh 均可）：

    .\scripts\ops\backup.ps1
    .\scripts\ops\backup.ps1 -DryRun
    .\scripts\ops\backup.ps1 -OutputDir D:\backups\nover -Timestamp 20261119-134500
    powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\ops\backup.ps1

  每次备份生成同一时间戳的三个文件（数据库与卷必须成对保存）：

    <OutputDir>/nover-<timestamp>.dump
    <OutputDir>/private-assets-<timestamp>.tar.gz
    <OutputDir>/manifest-<timestamp>.json

  安全边界：只读数据库与卷；不执行 docker compose down/rm，不修改卷内容，不删除历史备份。
  -DryRun 只打印将要执行的命令，不调用 docker、不写任何文件。
  文件保持 UTF-8 BOM + LF（Windows PowerShell 5.1 中文兼容）。
#>
[CmdletBinding()]
param(
  [string]$OutputDir = './backups',
  [string]$ComposeFile,
  [string]$ProjectName,
  [string]$DatabaseContainer = 'db',
  [string]$VolumeName,
  [string]$DatabaseUser,
  [string]$DatabaseName,
  [string]$HelperImage = 'alpine:3.20',
  [string]$Timestamp,
  [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
$script:ToolVersion = '0.4.0'
$script:ScriptName = 'scripts/ops/backup.ps1'

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

function New-BackupStamp([string]$Explicit) {
  if (-not [string]::IsNullOrWhiteSpace($Explicit)) {
    if ($Explicit -notmatch '^[0-9]{8}-[0-9]{6}$') {
      throw ('-Timestamp 必须形如 20261119-134500，实际为：{0}' -f $Explicit)
    }
    return $Explicit
  }
  return [System.DateTime]::Now.ToString('yyyyMMdd-HHmmss', [System.Globalization.CultureInfo]::InvariantCulture)
}

function ConvertTo-DockerMountPath([string]$Path) {
  # Docker Desktop 对 Windows 盘符路径使用正斜杠最稳妥；Linux 路径本来就使用正斜杠。
  return $Path.Replace('\', '/')
}

# ---------------------------------------------------------------------------
# 进程执行（PowerShell 5.1 兼容）
# 说明：二进制输出（pg_dump）不能用 PowerShell 的 ">" 重定向（会按文本编码损坏文件），
#       因此统一通过 ProcessStartInfo 直接复制原始字节流。
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

function Get-GitCommit([string]$RepoRoot) {
  if (-not (Get-Command 'git' -ErrorAction SilentlyContinue)) { return 'unknown' }
  $result = Invoke-Process -FilePath 'git' -Arguments @('-C', $RepoRoot, 'rev-parse', '--short=12', 'HEAD') -CaptureStdout -AllowFailure
  if ($result.ExitCode -ne 0) { return 'unknown' }
  return $result.StdOut.Trim()
}

function Get-FileSha256([string]$Path) {
  return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
}

# ---------------------------------------------------------------------------
# 主流程
# ---------------------------------------------------------------------------
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$workingDirectory = (Get-Location).Path
$createdFiles = New-Object System.Collections.ArrayList

try {
  # 1. 解析并校验基础路径
  $composePath = if ([string]::IsNullOrWhiteSpace($ComposeFile)) {
    Join-Path $repoRoot 'docker-compose.yml'
  } else {
    Resolve-FullPath $ComposeFile $workingDirectory
  }
  if (-not (Test-Path -LiteralPath $composePath -PathType Leaf)) {
    throw ('未找到 compose 文件：{0}' -f $composePath)
  }
  $composeDir = Split-Path -Parent $composePath

  $outputPath = Resolve-FullPath $OutputDir $workingDirectory
  $stamp = New-BackupStamp $Timestamp

  $databaseFileName = 'nover-{0}.dump' -f $stamp
  $assetsFileName = 'private-assets-{0}.tar.gz' -f $stamp
  $manifestFileName = 'manifest-{0}.json' -f $stamp
  $databasePath = Join-Path $outputPath $databaseFileName
  $assetsPath = Join-Path $outputPath $assetsFileName
  $manifestPath = Join-Path $outputPath $manifestFileName

  # 2. 从 .env 与 compose 目录推导默认值（可用参数覆盖）
  $envPath = Join-Path $composeDir '.env'
  $envMap = Read-DotEnv $envPath

  if ([string]::IsNullOrWhiteSpace($ProjectName)) {
    $ProjectName = Get-ComposeProjectName $composePath
  }
  if ([string]::IsNullOrWhiteSpace($VolumeName)) {
    $VolumeName = '{0}_private-assets' -f $ProjectName
  }
  if ([string]::IsNullOrWhiteSpace($DatabaseUser)) {
    $DatabaseUser = Get-EnvOrDefault $envMap 'POSTGRES_USER' 'nover'
  }
  if ([string]::IsNullOrWhiteSpace($DatabaseName)) {
    $DatabaseName = Get-EnvOrDefault $envMap 'POSTGRES_DB' 'nover'
  }

  Write-Host ('NOVER 备份（{0}）' -f $script:ToolVersion) -ForegroundColor Cyan
  Write-Info ('仓库根目录：{0}' -f $repoRoot)
  Write-Info ('compose 文件：{0}' -f $composePath)
  Write-Info ('输出目录：{0}' -f $outputPath)
  Write-Info ('时间戳：{0}' -f $stamp)
  Write-Info ('项目名：{0}' -f $ProjectName)
  Write-Info ('数据库：{0} / {1}（容器 {2}，pg_dump -Fc）' -f $DatabaseUser, $DatabaseName, $DatabaseContainer)
  Write-Info ('资产卷：{0}' -f $VolumeName)
  if (Test-Path -LiteralPath $envPath) {
    Write-Info ('环境文件：{0}' -f $envPath)
  } else {
    Write-WarnLine ('未找到 {0}；docker compose 可能缺少 POSTGRES_PASSWORD 等变量。' -f $envPath)
  }
  if ($DryRun) {
    Write-WarnLine 'DryRun 模式：只打印计划命令，不调用 docker、不写任何文件。'
  }

  # 3. 目标文件必须不存在，避免覆盖历史备份
  foreach ($target in @($databasePath, $assetsPath, $manifestPath)) {
    if (Test-Path -LiteralPath $target) {
      throw ('目标文件已存在：{0}；请更换 -OutputDir 或 -Timestamp 以保留历史备份。' -f $target)
    }
  }

  # 4. 计划命令（DryRun 与实际执行共用同一份定义，避免文档与实现漂移）
  $composeCommand = New-Object System.Collections.ArrayList
  [void]$composeCommand.AddRange([string[]]@('compose', '-f', $composePath))

  $dumpArguments = [string[]]@($composeCommand) + @('exec', '-T', $DatabaseContainer, 'pg_dump', '-U', $DatabaseUser, '-d', $DatabaseName, '-Fc')
  $tarArguments = [string[]]@(
    'run', '--rm',
    '-v', ('{0}:/data:ro' -f $VolumeName),
    '-v', ('{0}:/backup' -f (ConvertTo-DockerMountPath $outputPath)),
    $HelperImage,
    'tar', 'czf', ('/backup/{0}' -f $assetsFileName), '-C', '/data', '.'
  )

  if ($DryRun) {
    Write-Step 'DryRun：计划命令'
    if (-not (Test-Path -LiteralPath $outputPath)) {
      Write-Info ('将创建输出目录：{0}' -f $outputPath)
    }
    Write-Info ('1) 数据库导出（脚本直接把 pg_dump 的原始二进制输出写入文件，不使用 PowerShell ">" 重定向）：')
    Write-Host ('       {0}' -f (Format-CommandLine 'docker' $dumpArguments)) -ForegroundColor DarkGray
    Write-Host ('       二进制标准输出 -> "{0}"' -f $databasePath) -ForegroundColor DarkGray
    Write-Info ('2) 资产卷归档（卷只读挂载，输出目录可写挂载）：')
    Write-Host ('       {0}' -f (Format-CommandLine 'docker' $tarArguments)) -ForegroundColor DarkGray
    Write-Info ('3) 计算两个文件的 SHA-256 并写入 {0}：' -f $manifestFileName)
    Write-Host ('       {0}' -f $manifestPath) -ForegroundColor DarkGray
    Write-Info '4) 打印备份结果与恢复提示（manifest 版本、时间、项目、卷、数据库、文件大小、SHA-256）。'
    Write-Step 'DryRun 完成：未执行任何备份动作'
    Write-Info ('实际执行：{0} -OutputDir "{1}"' -f $script:ScriptName, $outputPath)
    exit 0
  }

  # 5. 实际备份
  Assert-DockerAvailable

  $volumeProbe = Invoke-Process -FilePath 'docker' -Arguments @('volume', 'inspect', $VolumeName) -CaptureStdout -AllowFailure
  if ($volumeProbe.ExitCode -ne 0) {
    throw ('卷不存在：{0}；请用 docker volume ls 确认卷名，或用 -VolumeName 指定。{1}' -f $VolumeName, $volumeProbe.StdErr.Trim())
  }

  if (-not (Test-Path -LiteralPath $outputPath)) {
    New-Item -ItemType Directory -Path $outputPath -Force | Out-Null
    Write-Info ('已创建输出目录：{0}' -f $outputPath)
  }

  Write-Step ('1/4 导出数据库 {0}' -f $DatabaseName)
  Invoke-Process -FilePath 'docker' -Arguments $dumpArguments -WorkingDirectory $composeDir -StdoutFile $databasePath | Out-Null
  [void]$createdFiles.Add($databasePath)
  $dumpBytes = (Get-Item -LiteralPath $databasePath).Length
  if ($dumpBytes -le 0) {
    throw ('pg_dump 输出为空：{0}' -f $databasePath)
  }
  Write-Ok ('{0}（{1} 字节）' -f $databaseFileName, $dumpBytes)

  Write-Step ('2/4 打包资产卷 {0}' -f $VolumeName)
  Invoke-Process -FilePath 'docker' -Arguments $tarArguments -WorkingDirectory $composeDir | Out-Null
  [void]$createdFiles.Add($assetsPath)
  $assetsBytes = (Get-Item -LiteralPath $assetsPath).Length
  if ($assetsBytes -le 0) {
    throw ('资产归档为空：{0}' -f $assetsPath)
  }
  Write-Ok ('{0}（{1} 字节）' -f $assetsFileName, $assetsBytes)

  Write-Step '3/4 计算 SHA-256 并写入 manifest'
  $dumpSha = Get-FileSha256 $databasePath
  $assetsSha = Get-FileSha256 $assetsPath
  Write-Ok ('数据库 SHA-256：{0}' -f $dumpSha)
  Write-Ok ('资产卷 SHA-256：{0}' -f $assetsSha)

  $gitCommit = Get-GitCommit $repoRoot
  $manifest = [ordered]@{
    manifest_version = 1
    tool             = $script:ScriptName
    tool_version     = $script:ToolVersion
    created_at       = [System.DateTime]::UtcNow.ToString('o', [System.Globalization.CultureInfo]::InvariantCulture)
    created_at_local = [System.DateTime]::Now.ToString('o', [System.Globalization.CultureInfo]::InvariantCulture)
    host             = [System.Environment]::MachineName
    repo             = [ordered]@{
      root       = $repoRoot
      git_commit = $gitCommit
    }
    compose          = [ordered]@{
      file    = $composePath
      project = $ProjectName
    }
    database         = [ordered]@{
      container = $DatabaseContainer
      user      = $DatabaseUser
      name      = $DatabaseName
      format    = 'custom (pg_dump -Fc)'
      file      = $databaseFileName
      bytes     = $dumpBytes
      sha256    = $dumpSha
    }
    assets           = [ordered]@{
      volume = $VolumeName
      file   = $assetsFileName
      bytes  = $assetsBytes
      sha256 = $assetsSha
    }
    restore          = [ordered]@{
      script = 'scripts/ops/restore.ps1'
      hint   = 'restore.ps1 -DatabaseDump <dump> -AssetsArchive <tar.gz> -Confirm'
    }
  }
  $manifestJson = $manifest | ConvertTo-Json -Depth 8
  $manifestJson = $manifestJson -replace "`r`n", "`n"
  $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllText($manifestPath, ($manifestJson + "`n"), $utf8NoBom)
  [void]$createdFiles.Add($manifestPath)

  # 回读校验，避免写出损坏 JSON
  $written = Get-Content -LiteralPath $manifestPath -Raw -Encoding UTF8 | ConvertFrom-Json
  if ($written.database.sha256 -ne $dumpSha -or $written.assets.sha256 -ne $assetsSha) {
    throw ('manifest 回读校验失败：{0}' -f $manifestPath)
  }
  Write-Ok ('{0} 已写入并通过回读校验' -f $manifestFileName)

  Write-Step '4/4 备份完成'
  Write-Host ''
  Write-Host '================ 备份结果 ================' -ForegroundColor Cyan
  Write-Host ('  数据库：  {0}' -f $databasePath)
  Write-Host ('  资产卷：  {0}' -f $assetsPath)
  Write-Host ('  manifest：{0}' -f $manifestPath)
  Write-Host ('  项目/卷： {0} / {1}' -f $ProjectName, $VolumeName)
  Write-Host ('  数据库：  {0} / {1}' -f $DatabaseUser, $DatabaseName)
  Write-Host ''
  Write-Host '请把三个文件复制到独立存储（异地/对象存储），并定期演练恢复。' -ForegroundColor Yellow
  Write-Host ('恢复命令：{0} -DatabaseDump "{1}" -AssetsArchive "{2}" -Confirm' -f 'scripts/ops/restore.ps1', $databasePath, $assetsPath)
  exit 0
} catch {
  $failureMessage = $_.Exception.Message
  $removed = 0
  foreach ($file in @($createdFiles)) {
    if ([string]::IsNullOrWhiteSpace([string]$file)) { continue }
    if (Test-Path -LiteralPath $file) {
      Remove-Item -LiteralPath $file -Force -ErrorAction SilentlyContinue
      if (-not (Test-Path -LiteralPath $file)) {
        $removed++
        Write-Info ('已清理未完成文件：{0}' -f $file)
      }
    }
  }
  Write-ErrorLine $failureMessage
  Write-Host ''
  Write-Host '备份失败：数据库与私有资产卷均未被修改。' -ForegroundColor Red
  if ($removed -gt 0) {
    Write-Host ('已删除 {0} 个未完成的输出文件，避免留下不完整的“成对备份”。' -f $removed) -ForegroundColor Yellow
  }
  Write-Host '排查建议：确认 docker compose 可用、db 容器与卷名正确、输出磁盘有足够空间。' -ForegroundColor Yellow
  exit 1
}
