param([switch]$KeepFixture)

$ErrorActionPreference = 'Stop'
$base = 'http://127.0.0.1:48080'
$operatorPath = 'cp-7f3c'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$envMap = @{}
Get-Content (Join-Path $repoRoot '.env') | ForEach-Object {
  if ($_ -match '^([^#=]+)=(.*)$') { $envMap[$matches[1]] = $matches[2] }
}
$user = if ($envMap['ADMIN_USERNAME']) { $envMap['ADMIN_USERNAME'] } else { 'admin' }
$pass = if ($envMap['ADMIN_PASSWORD']) { $envMap['ADMIN_PASSWORD'] } else { 'admin' }
$headers = @{}
$productId = $null
$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
$tempFiles = @(
  (Join-Path $env:TEMP "nover-$stamp-1.0.0.txt"),
  (Join-Path $env:TEMP "nover-$stamp-1.1.0.txt"),
  (Join-Path $env:TEMP "nover-$stamp-2.0.0-draft.txt")
)

function Api([string]$method, [string]$path, $body = $null, $customHeaders = $null) {
  $params = @{ Method = $method; Uri = "$base$path" }
  if ($customHeaders) { $params.Headers = $customHeaders }
  if ($null -ne $body) {
    $params.ContentType = 'application/json'
    $params.Body = ($body | ConvertTo-Json -Depth 8)
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
  $asset100 = Invoke-RestMethod -Method Post -Uri "$base/api/v1/$operatorPath/products/$productId/assets" -Headers $headers -Form @{ release_id = $release100.id; file = Get-Item $tempFiles[0] }

  $product = Api POST "/api/v1/$operatorPath/products/$productId/status" @{ status = 'ACTIVE' } $headers
  $release100 = Api POST "/api/v1/$operatorPath/products/$productId/releases/$($release100.id)/status" @{ status = 'PUBLISHED' } $headers
  $issued = Api POST "/api/v1/$operatorPath/products/$productId/codes" @{ count = 1; code_ttl_days = 7; access_days = 30; max_downloads = 10; external_order_ref = "acceptance-$stamp"; release_id = $release100.id } $headers
  $redeem = Api POST '/api/v1/redeem' @{ code = $issued.codes[0] }
  $buyerHeaders = @{ Authorization = "Bearer $($redeem.access_token)" }
  $delivery100 = Api GET '/api/v1/delivery' $null $buyerHeaders

  $release110 = Api POST "/api/v1/$operatorPath/products/$productId/releases" @{ version = '1.1.0'; changelog = '后续免费更新版本' } $headers
  Set-Content -LiteralPath $tempFiles[1] -Value 'NOVER release 1.1.0 acceptance file' -Encoding utf8
  $asset110 = Invoke-RestMethod -Method Post -Uri "$base/api/v1/$operatorPath/products/$productId/assets" -Headers $headers -Form @{ release_id = $release110.id; file = Get-Item $tempFiles[1] }
  $release110 = Api POST "/api/v1/$operatorPath/products/$productId/releases/$($release110.id)/status" @{ status = 'PUBLISHED' } $headers

  $draft200 = Api POST "/api/v1/$operatorPath/products/$productId/releases" @{ version = '2.0.0'; changelog = '未发布草稿，不应交付' } $headers
  Set-Content -LiteralPath $tempFiles[2] -Value 'NOVER release 2.0.0 draft file' -Encoding utf8
  $asset200 = Invoke-RestMethod -Method Post -Uri "$base/api/v1/$operatorPath/products/$productId/assets" -Headers $headers -Form @{ release_id = $draft200.id; file = Get-Item $tempFiles[2] }

  $delivery110 = Api GET '/api/v1/delivery' $null $buyerHeaders
  $withdrawCode = ''
  try {
    Api POST "/api/v1/$operatorPath/products/$productId/releases/$($release100.id)/status" @{ status = 'DRAFT' } $headers | Out-Null
    $withdrawCode = 'unexpected_success'
  } catch {
    $withdrawCode = if ($_.ErrorDetails.Message) { (($_.ErrorDetails.Message | ConvertFrom-Json).code) } else { 'http_error' }
  }

  $ticket = Api POST "/api/v1/assets/$($asset110.id)/ticket" $null $buyerHeaders
  $draftTicketCode = ''
  try {
    Api POST "/api/v1/assets/$($asset200.id)/ticket" $null $buyerHeaders | Out-Null
    $draftTicketCode = 'unexpected_success'
  } catch {
    $draftTicketCode = if ($_.ErrorDetails.Message) { (($_.ErrorDetails.Message | ConvertFrom-Json).code) } else { 'http_error' }
  }

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
  $summary.pass = (
    $summary.health -eq 'ready' -and
    $summary.base_release -eq '1.0.0' -and
    $summary.initial_assets -eq 1 -and
    $summary.latest_release -eq '1.1.0' -and
    $summary.updated_assets -eq 2 -and
    $summary.draft_hidden -and
    $withdrawCode -eq 'release_in_use' -and
    $summary.update_ticket_created -and
    $draftTicketCode -eq 'asset_not_found'
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
