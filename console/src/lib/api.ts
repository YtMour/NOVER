import { z } from 'zod'
import type { AdminActivityResponse, Asset, Delivery, DownloadTicket, JoinApplication, OperatorLoginResponse, OperatorSession, PlatformSettings, Product, ProductDraftInput, ProductImage, ProductList, RedeemResponse, TenantMember } from '../types'
import { OPERATOR_API_PATH } from './routes'

export class ApiError extends Error {
  constructor(
    message: string,
    readonly code: string,
    readonly status: number,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

function apiURL(path: string) {
  const configured = import.meta.env.VITE_API_BASE_URL?.replace(/\/$/, '')
  if (configured) return `${configured}${path}`
  if (typeof window === 'undefined') return path
  const port = import.meta.env.VITE_API_PORT || '48080'
  return `${window.location.protocol}//${window.location.hostname}:${port}${path}`
}

export async function loadServiceHealth() {
  return request<{ status: 'ready' }>('/readyz')
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
  const response = await fetch(apiURL(path), init)
  const payload = await response.json().catch(() => ({}))
  if (!response.ok) {
    throw new ApiError(
      payload.message ?? '请求暂时没有完成。',
      payload.code ?? 'request_failed',
      response.status,
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
  return request<{ tenant_id: string; tenant_name: string; items: TenantMember[] }>(operatorEndpoint('/members'), { headers: withBearer(token) })
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

export function updateCodeAction(token: string, codeId: string, action: 'SUSPEND' | 'RESUME' | 'REVOKE') {
  return request<{ status: string }>(operatorEndpoint(`/codes/${codeId}/action`), {
    method: 'POST',
    headers: { ...withBearer(token), 'Content-Type': 'application/json' },
    body: JSON.stringify({ action }),
  })
}

export function createProduct(token: string, name: string, description: string, detail: string, status: 'DRAFT' | 'ACTIVE' = 'DRAFT') {
  const input = z.object({ name: z.string().trim().min(1).max(120), description: z.string().trim().max(2000), detail: z.string().trim().max(6000), status: z.enum(['DRAFT', 'ACTIVE']) }).parse({ name, description, detail, status })
  return request<Product>(operatorEndpoint('/products'), {
    method: 'POST',
    headers: { ...withBearer(token), 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export function updateProduct(token: string, productId: string, draft: ProductDraftInput) {
  const input = z.object({ name: z.string().trim().min(1).max(120), description: z.string().trim().max(2000), detail: z.string().trim().max(6000) }).parse(draft)
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

export function uploadAsset(token: string, productId: string, file: File) {
  const body = new FormData()
  body.append('file', file)
  return request<Asset>(operatorEndpoint(`/products/${productId}/assets`), {
    method: 'POST',
    headers: withBearer(token),
    body,
  })
}

export function loadProductImages(token: string, productId: string) {
  return request<{ items: ProductImage[] }>(operatorEndpoint(`/products/${productId}/images`), { headers: withBearer(token) })
}

export async function loadProductImageBlob(token: string, productId: string, imageId: string) {
  const response = await fetch(apiURL(operatorEndpoint(`/products/${productId}/images/${imageId}`)), { headers: withBearer(token) })
  if (!response.ok) {
    const payload = await response.json().catch(() => ({}))
    throw new ApiError(payload.message ?? '图片暂时没有读取成功。', payload.code ?? 'image_load_failed', response.status)
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

export function issueCodes(
  token: string,
  productId: string,
  options: { count: number; code_ttl_days: number; access_days: number; max_downloads: number; external_order_ref: string },
) {
  const input = z.object({
    count: z.number().int().min(1).max(100),
    code_ttl_days: z.number().int().min(1).max(3650),
    access_days: z.number().int().min(1).max(3650),
    max_downloads: z.number().int().min(1).max(10_000),
    external_order_ref: z.string().max(200),
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
