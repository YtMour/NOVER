import { ArrowLeft, ArrowRight, Check, Download, FileText, KeyRound, ShieldCheck } from 'lucide-react'
import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { Product } from '../../types'
import { getDemoProductVisual } from '../../lib/productVisuals'

export function ProductDetailView({ product, preview = false, imageURLs }: { product: Product; preview?: boolean; imageURLs?: Record<string, string> }) {
  const fallback = getDemoProductVisual(product.id)
  const images = product.images?.length ? product.images : fallback ? [{ id: '__demo_visual__', url: fallback.src, alt_text: fallback.alt, mime_type: 'image/webp', size_bytes: 0 }] : []
  const imageSource = (image: Product['images'] extends (infer T)[] | undefined ? T : never) => image.id === '__demo_visual__' || imageURLs === undefined ? image.url : imageURLs[image.id]
  const markdownImageSource = (source?: string) => {
    if (!source || !imageURLs) return source
    const image = product.images?.find((item) => item.url === source)
    return image ? imageURLs[image.id] ?? source : source
  }

  return (
    <section className={`product-detail-page ${preview ? 'product-detail-page--preview' : ''}`}>
      {preview ? <span className="back-link"><ArrowLeft size={15} />返回商品目录</span> : <Link className="back-link" to="/#catalog"><ArrowLeft size={15} />返回商品目录</Link>}
      <div className="product-detail-layout">
        <div className="product-detail-copy">
          <div className="eyebrow"><span className="eyebrow-line" /> PRODUCT DETAIL</div>
          <div className="product-detail-index">NOVER / {(product.id || 'DRAFT').slice(0, 8).toUpperCase()}</div>
          <h1 className="product-detail-title">{product.name || '未命名商品'}</h1>
          <p className="product-detail-description">{product.description || '在左侧填写简短介绍，这里会同步显示买家看到的内容。'}</p>
          {product.current_release_version && <div className="product-release-badge"><span>当前版本</span><strong>{product.current_release_version}</strong>{product.current_release_changelog && <small>{product.current_release_changelog}</small>}</div>}
          {product.detail && <div className="product-detail-body markdown-body"><ReactMarkdown remarkPlugins={[remarkGfm]} components={{ img: ({ src, alt, ...props }) => <img {...props} src={markdownImageSource(src)} alt={alt ?? ''} /> }}>{product.detail}</ReactMarkdown></div>}
          <div className="product-detail-meta"><span><ShieldCheck size={15} />私有交付</span><span><Download size={15} />有效期内可重复获取</span></div>
        </div>

        <aside className="product-detail-action surface-panel">
          <div className="tool-icon tool-icon--blue"><KeyRound size={19} /></div>
          <span className="section-kicker">READY WHEN YOU ARE</span>
          <h2>已经完成购买？</h2>
          <p>使用卖家发送的兑换码，查看此商品的文件清单、大小和校验信息。</p>
          {preview ? <span className="button button-primary">输入兑换码 <ArrowRight size={16} /></span> : <Link className="button button-primary" to="/#redeem">输入兑换码 <ArrowRight size={16} /></Link>}
          <div className="panel-footnote"><span className="tiny-status-dot" /><span>实际文件不会出现在公开目录</span></div>
        </aside>
      </div>

      {images.length ? <section className="product-gallery" aria-label={`${product.name || '商品'}效果图片`}>
        {images.map((image, index) => {
          const source = imageSource(image)
          return <figure key={image.id} className={`product-gallery-item ${index === 0 ? 'product-gallery-item--featured' : ''}`}>{source ? <img src={source} alt={image.alt_text || `${product.name}效果图 ${index + 1}`} loading={preview || index === 0 ? 'eager' : 'lazy'} /> : <div className="product-gallery-loading">正在读取效果图…</div>}<figcaption>{image.alt_text || `${product.name || '商品'}效果图 ${index + 1}`}</figcaption></figure>
        })}
      </section> : <section className="product-gallery-empty"><span>PRODUCT PREVIEW</span><p>添加效果图后将在这里展示</p></section>}

      <section className="product-detail-section" aria-labelledby={`detail-includes-title-${preview ? 'preview' : 'public'}`}>
        <div className="section-heading-row"><div><div className="eyebrow"><span className="eyebrow-line" /> DELIVERY NOTES</div><h2 id={`detail-includes-title-${preview ? 'preview' : 'public'}`}>你将获得什么</h2></div><span className="section-side-note">兑换后显示真实文件信息</span></div>
        <div className="detail-points">
          <DetailPoint icon={<FileText size={17} />} title="商品交付说明" copy="当前页面展示商品简介；详细版本说明由运营者在交付时维护。" />
          <DetailPoint icon={<Download size={17} />} title="文件与更新" copy="兑换成功后查看文件名称、大小、SHA-256 和可用下载次数。" />
          <DetailPoint icon={<Check size={17} />} title="清晰的领取记录" copy="每次下载都会经过短期票据校验，权益状态可追踪。" />
        </div>
      </section>
    </section>
  )
}

function DetailPoint({ icon, title, copy }: { icon: ReactNode; title: string; copy: string }) {
  return <article className="detail-point"><span className="detail-point-icon">{icon}</span><div><h3>{title}</h3><p>{copy}</p></div></article>
}
