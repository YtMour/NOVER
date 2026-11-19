import { ArrowLeft } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate, useParams } from 'react-router-dom'
import { StatusMessage } from '../../components/StatusMessage'
import { ApiError, loadProductDetail } from '../../lib/api'
import { ProductDetailView } from './ProductDetailView'

function isProductNotFound(error: unknown) {
  return error instanceof ApiError && (error.code === 'product_not_found' || error.status === 404)
}

export function ProductDetailPage() {
  const { productID = '', spaceSlug } = useParams()
  const navigate = useNavigate()
  const detail = useQuery({
    queryKey: ['product-detail', spaceSlug ?? null, productID],
    queryFn: () => loadProductDetail(productID, spaceSlug),
    enabled: productID.length > 0,
    // 404 是确定性的“已下架/不存在”，无需重试；其他错误保留一次重试。
    retry: (failureCount, error) => !isProductNotFound(error) && failureCount < 2,
  })

  const catalogTo = spaceSlug ? `/spaces/${spaceSlug}` : '/'

  if (detail.isLoading) return <div className="loading-line">正在读取商品信息…</div>
  if (detail.isError && !isProductNotFound(detail.error)) return <section className="product-detail-state surface-panel"><StatusMessage kind="info">商品详情暂时没有响应，请稍后重试。</StatusMessage><button className="button button-secondary" onClick={() => navigate(catalogTo)}>返回首页</button></section>
  if (!detail.data) return <section className="product-detail-state surface-panel"><div className="section-kicker">PRODUCT NOT FOUND</div><h1 className="page-title">商品暂时不可用</h1><p>这个商品可能已经下架，或者链接已经过期。</p><button className="button button-secondary" onClick={() => navigate(spaceSlug ? `/spaces/${spaceSlug}#catalog` : '/#catalog')}><ArrowLeft size={16} />返回商品目录</button></section>

  return <ProductDetailView product={detail.data} backTo={spaceSlug ? `/spaces/${spaceSlug}#catalog` : '/#catalog'} redeemTo={spaceSlug ? `/spaces/${spaceSlug}#redeem` : '/#redeem'} />
}
