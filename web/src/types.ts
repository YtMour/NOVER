export type Product = {
  id: string
  name: string
  description: string
  status: string
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
