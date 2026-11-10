import { ArrowLeft } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate, useParams } from 'react-router-dom'
import { StatusMessage } from '../../components/StatusMessage'
import { loadPublicProducts } from '../../lib/api'
import { ProductDetailView } from './ProductDetailView'

export function ProductDetailPage() {
  const { productID } = useParams()
  const navigate = useNavigate()
  const products = useQuery({ queryKey: ['public-products'], queryFn: loadPublicProducts })
  const product = products.data?.items.find((item) => item.id === productID)

  if (products.isLoading) return <div className="loading-line">正在读取商品信息…</div>
  if (products.isError) return <section className="product-detail-state surface-panel"><StatusMessage kind="info">商品目录暂时没有响应，请稍后重试。</StatusMessage><button className="button button-secondary" onClick={() => navigate('/')}>返回首页</button></section>
  if (!product) return <section className="product-detail-state surface-panel"><div className="section-kicker">PRODUCT NOT FOUND</div><h1 className="page-title">商品暂时不可用</h1><p>这个商品可能已经下架，或者链接已经过期。</p><button className="button button-secondary" onClick={() => navigate('/#catalog')}><ArrowLeft size={16} />返回商品目录</button></section>

  return <ProductDetailView product={product} />
}
