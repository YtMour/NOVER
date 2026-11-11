param([switch]$KeepFixture)

$ErrorActionPreference = 'Stop'
$base = 'http://127.0.0.1:48080'
$operatorPath = 'cp-7f3c'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$envMap = @{}
Get-Content (Join-Path $repoRoot '.env') | ForEach-Object { if ($_ -match '^([^#=]+)=(.*)$') { $envMap[$matches[1]] = $matches[2] } }
$adminUser = if ($envMap['ADMIN_USERNAME']) { $envMap['ADMIN_USERNAME'] } else { 'admin' }
$adminPass = if ($envMap['ADMIN_PASSWORD']) { $envMap['ADMIN_PASSWORD'] } else { 'admin' }
$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$username = "qa$stamp"
$password = "Qa-only-$stamp!"
$file = Join-Path $env:TEMP "nover-tenant-$stamp.txt"
$imageFile = Join-Path $env:TEMP "nover-tenant-$stamp.png"
$application = $null
$product = $null
$asset = $null

function Api([string]$method, [string]$path, $body = $null, $headers = $null) {
  $params = @{ Method = $method; Uri = "$base$path" }
  if ($headers) { $params.Headers = $headers }
  if ($null -ne $body) { $params.ContentType = 'application/json'; $params.Body = ($body | ConvertTo-Json -Depth 8) }
  Invoke-RestMethod @params
}

function ErrorCode([scriptblock]$request) {
  try { & $request | Out-Null; return 'unexpected_success' }
  catch { if ($_.ErrorDetails.Message) { return (($_.ErrorDetails.Message | ConvertFrom-Json).code) }; return 'http_error' }
}

Push-Location $repoRoot
try {
  $admin = Api POST "/api/v1/$operatorPath/login" @{ username = $adminUser; password = $adminPass }
  $adminHeaders = @{ Authorization = "Bearer $($admin.access_token)" }
  $application = Api POST '/api/v1/join-applications' @{
    organization_name = "QA Space $stamp"; contact_name = 'QA Merchant'; email = "$username@example.test"
    desired_username = $username; expected_password = $password; use_case = 'Synthetic tenant delivery acceptance test.'
  }
  $pending = Api POST "/api/v1/$operatorPath/login" @{ username = $username; password = $password }
  $pendingHeaders = @{ Authorization = "Bearer $($pending.access_token)" }
  $pendingWrite = ErrorCode { Api POST "/api/v1/$operatorPath/products" @{ name = 'Blocked'; status = 'DRAFT' } $pendingHeaders }
  Api POST "/api/v1/$operatorPath/applications/$($application.id)/review" @{ action = 'APPROVE' } $adminHeaders | Out-Null
  $merchant = Api POST "/api/v1/$operatorPath/login" @{ username = $username; password = $password }
  $merchantHeaders = @{ Authorization = "Bearer $($merchant.access_token)" }
  $product = Api POST "/api/v1/$operatorPath/products" @{ name = "QA Merchant Product $stamp"; description = 'Synthetic merchant delivery'; detail = 'Tenant-isolated test'; status = 'DRAFT' } $merchantHeaders
  [IO.File]::WriteAllBytes($imageFile, [Convert]::FromBase64String('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9aO2sAAAAASUVORK5CYII='))
  $image = Invoke-RestMethod -Method Post -Uri "$base/api/v1/$operatorPath/products/$($product.id)/images" -Headers $merchantHeaders -Form @{ file = Get-Item $imageFile; alt_text = 'Synthetic preview' }
  Api PUT "/api/v1/$operatorPath/products/$($product.id)" @{ name = $product.name; description = $product.description; detail = "![Synthetic preview](/api/v1/products/$($product.id)/images/$($image.id))" } $merchantHeaders | Out-Null
  $release = Api POST "/api/v1/$operatorPath/products/$($product.id)/releases" @{ version = '1.0.0'; changelog = 'First merchant release' } $merchantHeaders
  Set-Content -LiteralPath $file -Value 'Synthetic tenant delivery file' -Encoding utf8
  $asset = Invoke-RestMethod -Method Post -Uri "$base/api/v1/$operatorPath/products/$($product.id)/assets" -Headers $merchantHeaders -Form @{ release_id = $release.id; file = Get-Item $file }
  Api POST "/api/v1/$operatorPath/products/$($product.id)/status" @{ status = 'ACTIVE' } $merchantHeaders | Out-Null
  Api POST "/api/v1/$operatorPath/products/$($product.id)/releases/$($release.id)/status" @{ status = 'PUBLISHED' } $merchantHeaders | Out-Null
  $issued = Api POST "/api/v1/$operatorPath/products/$($product.id)/codes" @{ count = 1; release_id = $release.id; code_ttl_days = 7; access_days = 30; max_downloads = 2; external_order_ref = "qa-$stamp" } $merchantHeaders
  $redeemed = Api POST '/api/v1/redeem' @{ code = $issued.codes[0] }
  $delivery = Api GET '/api/v1/delivery' $null @{ Authorization = "Bearer $($redeemed.access_token)" }
  $merchantSpace = Api GET "/api/v1/spaces/$($merchant.session.tenant_slug)/products"
  $spaceImage = "/api/v1/spaces/$($merchant.session.tenant_slug)/products/$($product.id)/images/$($image.id)"
  $imageResponse = Invoke-WebRequest -Method Get -Uri "$base$spaceImage"
  $wrongSpaceImage = ErrorCode { Api GET "/api/v1/spaces/default/products/$($product.id)/images/$($image.id)" }
  $defaultSpace = Api GET '/api/v1/products'
  $adminFiles = ErrorCode { Api GET "/api/v1/$operatorPath/products/$($product.id)/assets" $null $adminHeaders }
  $result = [ordered]@{
    pending_status = $pending.session.membership_status
    pending_write_result = $pendingWrite
    approved_status = $merchant.session.membership_status
    merchant_space_slug = $merchant.session.tenant_slug
    merchant_product_visible = @($merchantSpace.items.id) -contains $product.id
    merchant_image_url = $merchantSpace.items[0].images[0].url -eq $spaceImage
    merchant_markdown_image_url = $merchantSpace.items[0].detail.Contains($spaceImage)
    merchant_image_status = $imageResponse.StatusCode
    wrong_space_image_result = $wrongSpaceImage
    default_product_hidden = -not (@($defaultSpace.items.id) -contains $product.id)
    default_operator_asset_result = $adminFiles
    redeem_product = $redeemed.product_id
    delivered_asset = @($delivery.assets.id) -contains $asset.id
  }
  $result.pass = $result.pending_status -eq 'PENDING' -and $pendingWrite -eq 'operator_permission_required' -and $result.approved_status -eq 'ACTIVE' -and $result.merchant_product_visible -and $result.merchant_image_url -and $result.merchant_markdown_image_url -and $result.merchant_image_status -eq 200 -and $wrongSpaceImage -eq 'image_not_found' -and $result.default_product_hidden -and $adminFiles -eq 'product_not_found' -and $result.redeem_product -eq $product.id -and $result.delivered_asset
  if ($KeepFixture) { $result.fixture = @{ product_id = $product.id; tenant_slug = $merchant.session.tenant_slug } }
  $result | ConvertTo-Json -Depth 7
  if (-not $result.pass) { throw 'Tenant delivery acceptance failed.' }
} finally {
  if ($application -and -not $KeepFixture) {
    $cleanupRow = docker compose exec -T db psql -U nover -d nover -Atc "SELECT tenant_id || '|' || operator_user_id FROM join_applications WHERE id='$($application.id)';"
    $tenantId, $operatorUserId = ($cleanupRow.Trim() -split '\|', 2)
    if (-not $tenantId -or -not $operatorUserId) { throw 'QA fixture cleanup identity missing.' }
    if ($asset) {
      $key = docker compose exec -T db psql -U nover -d nover -Atc "SELECT object_key FROM assets WHERE id='$($asset.id)';"
      if ($key) { docker compose exec -T api rm -f "/var/lib/nover/files/$key" | Out-Null }
    }
    if ($image) {
      $imageKey = docker compose exec -T db psql -U nover -d nover -Atc "SELECT object_key FROM product_images WHERE id='$($image.id)';"
      if ($imageKey) { docker compose exec -T api rm -f "/var/lib/nover/files/$imageKey" | Out-Null }
    }
    $sql = @"
BEGIN;
DELETE FROM download_events WHERE tenant_id='$tenantId';
DELETE FROM download_tickets WHERE tenant_id='$tenantId';
UPDATE redeem_codes SET entitlement_id=NULL WHERE tenant_id='$tenantId';
DELETE FROM entitlements WHERE tenant_id='$tenantId';
DELETE FROM redeem_codes WHERE tenant_id='$tenantId';
DELETE FROM assets WHERE tenant_id='$tenantId';
DELETE FROM product_images WHERE tenant_id='$tenantId';
DELETE FROM product_releases WHERE tenant_id='$tenantId';
DELETE FROM products WHERE tenant_id='$tenantId';
DELETE FROM admin_events WHERE tenant_id='$tenantId';
DELETE FROM operator_sessions WHERE tenant_id='$tenantId';
DELETE FROM join_applications WHERE id='$($application.id)';
DELETE FROM tenant_memberships WHERE tenant_id='$tenantId';
DELETE FROM operator_users WHERE id='$operatorUserId';
DELETE FROM tenants WHERE id='$tenantId';
COMMIT;
"@
    $sql | docker compose exec -T db psql -v ON_ERROR_STOP=1 -U nover -d nover | Out-Null
  }
  Remove-Item -LiteralPath $file -Force -ErrorAction SilentlyContinue
  Remove-Item -LiteralPath $imageFile -Force -ErrorAction SilentlyContinue
  Pop-Location
}
