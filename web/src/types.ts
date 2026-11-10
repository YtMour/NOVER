export type Product = {
  id: string
  name: string
  description: string
  detail: string
  status: 'DRAFT' | 'ACTIVE' | 'ARCHIVED'
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
  file_name: string
  mime_type: string
  size_bytes: number
  sha256: string
}

export type Delivery = {
  entitlement_id: string
  valid_until: string
  product: Product
  assets: Asset[]
}

export type ProductList = { items: Product[] }
export type RedeemResponse = { access_token: string; product_id: string }
export type DownloadTicket = { url: string; expires_at: string }
