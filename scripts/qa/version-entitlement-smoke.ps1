param([switch]$KeepFixture)

# 版本权益与下载额度冒烟脚本。
# 兼容 Windows PowerShell 5.1、PowerShell 7 和 Linux：
# - 上传文件使用外部 curl（Windows 用 curl.exe），检查退出码与 HTTP 状态后 ConvertFrom-Json；
# - 路径全部用 Join-Path 拼接，临时目录用 [IO.Path]::GetTempPath()；
# - 文件保持 UTF-8 BOM + LF，保证 Windows PowerShell 5.1 能正确读取中文。

$ErrorActionPreference = 'Stop'
$base = 'http://127.0.0.1:48080'
$operatorPath = 'cp-7f3c'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
. (Join-Path $PSScriptRoot 'curl-upload.ps1')

$envMap = @{}
Get-Content (Join-Path $repoRoot '.env') | ForEach-Object {
  if ($_ -match '^([^#=]+)=(.*)$') { $envMap[$matches[1]] = $matches[2] }
}
$user = if ($envMap['ADMIN_USERNAME']) { $envMap['ADMIN_USERNAME'] } else { 'admin' }
$pass = if ($envMap['ADMIN_PASSWORD']) { $envMap['ADMIN_PASSWORD'] } else { 'admin' }
$headers = @{}
$productId = $null
$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
$tempDir = [System.IO.Path]::GetTempPath()
$tempFiles = @(
  (Join-Path $tempDir "nover-$stamp-1.0.0.txt"),
  (Join-Path $tempDir "nover-$stamp-1.1.0.txt"),
  (Join-Path $tempDir "nover-$stamp-2.0.0-draft.txt")
)

function Api([string]$method, [string]$path, $body = $null, $customHeaders = $null) {
  $params = @{ Method = $method; Uri = "$base$path" }
  if ($customHeaders) { $params.Headers = $customHeaders }
  if ($null -ne $body) {
    $params.ContentType = 'application/json; charset=utf-8'
    $params.Body = [System.Text.Encoding]::UTF8.GetBytes(($body | ConvertTo-Json -Depth 8))
  }
  Invoke-RestMethod @params
}

Push-Location $repoRoot
try {
  $summary = [ordered]@{}
  $summary.health = (Api GET '/readyz').status
  $login = Api POST "/api/v1/$operatorPath/login" @{ username = $user; password = $pass }
  $headers = @{ Authorization = "Bearer $($login.access_token)" }
  $summary.operator_role = $login.session.role

  $product = Api POST "/api/v1/$operatorPath/products" @{
    name = "版本权益验收-$stamp"
    description = '版本级交付自动验收商品'
    detail = '验证基础版本与有效期内更新版本交付。'
    status = 'DRAFT'
  } $headers
  $productId = $product.id

  $release100 = Api POST "/api/v1/$operatorPath/products/$productId/releases" @{ version = '1.0.0'; changelog = '首个可交付版本' } $headers
  Set-Content -LiteralPath $tempFiles[0] -Value 'NOVER release 1.0.0 acceptance file' -Encoding utf8
  $asset100 = Invoke-CurlFileUpload -Uri "$base/api/v1/$operatorPath/products/$productId/assets" -FilePath $tempFiles[0] -Headers $headers -Fields @{ release_id = $release100.id }

  $product = Api POST "/api/v1/$operatorPath/products/$productId/status" @{ status = 'ACTIVE' } $headers
  $release100 = Api POST "/api/v1/$operatorPath/products/$productId/releases/$($release100.id)/status" @{ status = 'PUBLISHED' } $headers
  $publishedAt = $release100.published_at
  $release100Again = Api POST "/api/v1/$operatorPath/products/$productId/releases/$($release100.id)/status" @{ status = 'PUBLISHED' } $headers
  $issued = Api POST "/api/v1/$operatorPath/products/$productId/codes" @{ count = 1; code_ttl_days = 7; access_days = 30; max_downloads = 10; external_order_ref = "acceptance-$stamp"; release_id = $release100.id } $headers
  $redeem = Api POST '/api/v1/redeem' @{ code = $issued.codes[0] }
  $buyerHeaders = @{ Authorization = "Bearer $($redeem.access_token)" }
  $delivery100 = Api GET '/api/v1/delivery' $null $buyerHeaders
  # 兑换后立即读取额度字段：max_downloads 与 downloads_used 必须由交付接口返回。
  $quotaBeforeTicket = $delivery100

  $release110 = Api POST "/api/v1/$operatorPath/products/$productId/releases" @{ version = '1.1.0'; changelog = '后续免费更新版本' } $headers
  Set-Content -LiteralPath $tempFiles[1] -Value 'NOVER release 1.1.0 acceptance file' -Encoding utf8
  $asset110 = Invoke-CurlFileUpload -Uri "$base/api/v1/$operatorPath/products/$productId/assets" -FilePath $tempFiles[1] -Headers $headers -Fields @{ release_id = $release110.id }
  $release110 = Api POST "/api/v1/$operatorPath/products/$productId/releases/$($release110.id)/status" @{ status = 'PUBLISHED' } $headers

  $draft200 = Api POST "/api/v1/$operatorPath/products/$productId/releases" @{ version = '2.0.0'; changelog = '未发布草稿，不应交付' } $headers
  Set-Content -LiteralPath $tempFiles[2] -Value 'NOVER release 2.0.0 draft file' -Encoding utf8
  $asset200 = Invoke-CurlFileUpload -Uri "$base/api/v1/$operatorPath/products/$productId/assets" -FilePath $tempFiles[2] -Headers $headers -Fields @{ release_id = $draft200.id }

  $delivery110 = Api GET '/api/v1/delivery' $null $buyerHeaders
  $withdrawCode = ''
  try {
    Api POST "/api/v1/$operatorPath/products/$productId/releases/$($release100.id)/status" @{ status = 'DRAFT' } $headers | Out-Null
    $withdrawCode = 'unexpected_success'
  } catch {
    $withdrawCode = if ($_.ErrorDetails.Message) { (($_.ErrorDetails.Message | ConvertFrom-Json).code) } else { 'http_error' }
  }

  $ticket = Api POST "/api/v1/assets/$($asset110.id)/ticket" $null $buyerHeaders
  # 创建下载票据会消耗一次额度，再次读取交付接口时 downloads_used 应加 1。
  $deliveryAfterTicket = Api GET '/api/v1/delivery' $null $buyerHeaders
  $draftTicketCode = ''
  try {
    Api POST "/api/v1/assets/$($asset200.id)/ticket" $null $buyerHeaders | Out-Null
    $draftTicketCode = 'unexpected_success'
  } catch {
    $draftTicketCode = if ($_.ErrorDetails.Message) { (($_.ErrorDetails.Message | ConvertFrom-Json).code) } else { 'http_error' }
  }
  $filesBeforeDelete = Api GET "/api/v1/$operatorPath/products/$productId/assets" $null $headers
  $publishedDeleteCode = ''
  try {
    Invoke-RestMethod -Method Delete -Uri "$base/api/v1/$operatorPath/products/$productId/assets/$($asset100.id)" -Headers $headers | Out-Null
    $publishedDeleteCode = 'unexpected_success'
  } catch {
    $publishedDeleteCode = if ($_.ErrorDetails.Message) { (($_.ErrorDetails.Message | ConvertFrom-Json).code) } else { 'http_error' }
  }
  Invoke-RestMethod -Method Delete -Uri "$base/api/v1/$operatorPath/products/$productId/assets/$($asset200.id)" -Headers $headers | Out-Null
  $filesAfterDelete = Api GET "/api/v1/$operatorPath/products/$productId/assets" $null $headers

  $summary.product_status = $product.status
  $summary.base_release = $delivery100.base_version
  $summary.initial_assets = @($delivery100.assets).Count
  $summary.latest_release = $delivery110.product.current_release_version
  $summary.updated_assets = @($delivery110.assets).Count
  $summary.visible_versions = @($delivery110.assets | ForEach-Object { $_.release_version } | Where-Object { $_ } | Sort-Object -Unique)
  $summary.draft_hidden = -not (@($delivery110.assets.id) -contains $asset200.id)
  $summary.base_withdraw_result = $withdrawCode
  $summary.update_ticket_created = [bool]$ticket.url
  $summary.draft_ticket_result = $draftTicketCode
  $summary.repeat_publish_kept_timestamp = $publishedAt -eq $release100Again.published_at
  $summary.asset_list_count = @($filesBeforeDelete.items).Count
  $summary.published_file_delete_result = $publishedDeleteCode
  $summary.draft_file_deleted = -not (@($filesAfterDelete.items.id) -contains $asset200.id)
  $summary.max_downloads = $quotaBeforeTicket.max_downloads
  $summary.downloads_used_before_ticket = $quotaBeforeTicket.downloads_used
  $summary.downloads_used_after_ticket = $deliveryAfterTicket.downloads_used
  $summary.pass = (
    $summary.health -eq 'ready' -and
    $summary.base_release -eq '1.0.0' -and
    $summary.initial_assets -eq 1 -and
    $summary.latest_release -eq '1.1.0' -and
    $summary.updated_assets -eq 2 -and
    $summary.draft_hidden -and
    $withdrawCode -eq 'release_in_use' -and
    $summary.update_ticket_created -and
    $draftTicketCode -eq 'asset_not_found' -and
    $summary.repeat_publish_kept_timestamp -and
    $summary.asset_list_count -eq 3 -and
    $publishedDeleteCode -eq 'asset_immutable' -and
    $summary.draft_file_deleted -and
    $summary.max_downloads -eq 10 -and
    $summary.downloads_used_before_ticket -eq 0 -and
    $summary.downloads_used_after_ticket -eq ($summary.downloads_used_before_ticket + 1)
  )
  if ($KeepFixture) {
    $summary.fixture = [ordered]@{
      product_id = $productId
      entitlement_token = $redeem.access_token
      base_asset_id = $asset100.id
      update_asset_id = $asset110.id
      draft_asset_id = $asset200.id
    }
  }
  $summary | ConvertTo-Json -Depth 8
} finally {
  if ($productId -and -not $KeepFixture) {
    $keys = docker compose exec -T db psql -U nover -d nover -Atc "SELECT object_key FROM assets WHERE product_id='$productId';"
    foreach ($key in @($keys)) {
      if ($key) { docker compose exec -T api rm -f "/var/lib/nover/files/$key" | Out-Null }
    }
    $sql = @"
BEGIN;
DELETE FROM download_events WHERE entitlement_id IN (SELECT id FROM entitlements WHERE product_id='$productId');
DELETE FROM download_tickets WHERE entitlement_id IN (SELECT id FROM entitlements WHERE product_id='$productId');
UPDATE redeem_codes SET entitlement_id=NULL WHERE product_id='$productId';
DELETE FROM entitlements WHERE product_id='$productId';
DELETE FROM redeem_codes WHERE product_id='$productId';
DELETE FROM assets WHERE product_id='$productId';
DELETE FROM product_releases WHERE product_id='$productId';
DELETE FROM product_images WHERE product_id='$productId';
DELETE FROM admin_events WHERE resource_id='$productId' OR metadata_json->>'product_id'='$productId';
DELETE FROM products WHERE id='$productId';
COMMIT;
"@
    $sql | docker compose exec -T db psql -v ON_ERROR_STOP=1 -U nover -d nover | Out-Null
  }
  Remove-Item -LiteralPath $tempFiles -Force -ErrorAction SilentlyContinue
  Pop-Location
}
