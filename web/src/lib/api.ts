import { z } from 'zod'
import type { Delivery, DownloadTicket, ProductList, RedeemResponse } from '../types'

export class ApiError extends Error {
  constructor(message: string, readonly code: string, readonly status: number) {
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

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(apiURL(path), init)
  const payload = await response.json().catch(() => ({}))
  if (!response.ok) throw new ApiError(payload.message ?? '请求暂时没有完成。', payload.code ?? 'request_failed', response.status)
  return payload as T
}

function withBearer(token: string): HeadersInit {
  return { Authorization: `Bearer ${token}` }
}

export function loadServiceHealth() {
  return request<{ status: 'ready' }>('/readyz')
}

export function loadPublicProducts() {
  return request<ProductList>('/api/v1/products')
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
