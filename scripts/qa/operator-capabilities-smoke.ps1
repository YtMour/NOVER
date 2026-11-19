param([switch]$KeepFixture)

# v0.3.0 运营能力冒烟：商品资料与更新策略、退款、成员管理、账户安全、审计日志。
# 兼容 Windows PowerShell 5.1、PowerShell 7 和 Linux；文件保持 UTF-8 BOM + LF。

$ErrorActionPreference = 'Stop'
$base = 'http://127.0.0.1:48080'
$operatorPath = 'cp-7f3c'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
. (Join-Path $PSScriptRoot 'curl-upload.ps1')

$envMap = @{}
Get-Content (Join-Path $repoRoot '.env') | ForEach-Object {
  if ($_ -match '^([^#=]+)=(.*)$') { $envMap[$matches[1]] = $matches[2] }
}
$adminUser = if ($envMap['ADMIN_USERNAME']) { $envMap['ADMIN_USERNAME'] } else { 'admin' }
$adminPass = if ($envMap['ADMIN_PASSWORD']) { $envMap['ADMIN_PASSWORD'] } else { 'admin' }
$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$tempDir = [System.IO.Path]::GetTempPath()
$file100 = Join-Path $tempDir "nover-v3-$stamp-1.0.0.txt"
$file110 = Join-Path $tempDir "nover-v3-$stamp-1.1.0.txt"
$memberUsername = "qa$stamp"
$memberPassword = "QaMember-$stamp!"
$memberNewPassword = "QaMemberNew-$stamp!"
$productId = $null
$membershipId = $null
$memberUserId = $null
$memberToken = $null
$updateUntil = (Get-Date).ToUniversalTime().AddDays(-1).ToString("yyyy-MM-ddTHH:mm:ssZ")

function Api([string]$method, [string]$path, $body = $null, $customHeaders = $null) {
  $params = @{ Method = $method; Uri = "$base$path" }
  if ($customHeaders) { $params.Headers = $customHeaders }
  if ($null -ne $body) {
    $params.ContentType = 'application/json; charset=utf-8'
    $params.Body = [System.Text.Encoding]::UTF8.GetBytes(($body | ConvertTo-Json -Depth 8))
  }
  Invoke-RestMethod @params
}

function ErrorCode([scriptblock]$request) {
  try { & $request | Out-Null; return 'unexpected_success' }
  catch { if ($_.ErrorDetails.Message) { return (($_.ErrorDetails.Message | ConvertFrom-Json).code) }; return 'http_error' }
}

Push-Location $repoRoot
try {
  $summary = [ordered]@{}
  $summary.health = (Api GET '/readyz').status

  $admin = Api POST "/api/v1/$operatorPath/login" @{ username = $adminUser; password = $adminPass }
  $adminHeaders = @{ Authorization = "Bearer $($admin.access_token)" }
  $adminPermissions = @($admin.session.permissions)

  # --- 商品资料 v2 与更新策略 ---------------------------------------------
  $product = Api POST "/api/v1/$operatorPath/products" @{
    name = "v0.3 capability $stamp"
    description = '结构化商品资料与更新策略验收'
    detail = '用于验证亮点、授权、技术要求与更新策略。'
    highlights = @('亮点一', '亮点二')
    license_terms = '仅限验收用途的合成授权说明。'
    requirements = 'Node.js 24+ / Go 1.24+'
    support_period = '30 天'
    update_policy = 'TILL_DATE'
    update_until = $updateUntil
    status = 'DRAFT'
  } $adminHeaders
  $productId = $product.id

  $draftDetailCode = ErrorCode { Api GET "/api/v1/products/$productId" }

  $release100 = Api POST "/api/v1/$operatorPath/products/$productId/releases" @{ version = '1.0.0'; changelog = '基础版本' } $adminHeaders
  Set-Content -LiteralPath $file100 -Value 'NOVER v0.3 base release' -Encoding utf8
  $asset100 = Invoke-CurlFileUpload -Uri "$base/api/v1/$operatorPath/products/$productId/assets" -FilePath $file100 -Headers $adminHeaders -Fields @{ release_id = $release100.id }
  Api POST "/api/v1/$operatorPath/products/$productId/status" @{ status = 'ACTIVE' } $adminHeaders | Out-Null
  Api POST "/api/v1/$operatorPath/products/$productId/releases/$($release100.id)/status" @{ status = 'PUBLISHED' } $adminHeaders | Out-Null

  $detail = Api GET "/api/v1/products/$productId"
  $spaceDetail = Api GET "/api/v1/spaces/default/products/$productId"

  $issued = Api POST "/api/v1/$operatorPath/products/$productId/codes" @{ count = 1; code_ttl_days = 7; access_days = 30; max_downloads = 5; external_order_ref = "v3-$stamp"; release_id = $release100.id } $adminHeaders
  $code = $issued.codes[0]
  $redeem = Api POST '/api/v1/redeem' @{ code = $code }
  $buyerHeaders = @{ Authorization = "Bearer $($redeem.access_token)" }
  $deliveryBase = Api GET '/api/v1/delivery' $null $buyerHeaders

  $release110 = Api POST "/api/v1/$operatorPath/products/$productId/releases" @{ version = '1.1.0'; changelog = '超出更新截止的版本' } $adminHeaders
  Set-Content -LiteralPath $file110 -Value 'NOVER v0.3 newer release' -Encoding utf8
  $asset110 = Invoke-CurlFileUpload -Uri "$base/api/v1/$operatorPath/products/$productId/assets" -FilePath $file110 -Headers $adminHeaders -Fields @{ release_id = $release110.id }
  Api POST "/api/v1/$operatorPath/products/$productId/releases/$($release110.id)/status" @{ status = 'PUBLISHED' } $adminHeaders | Out-Null
  $deliveryAfterUpdate = Api GET '/api/v1/delivery' $null $buyerHeaders
  $newAssetTicketCode = ErrorCode { Api POST "/api/v1/assets/$($asset110.id)/ticket" $null $buyerHeaders }
  $baseTicket = Api POST "/api/v1/assets/$($asset100.id)/ticket" $null $buyerHeaders

  # --- 退款 ---------------------------------------------------------------
  $activity = Api GET "/api/v1/$operatorPath/activity?limit=50" $null $adminHeaders
  $codeRow = @($activity.items | Where-Object { $_.product_id -eq $productId -and $_.code_status -eq 'REDEEMED' } | Select-Object -First 1)
  $codeId = if ($codeRow.Count -gt 0) { $codeRow[0].code_id } else { $null }
  if (-not $codeId) { throw '无法从活动记录定位刚兑换的兑换码。' }
  $refundReason = "v0.3 冒烟退款 $stamp"
  $refund = Api POST "/api/v1/$operatorPath/codes/$codeId/action" @{ action = 'REFUND'; reason = $refundReason } $adminHeaders
  $refundedDeliveryCode = ErrorCode { Api GET '/api/v1/delivery' $null $buyerHeaders }
  $refundAgainCode = ErrorCode { Api POST "/api/v1/$operatorPath/codes/$codeId/action" @{ action = 'REFUND' } $adminHeaders }

  # --- 成员管理 -----------------------------------------------------------
  $member = Api POST "/api/v1/$operatorPath/members" @{ username = $memberUsername; display_name = 'QA 编辑'; password = $memberPassword; role = 'EDITOR' } $adminHeaders
  $membershipId = $member.membership_id
  $memberUserId = $member.user_id

  $memberLogin = Api POST "/api/v1/$operatorPath/login" @{ username = $memberUsername; password = $memberPassword }
  $memberToken = $memberLogin.access_token
  $memberHeaders = @{ Authorization = "Bearer $memberToken" }
  $memberPermissions = @($memberLogin.session.permissions)
  $memberCreateBlocked = ErrorCode { Api POST "/api/v1/$operatorPath/members" @{ username = "blocked$stamp"; display_name = 'Blocked'; password = 'blocked-password'; role = 'EDITOR' } $memberHeaders }
  $memberSessions = Api GET "/api/v1/$operatorPath/sessions" $null $memberHeaders
  $revokedOthers = Api POST "/api/v1/$operatorPath/sessions/revoke-others" $null $memberHeaders
  Api POST "/api/v1/$operatorPath/password" @{ current_password = $memberPassword; new_password = $memberNewPassword } $memberHeaders | Out-Null
  $oldPasswordLogin = ErrorCode { Api POST "/api/v1/$operatorPath/login" @{ username = $memberUsername; password = $memberPassword } }
  $newPasswordLogin = Api POST "/api/v1/$operatorPath/login" @{ username = $memberUsername; password = $memberNewPassword }
  $memberTokenAfterChange = $newPasswordLogin.access_token

  $updatedMember = Api PATCH "/api/v1/$operatorPath/members/$membershipId" @{ role = 'ANALYST'; status = 'SUSPENDED' } $adminHeaders
  $suspendedLogin = ErrorCode { Api POST "/api/v1/$operatorPath/login" @{ username = $memberUsername; password = $memberNewPassword } }
  $suspendedSession = ErrorCode { Api GET "/api/v1/$operatorPath/session" $null @{ Authorization = "Bearer $memberTokenAfterChange" } }

  # --- 审计日志 -----------------------------------------------------------
  $audit = Api GET "/api/v1/$operatorPath/audit-events?action=REFUND&limit=5" $null $adminHeaders
  $auditReason = ''
  if (@($audit.items).Count -gt 0) { $auditReason = [string]$audit.items[0].metadata.reason }
  $memberAudit = Api GET "/api/v1/$operatorPath/audit-events?action=MEMBER_UPDATED&limit=5" $null $adminHeaders

  $summary.draft_detail = $draftDetailCode
  $summary.profile_highlights = @($detail.highlights).Count
  $summary.profile_policy = $detail.update_policy
  $summary.profile_update_until = $detail.update_until
  $summary.space_detail_status = $spaceDetail.id -eq $productId
  $summary.base_assets = @($deliveryBase.assets).Count
  $summary.base_policy = $deliveryBase.update_policy
  $summary.updated_assets = @($deliveryAfterUpdate.assets).Count
  $summary.newer_asset_ticket = $newAssetTicketCode
  $summary.base_ticket = [bool]$baseTicket.url
  $summary.refund_status = $refund.status
  $summary.refunded_delivery = $refundedDeliveryCode
  $summary.refund_again = $refundAgainCode
  $summary.member_role = $member.role
  $summary.member_create_blocked = $memberCreateBlocked
  $summary.member_session_current = [bool](@($memberSessions.items | Where-Object { $_.current }).Count -gt 0)
  $summary.revoked_others = $revokedOthers.revoked
  $summary.old_password_login = $oldPasswordLogin
  $summary.new_password_login = [bool]$newPasswordLogin.access_token
  $summary.member_updated = "$($updatedMember.role)/$($updatedMember.status)"
  $summary.suspended_login = $suspendedLogin
  $summary.suspended_session = $suspendedSession
  $summary.audit_refund_total = $audit.total
  $summary.audit_refund_reason = $auditReason
  $summary.audit_member_updates = $memberAudit.total
  $summary.admin_has_member_manage = $adminPermissions -contains 'member.manage'
  $summary.admin_has_audit_read = $adminPermissions -contains 'audit.read'
  $summary.member_has_member_manage = $memberPermissions -contains 'member.manage'

  $summary.pass = (
    $summary.health -eq 'ready' -and
    $draftDetailCode -eq 'product_not_found' -and
    $summary.profile_highlights -eq 2 -and
    $summary.profile_policy -eq 'TILL_DATE' -and
    -not [string]::IsNullOrEmpty($summary.profile_update_until) -and
    $summary.space_detail_status -and
    $summary.base_assets -eq 1 -and
    $summary.base_policy -eq 'TILL_DATE' -and
    $summary.updated_assets -eq 1 -and
    $newAssetTicketCode -eq 'asset_not_found' -and
    $summary.base_ticket -and
    $summary.refund_status -eq 'REFUNDED' -and
    $refundedDeliveryCode -eq 'delivery_auth_invalid' -and
    $refundAgainCode -eq 'code_not_refundable' -and
    $member.role -eq 'EDITOR' -and
    $memberCreateBlocked -eq 'operator_permission_required' -and
    $summary.member_session_current -and
    $oldPasswordLogin -eq 'operator_login_failed' -and
    $summary.new_password_login -and
    $summary.member_updated -eq 'ANALYST/SUSPENDED' -and
    $suspendedLogin -eq 'operator_login_failed' -and
    $suspendedSession -eq 'operator_auth_required' -and
    $summary.audit_refund_total -ge 1 -and
    $auditReason -eq $refundReason -and
    $summary.audit_member_updates -ge 1 -and
    $summary.admin_has_member_manage -and
    $summary.admin_has_audit_read -and
    (-not $summary.member_has_member_manage)
  )

  if ($KeepFixture) {
    $summary.fixture = [ordered]@{
      product_id = $productId
      membership_id = $membershipId
      user_id = $memberUserId
      refunded_code_id = $codeId
    }
  }
  $summary | ConvertTo-Json -Depth 8
} finally {
  if ($productId -and -not $KeepFixture) {
    $keys = docker compose exec -T db psql -U nover -d nover -Atc "SELECT object_key FROM assets WHERE product_id='$productId';"
    foreach ($key in @($keys)) {
      if ($key) { docker compose exec -T api rm -f "/var/lib/nover/files/$key" | Out-Null }
    }
    $memberSql = ''
    if ($membershipId -and $memberUserId) {
      $memberSql = "DELETE FROM admin_events WHERE resource_id='$membershipId' OR resource_id='$memberUserId';`nDELETE FROM operator_sessions WHERE user_id='$memberUserId';`nDELETE FROM tenant_memberships WHERE id='$membershipId';`nDELETE FROM operator_users WHERE id='$memberUserId';"
    }
    $sql = @"
BEGIN;
DELETE FROM download_events WHERE entitlement_id IN (SELECT id FROM entitlements WHERE product_id='$productId');
DELETE FROM download_tickets WHERE entitlement_id IN (SELECT id FROM entitlements WHERE product_id='$productId');
DELETE FROM admin_events WHERE resource_type='redeem_code' AND resource_id IN (SELECT id FROM redeem_codes WHERE product_id='$productId');
UPDATE redeem_codes SET entitlement_id=NULL WHERE product_id='$productId';
DELETE FROM entitlements WHERE product_id='$productId';
DELETE FROM redeem_codes WHERE product_id='$productId';
DELETE FROM assets WHERE product_id='$productId';
DELETE FROM product_releases WHERE product_id='$productId';
DELETE FROM product_images WHERE product_id='$productId';
DELETE FROM admin_events WHERE resource_id='$productId' OR metadata_json->>'product_id'='$productId';
DELETE FROM products WHERE id='$productId';
$memberSql
COMMIT;
"@
    $sql | docker compose exec -T db psql -v ON_ERROR_STOP=1 -U nover -d nover | Out-Null
  }
  Remove-Item -LiteralPath $file100, $file110 -Force -ErrorAction SilentlyContinue
  Pop-Location
}
