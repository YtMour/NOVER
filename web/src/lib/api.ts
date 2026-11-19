import { z } from 'zod'
import type { Delivery, DownloadTicket, Meta, Product, ProductList, RedeemResponse } from '../types'

export class ApiError extends Error {
  constructor(message: string, readonly code: string, readonly status: number, readonly retryAfter?: number) {
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
  // The Vite/Nginx proxy keeps localhost, 127.0.0.1 and custom domains on
  // the same origin; an explicit base URL remains available for split deploys.
  return path
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

export function loadServiceHealth() {
  return request<{ status: 'ready' }>('/readyz')
}

/** 公开版本元数据；页脚据此显示当前部署版本，接口失败时静默隐藏。 */
export function loadMeta() {
  return request<Meta>('/api/v1/meta')
}

export function loadPublicProducts(spaceSlug?: string) {
  return request<ProductList>(spaceSlug ? `/api/v1/spaces/${encodeURIComponent(spaceSlug)}/products` : '/api/v1/products')
}

/** 公开商品详情：不存在或已下架时后端返回 404 product_not_found。 */
export function loadProductDetail(productID: string, spaceSlug?: string) {
  const productPath = `/products/${encodeURIComponent(productID)}`
  return request<Product>(spaceSlug ? `/api/v1/spaces/${encodeURIComponent(spaceSlug)}${productPath}` : `/api/v1${productPath}`)
}

export function submitJoinApplication(input: { organization_name: string; contact_name: string; email: string; desired_username: string; expected_password: string; use_case: string }) {
  const payload = z.object({
    organization_name: z.string().trim().min(2).max(120),
    contact_name: z.string().trim().min(2).max(80),
    email: z.string().trim().email().max(254),
    desired_username: z.string().trim().toLowerCase().regex(/^[a-z][a-z0-9_-]{3,31}$/),
    expected_password: z.string().min(8).max(200),
    use_case: z.string().trim().min(10).max(1200),
  }).parse(input)
  return request<{ id: string; status: 'PENDING'; created_at: string }>('/api/v1/join-applications', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload),
  })
}

export async function redeemCode(code: string) {
  const normalized = code.trim().toUpperCase().replaceAll('-', '').replace(/^NVR/, '')
  if (!/^[A-Z2-7]{26}$/.test(normalized)) throw new ApiError('兑换码格式不正确，请核对字符后重试。', 'invalid_code_format', 400)
  const groups = normalized.match(/.{1,4}/g) ?? []
  const input = z.object({ code: z.string().trim().min(1).max(40) }).parse({ code: `NVR-${groups.join('-')}` })
  return request<RedeemResponse>('/api/v1/redeem', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input),
  })
}

export function loadDelivery(token: string) {
  return request<Delivery>('/api/v1/delivery', { headers: withBearer(token) })
}

export function createDownloadTicket(token: string, assetId: string) {
  return request<DownloadTicket>(`/api/v1/assets/${assetId}/ticket`, { method: 'POST', headers: withBearer(token) })
}
