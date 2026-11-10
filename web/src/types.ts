export type Product = {
  id: string
  name: string
  description: string
  detail: string
  status: 'DRAFT' | 'ACTIVE' | 'ARCHIVED'
  asset_count?: number
  images?: ProductImage[]
}

export type ProductDraftInput = Pick<Product, 'name' | 'description' | 'detail'>

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
export type OperatorLoginResponse = { access_token: string; expires_at: string }
export type AdminActivity = {
  code_id: string
  product_name: string
  code_suffix: string
  code_status: string
  external_order_ref: string
  code_expires_at: string
  redeemed_at: string | null
  entitlement_until: string | null
  downloads_issued: number
  download_events: number
}
export type RedeemResponse = { access_token: string; product_id: string }
export type DownloadTicket = { url: string; expires_at: string }

export type JoinApplication = {
  id: string
  organization_name: string
  contact_name: string
  email: string
  desired_username: string
  use_case: string
  status: 'PENDING' | 'REVIEWING' | 'APPROVED' | 'REJECTED'
  reviewer: string
  reviewed_at: string | null
  created_at: string
  updated_at: string
}

export type PlatformSettings = {
  deployment_mode: 'COLOCATED' | 'SPLIT_ORIGIN' | 'SPLIT_APPS'
  storefront_url: string
  console_url: string
  api_base_url: string
  operator_web_path: string
  operator_api_path: string
  updated_at: string
}
