import { z } from 'zod'
import type { AdminActivity, Asset, Delivery, DownloadTicket, OperatorLoginResponse, Product, ProductList, RedeemResponse } from '../types'

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

export async function loadServiceHealth() {
  return request<{ status: 'ready' }>('/readyz')
}

export function loadPublicProducts() {
  return request<ProductList>('/api/v1/products')
}

export function operatorLogin(username: string, password: string) {
  const input = z.object({ username: z.string().trim().min(1).max(100), password: z.string().min(1).max(200) }).parse({ username, password })
  return request<OperatorLoginResponse>('/api/v1/ops/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, init)
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
  return request<ProductList>('/api/v1/ops/products', { headers: withBearer(token) })
}

export function loadActivity(token: string) {
  return request<{ items: AdminActivity[] }>('/api/v1/ops/activity', { headers: withBearer(token) })
}

export function updateCodeAction(token: string, codeId: string, action: 'SUSPEND' | 'RESUME' | 'REVOKE') {
  return request<{ status: string }>(`/api/v1/ops/codes/${codeId}/action`, {
    method: 'POST',
    headers: { ...withBearer(token), 'Content-Type': 'application/json' },
    body: JSON.stringify({ action }),
  })
}

export function createProduct(token: string, name: string, description: string) {
  const input = z.object({ name: z.string().trim().min(1).max(120), description: z.string().trim().max(2000) }).parse({ name, description })
  return request<Product>('/api/v1/ops/products', {
    method: 'POST',
    headers: { ...withBearer(token), 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export function uploadAsset(token: string, productId: string, file: File) {
  const body = new FormData()
  body.append('file', file)
  return request<Asset>(`/api/v1/ops/products/${productId}/assets`, {
    method: 'POST',
    headers: withBearer(token),
    body,
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
    `/api/v1/ops/products/${productId}/codes`,
    {
      method: 'POST',
      headers: { ...withBearer(token), 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    },
  )
}

export function createDownloadTicket(token: string, assetId: string) {
  return request<DownloadTicket>(`/api/v1/assets/${assetId}/ticket`, {
    method: 'POST',
    headers: withBearer(token),
  })
}
