export type UpdatePolicy = 'LIFETIME' | 'TILL_DATE' | 'CURRENT_ONLY'

export type Product = {
  id: string
  tenant_id?: string
  tenant_name?: string
  owner_user_id?: string
  owner_name?: string
  name: string
  description: string
  detail: string
  status: 'DRAFT' | 'ACTIVE' | 'ARCHIVED'
  current_release_version?: string
  current_release_changelog?: string
  asset_count?: number
  images?: ProductImage[]
  // 商品资料 v2：旧版 API 不返回这些字段，前端按可选处理并回退默认值。
  highlights?: string[]
  license_terms?: string
  requirements?: string
  support_period?: string
  update_policy?: UpdatePolicy
  update_until?: string | null
}

export type ProductImage = {
  id: string
  url: string
  alt_text: string
  mime_type: string
  size_bytes: number
}

export type Asset = {
  id: string
  product_id: string
  release_id?: string
  release_version?: string
  file_name: string
  mime_type: string
  size_bytes: number
  sha256: string
}

export type Delivery = {
  entitlement_id: string
  valid_until: string
  max_downloads?: number
  downloads_used?: number
  base_release_id?: string
  base_version?: string
  update_policy?: UpdatePolicy
  update_until?: string | null
  product: Product
  assets: Asset[]
}

export type ProductList = { items: Product[]; space_name?: string }
export type RedeemResponse = { access_token: string; product_id: string }
export type DownloadTicket = { url: string; expires_at: string }

// 公开构建元数据（GET /api/v1/meta）。页脚只消费 version，其余字段用于诊断。
export type Meta = {
  name: string
  version: string
  commit: string
  built_at: string
  environment: string
  schema_version?: string
}
