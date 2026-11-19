param([switch]$KeepFixture)

# v0.4.0 身份冒烟：一次性邀请、设置密码、TOTP MFA 启用/登录/恢复码、管理员重置密码。
# 兼容 Windows PowerShell 5.1 / PowerShell 7 / Linux；文件保持 UTF-8 BOM + LF。
# 需要 API 已在 127.0.0.1:48080 运行，且 .env 中有管理员凭证。

$ErrorActionPreference = 'Stop'
$base = 'http://127.0.0.1:48080'
$operatorPath = 'cp-7f3c'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path

$envMap = @{}
Get-Content (Join-Path $repoRoot '.env') | ForEach-Object {
  if ($_ -match '^([^#=]+)=(.*)$') { $envMap[$matches[1]] = $matches[2] }
}
$adminUser = if ($envMap['ADMIN_USERNAME']) { $envMap['ADMIN_USERNAME'] } else { 'admin' }
$adminPass = if ($envMap['ADMIN_PASSWORD']) { $envMap['ADMIN_PASSWORD'] } else { 'admin' }

$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$username = "mfa$stamp"
$passwordOne = "QaIdentity-$stamp!"
$passwordTwo = "QaIdentityNew-$stamp!"
$adminHeaders = $null
$userToken = $null
$userHeaders = $null
$invitationId = $null
$resetInvitationId = $null
$memberUserId = $null

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

# --- TOTP helpers (RFC 6238, SHA1/6 digits/30s) ---------------------------
function Decode-Base32([string]$value) {
  $alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  $normalized = $value.ToUpper().Replace('-', '').Replace(' ', '').TrimEnd('=')
  $bits = New-Object System.Collections.Generic.List[int]
  foreach ($character in $normalized.ToCharArray()) {
    $index = $alphabet.IndexOf([string]$character)
    if ($index -lt 0) { throw "非法 base32 字符：$character" }
    for ($shift = 4; $shift -ge 0; $shift--) {
      $bits.Add(($index -shr $shift) -band 1)
    }
  }
  $bytes = New-Object System.Collections.Generic.List[byte]
  for ($offset = 0; $offset + 7 -lt $bits.Count; $offset += 8) {
    $byte = 0
    for ($bit = 0; $bit -lt 8; $bit++) {
      $byte = ($byte -shl 1) -bor $bits[$offset + $bit]
    }
    $bytes.Add([byte]$byte)
  }
  return ,$bytes.ToArray()
}

function Get-Totp([string]$secret, [long]$unixTime) {
  $key = Decode-Base32 $secret
  $counter = [long][math]::Floor($unixTime / 30)
  $counterBytes = [BitConverter]::GetBytes($counter)
  if ([BitConverter]::IsLittleEndian) { [Array]::Reverse($counterBytes) }
  $hmac = New-Object System.Security.Cryptography.HMACSHA1 -ArgumentList (,$key)
  try {
    $hash = $hmac.ComputeHash($counterBytes)
  } finally {
    $hmac.Dispose()
  }
  $offset = $hash[$hash.Length - 1] -band 0x0f
  $binary = ([long]([int]$hash[$offset] -band 0x7f) -shl 24) -bor
    ([long]([int]$hash[$offset + 1]) -shl 16) -bor
    ([long]([int]$hash[$offset + 2]) -shl 8) -bor
    [long]([int]$hash[$offset + 3])
  return ($binary % 1000000).ToString('000000')
}

# Self-check against RFC 6238 (T=59 -> 8-digit 94287082, 6-digit 287082).
$selfCheck = Get-Totp 'GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ' 59
if ($selfCheck -ne '287082') { throw "TOTP 自检失败：得到 $selfCheck，期望 287082" }

function New-CurrentCode([string]$secret) {
  return Get-Totp $secret ([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())
}

Push-Location $repoRoot
try {
  $summary = [ordered]@{}
  $summary.health = (Api GET '/readyz').status
  $admin = Api POST "/api/v1/$operatorPath/login" @{ username = $adminUser; password = $adminPass }
  $adminHeaders = @{ Authorization = "Bearer $($admin.access_token)" }

  # --- 邀请成员并设置密码 -------------------------------------------------
  $invitation = Api POST "/api/v1/$operatorPath/members/invitations" @{ username = $username; display_name = 'QA 身份成员'; role = 'ADMIN' } $adminHeaders
  $invitationId = $invitation.id
  $summary.invitation_kind = $invitation.kind
  $summary.invitation_has_token = [bool]$invitation.token

  $accepted = Api POST '/api/v1/invitations/accept' @{ token = $invitation.token; password = $passwordOne }
  $summary.accepted_kind = $accepted.kind
  $summary.accepted_username = $accepted.username

  $summary.duplicate_accept = ErrorCode { Api POST '/api/v1/invitations/accept' @{ token = $invitation.token; password = $passwordOne } }

  $userLogin = Api POST "/api/v1/$operatorPath/login" @{ username = $username; password = $passwordOne }
  $userToken = $userLogin.access_token
  $userHeaders = @{ Authorization = "Bearer $userToken" }
  $summary.user_role = $userLogin.session.role
  $summary.user_has_member_manage = (@($userLogin.session.permissions) -contains 'member.manage')

  # --- MFA 启用 -----------------------------------------------------------
  $setup = Api POST "/api/v1/$operatorPath/mfa/setup" $null $userHeaders
  $secret = $setup.secret
  $recoveryCodes = @($setup.recovery_codes)
  $summary.mfa_secret_length = $secret.Length
  $summary.otpauth_prefix = $setup.otpauth_url.StartsWith('otpauth://totp/')
  $summary.recovery_code_count = $recoveryCodes.Count

  $enableCode = New-CurrentCode $secret
  $enabled = Api POST "/api/v1/$operatorPath/mfa/enable" @{ code = $enableCode } $userHeaders
  $summary.mfa_enabled = $enabled.enabled
  $mfaStatus = Api GET "/api/v1/$operatorPath/mfa" $null $userHeaders
  $summary.recovery_remaining = $mfaStatus.recovery_codes_remaining

  # --- 两步登录 -----------------------------------------------------------
  $stepOne = Api POST "/api/v1/$operatorPath/login" @{ username = $username; password = $passwordOne }
  $summary.mfa_required = [bool]$stepOne.mfa_required
  $summary.step_one_has_session = [bool]$stepOne.session
  $challenge = $stepOne.challenge_token

  $referenceCode = New-CurrentCode $secret
  $invalidCode = (([int]$referenceCode + 500000) % 1000000).ToString('000000')
  $summary.invalid_mfa_code = ErrorCode { Api POST "/api/v1/$operatorPath/login/mfa" @{ challenge_token = $challenge; code = $invalidCode } }
  $loginCode = New-CurrentCode $secret
  $verified = Api POST "/api/v1/$operatorPath/login/mfa" @{ challenge_token = $challenge; code = $loginCode }
  $summary.mfa_login = [bool]$verified.access_token

  # --- 恢复码登录并消耗一个 ----------------------------------------------
  $stepOneRecovery = Api POST "/api/v1/$operatorPath/login" @{ username = $username; password = $passwordOne }
  $recoveryLogin = Api POST "/api/v1/$operatorPath/login/mfa" @{ challenge_token = $stepOneRecovery.challenge_token; code = $recoveryCodes[0] }
  $summary.recovery_login = [bool]$recoveryLogin.access_token
  $mfaStatusAfterRecovery = Api GET "/api/v1/$operatorPath/mfa" $null $userHeaders
  $summary.recovery_remaining_after_use = $mfaStatusAfterRecovery.recovery_codes_remaining

  # --- 管理员重置密码 -----------------------------------------------------
  $members = Api GET "/api/v1/$operatorPath/members" $null $adminHeaders
  $memberRow = @($members.items | Where-Object { $_.username -eq $username } | Select-Object -First 1)
  $membershipId = if ($memberRow.Count -gt 0) { $memberRow[0].membership_id } else { $null }
  if (-not $membershipId) { throw '未在成员列表中找到刚邀请的成员。' }
  $reset = Api POST "/api/v1/$operatorPath/members/$membershipId/password-reset" $null $adminHeaders
  $resetInvitationId = $reset.id
  $summary.reset_kind = $reset.kind
  $resetAccepted = Api POST '/api/v1/invitations/accept' @{ token = $reset.token; password = $passwordTwo }
  $summary.reset_accepted = $resetAccepted.kind
  $summary.old_password_login = ErrorCode { Api POST "/api/v1/$operatorPath/login" @{ username = $username; password = $passwordOne } }
  $newStepOne = Api POST "/api/v1/$operatorPath/login" @{ username = $username; password = $passwordTwo }
  $summary.new_password_requires_mfa = [bool]$newStepOne.mfa_required

  $newLoginCode = New-CurrentCode $secret
  $newSession = Api POST "/api/v1/$operatorPath/login/mfa" @{ challenge_token = $newStepOne.challenge_token; code = $newLoginCode }
  $newUserHeaders = @{ Authorization = "Bearer $($newSession.access_token)" }

  # --- 关闭 MFA 后正常登录 ------------------------------------------------
  $disableCode = New-CurrentCode $secret
  Api POST "/api/v1/$operatorPath/mfa/disable" @{ password = $passwordTwo; code = $disableCode } $newUserHeaders | Out-Null
  $mfaDisabledStatus = Api GET "/api/v1/$operatorPath/mfa" $null $newUserHeaders
  $summary.mfa_disabled = -not $mfaDisabledStatus.enabled
  $plainLogin = Api POST "/api/v1/$operatorPath/login" @{ username = $username; password = $passwordTwo }
  $summary.plain_login_after_disable = [bool]$plainLogin.access_token

  # --- 审计与邀请列表 -----------------------------------------------------
  $audit = Api GET "/api/v1/$operatorPath/audit-events?limit=100" $null $adminHeaders
  $actions = @($audit.items | ForEach-Object { $_.action })
  $summary.audit_has_mfa_enabled = $actions -contains 'MFA_ENABLED'
  $summary.audit_has_mfa_disabled = $actions -contains 'MFA_DISABLED'
  $summary.audit_has_recovery_used = $actions -contains 'MFA_RECOVERY_USED'
  $summary.audit_has_invitation = $actions -contains 'INVITATION_ACCEPTED'
  $summary.audit_has_password_reset = $actions -contains 'PASSWORD_RESET_COMPLETED'
  $invitationList = Api GET "/api/v1/$operatorPath/members/invitations" $null $adminHeaders
  $invitationStatuses = @($invitationList.items | Where-Object { $_.username -eq $username } | ForEach-Object { $_.status })
  $summary.invitation_used = $invitationStatuses -contains 'USED'

  $summary.pass = (
    $summary.health -eq 'ready' -and
    $summary.invitation_kind -eq 'INVITE' -and
    $summary.invitation_has_token -and
    $summary.accepted_kind -eq 'INVITE' -and
    $summary.accepted_username -eq $username -and
    $summary.duplicate_accept -eq 'invitation_used' -and
    $summary.user_role -eq 'ADMIN' -and
    $summary.user_has_member_manage -and
    $summary.mfa_secret_length -ge 16 -and
    $summary.otpauth_prefix -and
    $summary.recovery_code_count -eq 10 -and
    $summary.mfa_enabled -and
    $summary.recovery_remaining -eq 10 -and
    $summary.mfa_required -and
    (-not $summary.step_one_has_session) -and
    $summary.invalid_mfa_code -eq 'invalid_mfa_code' -and
    $summary.mfa_login -and
    $summary.recovery_login -and
    $summary.recovery_remaining_after_use -eq 9 -and
    $summary.reset_kind -eq 'PASSWORD_RESET' -and
    $summary.reset_accepted -eq 'PASSWORD_RESET' -and
    $summary.old_password_login -eq 'operator_login_failed' -and
    $summary.new_password_requires_mfa -and
    $summary.mfa_disabled -and
    $summary.plain_login_after_disable -and
    $summary.audit_has_mfa_enabled -and
    $summary.audit_has_mfa_disabled -and
    $summary.audit_has_recovery_used -and
    $summary.audit_has_invitation -and
    $summary.audit_has_password_reset -and
    $summary.invitation_used
  )

  $summary | ConvertTo-Json -Depth 8
} finally {
  if (-not $KeepFixture) {
    $cleanup = docker compose exec -T db psql -U nover -d nover -Atc "SELECT id FROM operator_users WHERE LOWER(username)=LOWER('$username');" 2>$null
    $userId = ($cleanup | Out-String).Trim()
    if ($userId) {
      $sql = @"
BEGIN;
DELETE FROM admin_events WHERE resource_id IN ('$invitationId', '$resetInvitationId', '$userId') OR metadata_json->>'username' = '$username';
DELETE FROM operator_sessions WHERE user_id='$userId';
DELETE FROM operator_invitations WHERE user_id='$userId';
DELETE FROM tenant_memberships WHERE user_id='$userId';
DELETE FROM operator_users WHERE id='$userId';
COMMIT;
"@
      $sql | docker compose exec -T db psql -v ON_ERROR_STOP=1 -U nover -d nover | Out-Null
    }
  }
  Pop-Location
}
