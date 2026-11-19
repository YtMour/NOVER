# 跨平台 curl 上传辅助函数（dot-source 使用）。
#
# 背景：Windows PowerShell 5.1 的 Invoke-RestMethod 不支持 -Form（multipart），
# 旧版 .NET 也没有对应实现；因此冒烟脚本统一改用外部 curl 上传文件，
# 通过 $LASTEXITCODE 与 HTTP 状态码判断结果，再把响应 ConvertFrom-Json。
#
# 兼容性：Windows PowerShell 5.1 / PowerShell 7 / Linux（curl 或 curl.exe）。
# 文件保持 UTF-8 BOM + LF，保证 Windows PowerShell 5.1 能正确读取中文。

function Get-CurlCommand {
  if ($env:OS -eq 'Windows_NT') { return 'curl.exe' }
  return 'curl'
}

function Assert-CurlAvailable {
  $curl = Get-CurlCommand
  if (-not (Get-Command $curl -ErrorAction SilentlyContinue)) {
    throw "未找到 curl 命令（$curl）。Windows 10+ 自带 curl.exe；Linux 请先安装 curl。"
  }
  return $curl
}

# 以 multipart/form-data POST 上传一个文件，并返回解析后的 JSON 响应。
# -Fields 为普通表单字段（如 release_id、alt_text）；文件字段名固定为 file。
function Invoke-CurlFileUpload {
  [CmdletBinding()]
  param(
    [Parameter(Mandatory = $true)][string]$Uri,
    [Parameter(Mandatory = $true)][string]$FilePath,
    [hashtable]$Headers = @{},
    [hashtable]$Fields = @{}
  )

  $curl = Assert-CurlAvailable
  $bodyFile = [System.IO.Path]::GetTempFileName()
  try {
    # -o/-w 分开保存响应体与 HTTP 状态码，避免把状态码混进 JSON。
    $arguments = @('-sS', '-X', 'POST', $Uri, '-o', $bodyFile, '-w', '%{http_code}')
    foreach ($name in @($Headers.Keys)) {
      $arguments += @('-H', ('{0}: {1}' -f $name, $Headers[$name]))
    }
    foreach ($name in @($Fields.Keys)) {
      $arguments += @('-F', ('{0}={1}' -f $name, $Fields[$name]))
    }
    $arguments += @('-F', ('file=@' + $FilePath))

    $httpStatus = & $curl @arguments
    $exitCode = $LASTEXITCODE
    $body = ''
    if (Test-Path -LiteralPath $bodyFile) {
      $body = Get-Content -LiteralPath $bodyFile -Raw -ErrorAction SilentlyContinue
    }

    if ($exitCode -ne 0) {
      throw ('curl 上传失败：{0}（退出码 {1}）。响应：{2}' -f $Uri, $exitCode, $body)
    }

    $status = 0
    $statusText = [string](@($httpStatus) | Select-Object -Last 1)
    if (-not [int]::TryParse($statusText, [ref]$status)) { $status = 0 }
    if ($status -lt 200 -or $status -ge 300) {
      throw ('curl 上传失败：{0} 返回 HTTP {1}。响应：{2}' -f $Uri, $status, $body)
    }
    if ([string]::IsNullOrWhiteSpace($body)) {
      throw ('curl 上传失败：{0} 返回空响应。' -f $Uri)
    }

    return ($body | ConvertFrom-Json)
  } finally {
    Remove-Item -LiteralPath $bodyFile -Force -ErrorAction SilentlyContinue
  }
}
