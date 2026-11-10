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

export type ProductRelease = {
  id: string
  tenant_id: string
  product_id: string
  version: string
  changelog: string
  status: 'DRAFT' | 'PUBLISHED' | 'ARCHIVED'
  created_by: string
  creator_name: string
  asset_count: number
  published_at: string | null
  created_at: string
  updated_at: string
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
  product: Product
  assets: Asset[]
}

export type ProductList = { items: Product[] }
export type OperatorSession = {
  user_id: string
  username: string
  display_name: string
  platform_role: '' | 'PLATFORM_OWNER' | 'PLATFORM_SUPPORT' | 'PLATFORM_AUDITOR'
  membership_id: string
  tenant_id: string
  tenant_name: string
  role: 'OWNER' | 'ADMIN' | 'EDITOR' | 'FULFILLMENT' | 'ANALYST'
  membership_status: 'PENDING' | 'ACTIVE' | 'SUSPENDED'
  application_id?: string
  application_status?: 'PENDING' | 'REVIEWING' | 'APPROVED' | 'REJECTED'
  permissions: string[]
}

export type OperatorLoginResponse = { access_token: string; expires_at: string; session: OperatorSession }

export type TenantMember = {
  user_id: string
  username: string
  display_name: string
  role: OperatorSession['role']
  platform_role: OperatorSession['platform_role']
  status: 'ACTIVE' | 'SUSPENDED'
  joined_at: string
  last_active_at: string | null
}
export type AdminActivity = {
  code_id: string
  product_id: string
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
export type AdminActivitySummary = {
  total_codes: number
  redeemed_codes: number
  download_events: number
  products: Array<{ product_id: string; codes: number; redeemed: number; download_events: number }>
}
export type AdminActivityResponse = { items: AdminActivity[]; summary: AdminActivitySummary }
export type RedeemResponse = { access_token: string; product_id: string }
export type DownloadTicket = { url: string; expires_at: string }

export type JoinApplication = {
  id: string
  tenant_id?: string
  operator_user_id?: string
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
