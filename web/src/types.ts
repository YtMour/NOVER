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
  base_release_id?: string
  base_version?: string
  product: Product
  assets: Asset[]
}

export type ProductList = { items: Product[] }
export type RedeemResponse = { access_token: string; product_id: string }
export type DownloadTicket = { url: string; expires_at: string }
