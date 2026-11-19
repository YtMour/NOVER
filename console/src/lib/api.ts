import { z } from 'zod'
import type {
  AdminActivityResponse,
  Asset,
  AuditEventList,
  AuditEventQuery,
  CodeAction,
  Delivery,
  DownloadTicket,
  InvitationAcceptResponse,
  JoinApplication,
  MemberCreateInput,
  MemberInvitation,
  MemberInvitationCreated,
  MemberInvitationInput,
  MemberPasswordResetCreated,
  MemberPatchInput,
  Meta,
  MfaSetupResponse,
  MfaStatus,
  OperatorLoginResponse,
  OperatorLoginSuccess,
  OperatorSession,
  OperatorSessionsResponse,
  PlatformSettings,
  Product,
  ProductDraftInput,
  ProductImage,
  ProductList,
  ProductRelease,
  RedeemResponse,
  SystemStatus,
  TenantMember,
} from '../types'
import { OPERATOR_API_PATH } from './routes'

export class ApiError extends Error {
  constructor(
    message: string,
    readonly code: string,
    readonly status: number,
    readonly retryAfter?: number,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

const NETWORK_ERROR_MESSAGE = '网络连接异常，请检查网络后重试。'

function parseRetryAfter(response: Response): number | undefined {
  if (response.status !== 429) return undefined
  const header = response.headers.get('Retry-After')
  if (!header) return undefined
  const seconds = Number(header.trim())
  if (!Number.isFinite(seconds) || seconds < 0) return undefined
  return Math.ceil(seconds)
}

function rateLimitMessage(message: string, retryAfter?: number) {
  return retryAfter === undefined ? `${message} 请稍后重试。` : `${message} 请 ${retryAfter} 秒后重试。`
}

function apiURL(path: string) {
  const configured = import.meta.env.VITE_API_BASE_URL?.replace(/\/$/, '')
  if (configured) return `${configured}${path}`
  return path
}

export async function loadServiceHealth() {
  return request<{ status: 'ready' }>('/readyz')
}

/** 公开构建元数据；顶栏据此显示 API 版本，失败时静默隐藏。 */
export function loadMeta() {
  return request<Meta>('/api/v1/meta')
}

export function loadPublicProducts() {
  return request<ProductList>('/api/v1/products')
}

export function operatorLogin(username: string, password: string) {
  const input = z.object({ username: z.string().trim().min(1).max(100), password: z.string().min(1).max(200) }).parse({ username, password })
  return request<OperatorLoginResponse>(`/api/v1/${OPERATOR_API_PATH}/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

// Recovery codes are formatted XXXX-XXXX-XXXX; keep hyphens and allow either a
// 6-digit TOTP code or a recovery code in the same input.
const mfaCodeSchema = z.string().trim().min(6).max(64)

export function operatorLoginMfa(challengeToken: string, code: string) {
  const input = z.object({ challenge_token: z.string().trim().min(1).max(4096), code: mfaCodeSchema }).parse({ challenge_token: challengeToken, code })
  return request<OperatorLoginSuccess>(`/api/v1/${OPERATOR_API_PATH}/login/mfa`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export function submitJoinApplication(input: { organization_name: string; contact_name: string; email: string; desired_username: string; use_case: string }) {
  const payload = z.object({
    organization_name: z.string().trim().min(2).max(120),
    contact_name: z.string().trim().min(2).max(80),
    email: z.string().trim().email().max(254),
    desired_username: z.string().trim().toLowerCase().regex(/^[a-z][a-z0-9_-]{3,31}$/),
    use_case: z.string().trim().min(10).max(1200),
  }).parse(input)
  return request<{ id: string; status: JoinApplication['status']; created_at: string }>('/api/v1/join-applications', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

function operatorEndpoint(path: string) {
  return `/api/v1/${OPERATOR_API_PATH}${path}`
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let response: Response
  try {
    response = await fetch(apiURL(path), init)
  } catch {
    throw new ApiError(NETWORK_ERROR_MESSAGE, 'network_error', 0)
  }
  const payload = await response.json().catch(() => ({}))
  if (!response.ok) {
    const retryAfter = parseRetryAfter(response)
    const message = payload.message ?? '请求暂时没有完成。'
    throw new ApiError(
      response.status === 429 ? rateLimitMessage(message, retryAfter) : message,
      payload.code ?? 'request_failed',
      response.status,
      retryAfter,
    )
  }
  return payload as T
}

function withBearer(token: string): HeadersInit {
  return { Authorization: `Bearer ${token}` }
}

export function loadOperatorSession(token: string) {
  return request<OperatorSession>(operatorEndpoint('/session'), { headers: withBearer(token) })
}

export function revokeOperatorSession(token: string) {
  return request<void>(operatorEndpoint('/session/revoke'), { method: 'POST', headers: withBearer(token) })
}

export function loadTenantMembers(token: string) {
  return request<{ tenant_id?: string; tenant_name?: string; items: TenantMember[] }>(operatorEndpoint('/members'), { headers: withBearer(token) })
}

const memberRoleSchema = z.enum(['ADMIN', 'EDITOR', 'FULFILLMENT', 'ANALYST'])
const memberStatusSchema = z.enum(['ACTIVE', 'SUSPENDED'])

export function createTenantMember(token: string, input: MemberCreateInput) {
  const payload = z.object({
    username: z.string().trim().toLowerCase().regex(/^[a-z][a-z0-9_-]{3,31}$/),
    display_name: z.string().trim().min(2).max(80),
    password: z.string().min(8).max(200),
    role: memberRoleSchema,
  }).parse(input)
  return request<TenantMember>(operatorEndpoint('/members'), {
    method: 'POST',
    headers: { ...withBearer(token), 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export function updateTenantMember(token: string, membershipId: string, patch: MemberPatchInput) {
  const payload = z.object({ role: memberRoleSchema.optional(), status: memberStatusSchema.optional() }).parse(patch)
  return request<TenantMember>(operatorEndpoint(`/members/${encodeURIComponent(membershipId)}`), {
    method: 'PATCH',
    headers: { ...withBearer(token), 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

const invitationRoleSchema = z.enum(['ADMIN', 'EDITOR', 'FULFILLMENT', 'ANALYST'])

export function loadMemberInvitations(token: string) {
  return request<{ items: MemberInvitation[] }>(operatorEndpoint('/members/invitations'), { headers: withBearer(token) })
}

export function createMemberInvitation(token: string, input: MemberInvitationInput) {
  const payload = z.object({
    username: z.string().trim().toLowerCase().regex(/^[a-z][a-z0-9_-]{3,31}$/),
    display_name: z.string().trim().min(2).max(80),
    role: invitationRoleSchema,
  }).parse(input)
  return request<MemberInvitationCreated>(operatorEndpoint('/members/invitations'), {
    method: 'POST',
    headers: { ...withBearer(token), 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export function revokeMemberInvitation(token: string, invitationId: string) {
  return request<void>(operatorEndpoint(`/members/invitations/${encodeURIComponent(invitationId)}/revoke`), {
    method: 'POST',
    headers: withBearer(token),
  })
}

export function createMemberPasswordReset(token: string, membershipId: string) {
  return request<MemberPasswordResetCreated>(operatorEndpoint(`/members/${encodeURIComponent(membershipId)}/password-reset`), {
    method: 'POST',
    headers: withBearer(token),
  })
}

// Public endpoint: no operator session is required because the invitation token
// itself proves the right to set the password.
export function acceptInvitation(invitationToken: string, password: string) {
  const payload = z.object({
    token: z.string().trim().min(1).max(1024),
    password: z.string().trim().min(8).max(200),
  }).parse({ token: invitationToken, password })
  return request<InvitationAcceptResponse>('/api/v1/invitations/accept', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export function loadOperatorSessions(token: string) {
  return request<OperatorSessionsResponse>(operatorEndpoint('/sessions'), { headers: withBearer(token) })
}

export function revokeOperatorSessionById(token: string, sessionId: string) {
  return request<void>(operatorEndpoint(`/sessions/${encodeURIComponent(sessionId)}/revoke`), { method: 'POST', headers: withBearer(token) })
}

export function revokeOtherOperatorSessions(token: string) {
  return request<{ revoked: number }>(operatorEndpoint('/sessions/revoke-others'), { method: 'POST', headers: withBearer(token) })
}

export function changeOperatorPassword(token: string, currentPassword: string, newPassword: string) {
  const payload = z.object({
    current_password: z.string().min(1).max(200),
    new_password: z.string().min(8).max(200),
  }).parse({ current_password: currentPassword, new_password: newPassword })
  return request<void>(operatorEndpoint('/password'), {
    method: 'POST',
    headers: { ...withBearer(token), 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export function loadMfaStatus(token: string) {
  return request<MfaStatus>(operatorEndpoint('/mfa'), { headers: withBearer(token) })
}

export function setupMfa(token: string) {
  return request<MfaSetupResponse>(operatorEndpoint('/mfa/setup'), { method: 'POST', headers: withBearer(token) })
}

export function enableMfa(token: string, code: string) {
  const payload = z.object({ code: mfaCodeSchema }).parse({ code })
  return request<{ enabled: true }>(operatorEndpoint('/mfa/enable'), {
    method: 'POST',
    headers: { ...withBearer(token), 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export function disableMfa(token: string, password: string, code: string) {
  const payload = z.object({
    password: z.string().min(1).max(200),
    code: mfaCodeSchema,
  }).parse({ password, code })
  return request<void>(operatorEndpoint('/mfa/disable'), {
    method: 'POST',
    headers: { ...withBearer(token), 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

const auditQuerySchema = z.object({
  limit: z.number().int().min(1).max(200),
  offset: z.number().int().min(0),
  action: z.string().trim().max(64),
  resource_type: z.string().trim().max(64),
})

export function loadAuditEvents(token: string, query: AuditEventQuery = {}) {
  const input = auditQuerySchema.parse({
    limit: query.limit ?? 20,
    offset: query.offset ?? 0,
    action: query.action ?? '',
    resource_type: query.resource_type ?? '',
  })
  const params = new URLSearchParams({ limit: String(input.limit), offset: String(input.offset) })
  if (input.action) params.set('action', input.action)
  if (input.resource_type) params.set('resource_type', input.resource_type)
  return request<AuditEventList>(operatorEndpoint(`/audit-events?${params.toString()}`), { headers: withBearer(token) })
}

export async function redeemCode(code: string) {
  const normalized = code.trim().toUpperCase().replaceAll('-', '').replace(/^NVR/, '')
  if (!/^[A-Z2-7]{26}$/.test(normalized)) {
    throw new ApiError('兑换码格式不正确，请核对字符后重试。', 'invalid_code_format', 400)
  }
  const groups = normalized.match(/.{1,4}/g) ?? []
  const input = z.object({ code: z.string().trim().min(1).max(40) }).parse({ code: `NVR-${groups.join('-')}` })
  return request<RedeemResponse>('/api/v1/redeem', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export function loadDelivery(token: string) {
  return request<Delivery>('/api/v1/delivery', { headers: withBearer(token) })
}

export function loadProducts(token: string) {
  return request<ProductList>(operatorEndpoint('/products'), { headers: withBearer(token) })
}

export function loadActivity(token: string) {
  return request<AdminActivityResponse>(operatorEndpoint('/activity'), { headers: withBearer(token) })
}

const codeActionSchema = z.enum(['SUSPEND', 'RESUME', 'REVOKE', 'REFUND'])

export function updateCodeAction(token: string, codeId: string, action: CodeAction, reason?: string) {
  const trimmedReason = reason ? Array.from(reason.trim()).slice(0, 300).join('') : ''
  const payload = z.object({ action: codeActionSchema, reason: z.string().refine((value) => Array.from(value).length <= 300, '退款原因不能超过 300 字').optional() }).parse({
    action,
    ...(trimmedReason ? { reason: trimmedReason } : {}),
  })
  return request<{ status: string }>(operatorEndpoint(`/codes/${codeId}/action`), {
    method: 'POST',
    headers: { ...withBearer(token), 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

const updatePolicySchema = z.enum(['LIFETIME', 'TILL_DATE', 'CURRENT_ONLY'])
const rfc3339Timestamp = z.string().trim().min(1).max(64).refine((value) => !Number.isNaN(Date.parse(value)), '时间必须是 RFC3339 格式')

const productDraftSchema = z.object({
  name: z.string().trim().min(1).max(120),
  description: z.string().trim().max(2000),
  detail: z.string().trim().max(6000),
  highlights: z.array(z.string().trim().min(1).max(120)).max(8),
  license_terms: z.string().trim().max(2000),
  requirements: z.string().trim().max(2000),
  support_period: z.string().trim().max(120),
  update_policy: updatePolicySchema,
  update_until: rfc3339Timestamp.nullable(),
}).superRefine((value, ctx) => {
  if (value.update_policy === 'TILL_DATE' && !value.update_until) {
    ctx.addIssue({ code: 'custom', path: ['update_until'], message: '更新策略为 TILL_DATE 时必须填写 update_until' })
  }
  if (value.update_policy !== 'TILL_DATE' && value.update_until) {
    ctx.addIssue({ code: 'custom', path: ['update_until'], message: '仅 TILL_DATE 策略可以填写 update_until' })
  }
})

export function createProduct(token: string, draft: ProductDraftInput, status: 'DRAFT' | 'ACTIVE' = 'DRAFT') {
  const input = productDraftSchema.parse(draft)
  return request<Product>(operatorEndpoint('/products'), {
    method: 'POST',
    headers: { ...withBearer(token), 'Content-Type': 'application/json' },
    body: JSON.stringify({ ...input, status }),
  })
}

export function updateProduct(token: string, productId: string, draft: ProductDraftInput) {
  const input = productDraftSchema.parse(draft)
  return request<Product>(operatorEndpoint(`/products/${productId}`), {
    method: 'PUT',
    headers: { ...withBearer(token), 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export function updateProductStatus(token: string, productId: string, status: Product['status']) {
  return request<Product>(operatorEndpoint(`/products/${productId}/status`), {
    method: 'POST',
    headers: { ...withBearer(token), 'Content-Type': 'application/json' },
    body: JSON.stringify({ status }),
  })
}

export function uploadAsset(token: string, productId: string, releaseId: string, file: File) {
  const body = new FormData()
  body.append('file', file)
  body.append('release_id', releaseId)
  return request<Asset>(operatorEndpoint(`/products/${productId}/assets`), {
    method: 'POST',
    headers: withBearer(token),
    body,
  })
}

export function loadProductAssets(token: string, productId: string) {
  return request<{ items: Asset[] }>(operatorEndpoint(`/products/${productId}/assets`), { headers: withBearer(token) })
}

export function deleteDraftAsset(token: string, productId: string, assetId: string) {
  return request<void>(operatorEndpoint(`/products/${productId}/assets/${assetId}`), { method: 'DELETE', headers: withBearer(token) })
}

export function loadProductImages(token: string, productId: string) {
  return request<{ items: ProductImage[] }>(operatorEndpoint(`/products/${productId}/images`), { headers: withBearer(token) })
}

export async function loadProductImageBlob(token: string, productId: string, imageId: string) {
  let response: Response
  try {
    response = await fetch(apiURL(operatorEndpoint(`/products/${productId}/images/${imageId}`)), { headers: withBearer(token) })
  } catch {
    throw new ApiError(NETWORK_ERROR_MESSAGE, 'network_error', 0)
  }
  if (!response.ok) {
    const payload = await response.json().catch(() => ({}))
    const retryAfter = parseRetryAfter(response)
    const message = payload.message ?? '图片暂时没有读取成功。'
    throw new ApiError(
      response.status === 429 ? rateLimitMessage(message, retryAfter) : message,
      payload.code ?? 'image_load_failed',
      response.status,
      retryAfter,
    )
  }
  return response.blob()
}

export function uploadProductImage(token: string, productId: string, file: File, altText: string) {
  const body = new FormData()
  body.append('file', file)
  body.append('alt_text', altText)
  return request<ProductImage>(operatorEndpoint(`/products/${productId}/images`), {
    method: 'POST',
    headers: withBearer(token),
    body,
  })
}

export function deleteProductImage(token: string, productId: string, imageId: string) {
  return request<void>(operatorEndpoint(`/products/${productId}/images/${imageId}`), {
    method: 'DELETE',
    headers: withBearer(token),
  })
}

export function loadProductReleases(token: string, productId: string) {
  return request<{ product_id: string; items: ProductRelease[] }>(operatorEndpoint(`/products/${productId}/releases`), { headers: withBearer(token) })
}

export function createProductRelease(token: string, productId: string, version: string, changelog: string) {
  const input = z.object({ version: z.string().trim().regex(/^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$/).max(64), changelog: z.string().trim().max(6000) }).parse({ version, changelog })
  return request<ProductRelease>(operatorEndpoint(`/products/${productId}/releases`), {
    method: 'POST',
    headers: { ...withBearer(token), 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export function updateProductReleaseStatus(token: string, productId: string, releaseId: string, status: ProductRelease['status']) {
  return request<ProductRelease>(operatorEndpoint(`/products/${productId}/releases/${releaseId}/status`), {
    method: 'POST',
    headers: { ...withBearer(token), 'Content-Type': 'application/json' },
    body: JSON.stringify({ status }),
  })
}

export function issueCodes(
  token: string,
  productId: string,
  options: { count: number; code_ttl_days: number; access_days: number; max_downloads: number; external_order_ref: string; release_id: string },
) {
  const input = z.object({
    count: z.number().int().min(1).max(100),
    code_ttl_days: z.number().int().min(1).max(3650),
    access_days: z.number().int().min(1).max(3650),
    max_downloads: z.number().int().min(1).max(10_000),
    external_order_ref: z.string().max(200),
    release_id: z.string().min(1),
  }).parse(options)
  return request<{ codes: string[]; code_ttl_until: string; access_days: number; max_downloads: number }>(
    operatorEndpoint(`/products/${productId}/codes`),
    {
      method: 'POST',
      headers: { ...withBearer(token), 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    },
  )
}

export function loadJoinApplications(token: string) {
  return request<{ items: JoinApplication[] }>(operatorEndpoint('/applications'), { headers: withBearer(token) })
}

export function reviewJoinApplication(token: string, applicationId: string, action: 'REVIEW' | 'APPROVE' | 'REJECT') {
  return request<JoinApplication>(operatorEndpoint(`/applications/${applicationId}/review`), {
    method: 'POST',
    headers: { ...withBearer(token), 'Content-Type': 'application/json' },
    body: JSON.stringify({ action }),
  })
}

export function loadPlatformSettings(token: string) {
  return request<PlatformSettings>(operatorEndpoint('/settings'), { headers: withBearer(token) })
}

/** 运行中部署的版本、数据库、迁移与存储状态；需要 settings.read 权限。 */
export function loadSystemStatus(token: string) {
  return request<SystemStatus>(operatorEndpoint('/system/status'), { headers: withBearer(token) })
}

export function savePlatformSettings(token: string, settings: Omit<PlatformSettings, 'updated_at'>) {
  return request<PlatformSettings>(operatorEndpoint('/settings'), {
    method: 'PUT',
    headers: { ...withBearer(token), 'Content-Type': 'application/json' },
    body: JSON.stringify(settings),
  })
}

export function createDownloadTicket(token: string, assetId: string) {
  return request<DownloadTicket>(`/api/v1/assets/${assetId}/ticket`, {
    method: 'POST',
    headers: withBearer(token),
  })
}
