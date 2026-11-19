import { ArrowLeft, ArrowRight, CalendarClock, Check, Cpu, Download, FileText, KeyRound, RefreshCw, ShieldCheck } from 'lucide-react'
import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { Product } from '../../types'
import { getDemoProductVisual } from '../../lib/productVisuals'
import { updatePolicyLabel } from '../../lib/productProfile'

export function ProductDetailView({ product, preview = false, imageURLs, backTo = '/#catalog', redeemTo = '/#redeem' }: { product: Product; preview?: boolean; imageURLs?: Record<string, string>; backTo?: string; redeemTo?: string }) {
  const fallback = getDemoProductVisual(product.id)
  const images = product.images?.length ? product.images : fallback ? [{ id: '__demo_visual__', url: fallback.src, alt_text: fallback.alt, mime_type: 'image/webp', size_bytes: 0 }] : []
  const imageSource = (image: Product['images'] extends (infer T)[] | undefined ? T : never) => image.id === '__demo_visual__' || imageURLs === undefined ? image.url : imageURLs[image.id]
  const markdownImageSource = (source?: string) => {
    if (!source || !imageURLs) return source
    const image = product.images?.find((item) => item.url === source)
    return image ? imageURLs[image.id] ?? source : source
  }

  // v0.3.0 商品资料：旧版 API 不返回这些字段，统一按空值处理，空区域不渲染。
  const sectionSuffix = preview ? 'preview' : 'public'
  const highlights = (product.highlights ?? []).map((highlight) => highlight.trim()).filter(Boolean)
  const licenseTerms = product.license_terms?.trim() ?? ''
  const supportPeriod = product.support_period?.trim() ?? ''
  const requirements = product.requirements?.trim() ?? ''
  const updatePolicy = updatePolicyLabel(product.update_policy, product.update_until)

  return (
    <section className={`product-detail-page ${preview ? 'product-detail-page--preview' : ''}`}>
      {preview ? <span className="back-link"><ArrowLeft size={15} />返回商品目录</span> : <Link className="back-link" to={backTo}><ArrowLeft size={15} />返回商品目录</Link>}
      <div className="product-detail-layout">
        <div className="product-detail-copy">
          <div className="eyebrow"><span className="eyebrow-line" /> PRODUCT DETAIL</div>
          <div className="product-detail-index">NOVER / {(product.id || 'DRAFT').slice(0, 8).toUpperCase()}</div>
          <h1 className="product-detail-title">{product.name || '未命名商品'}</h1>
          <p className="product-detail-description">{product.description || '在左侧填写简短介绍，这里会同步显示买家看到的内容。'}</p>
          {highlights.length > 0 && <section className="product-highlights-block" aria-labelledby={`detail-highlights-title-${sectionSuffix}`}>
            <h2 className="product-highlights-title" id={`detail-highlights-title-${sectionSuffix}`}>商品亮点</h2>
            <ul className="product-highlights">
              {highlights.map((highlight, index) => <li key={`${index}-${highlight}`}>{highlight}</li>)}
            </ul>
          </section>}
          {product.current_release_version && <div className="product-release-badge"><span>当前版本</span><strong>{product.current_release_version}</strong>{product.current_release_changelog && <small>{product.current_release_changelog}</small>}</div>}
          {product.detail && <div className="product-detail-body markdown-body"><ReactMarkdown remarkPlugins={[remarkGfm]} components={{ img: ({ src, alt, ...props }) => <img {...props} src={markdownImageSource(src)} alt={alt ?? ''} /> }}>{product.detail}</ReactMarkdown></div>}
          <div className="product-detail-meta"><span><ShieldCheck size={15} />私有交付</span><span><Download size={15} />有效期内可重复获取</span></div>
        </div>

        <aside className="product-detail-action surface-panel">
          <div className="tool-icon tool-icon--blue"><KeyRound size={19} /></div>
          <span className="section-kicker">READY WHEN YOU ARE</span>
          <h2>已经完成购买？</h2>
          <p>使用卖家发送的兑换码，查看此商品的文件清单、大小和校验信息。</p>
          {preview ? <span className="button button-primary">输入兑换码 <ArrowRight size={16} /></span> : <Link className="button button-primary" to={redeemTo}>输入兑换码 <ArrowRight size={16} /></Link>}
          <div className="panel-footnote"><span className="tiny-status-dot" /><span>实际文件不会出现在公开目录</span></div>
        </aside>
      </div>

      {images.length ? <section className="product-gallery" aria-label={`${product.name || '商品'}效果图片`}>
        {images.map((image, index) => {
          const source = imageSource(image)
          return <figure key={image.id} className={`product-gallery-item ${index === 0 ? 'product-gallery-item--featured' : ''}`}>{source ? <img src={source} alt={image.alt_text || `${product.name}效果图 ${index + 1}`} loading={preview || index === 0 ? 'eager' : 'lazy'} /> : <div className="product-gallery-loading">正在读取效果图…</div>}<figcaption>{image.alt_text || `${product.name || '商品'}效果图 ${index + 1}`}</figcaption></figure>
        })}
      </section> : <section className="product-gallery-empty"><span>PRODUCT PREVIEW</span><p>添加效果图后将在这里展示</p></section>}

      <section className="product-detail-section product-profile" aria-labelledby={`detail-profile-title-${sectionSuffix}`}>
        <div className="section-heading-row"><div><div className="eyebrow"><span className="eyebrow-line" /> PRODUCT PROFILE</div><h2 id={`detail-profile-title-${sectionSuffix}`}>商品资料</h2></div><span className="section-side-note">购买前请确认交付与授权范围</span></div>
        <dl className="product-profile-grid">
          {licenseTerms && <ProfileItem icon={<FileText size={15} aria-hidden="true" />} title="交付与授权"><p className="product-profile-text">{licenseTerms}</p></ProfileItem>}
          {supportPeriod && <ProfileItem icon={<CalendarClock size={15} aria-hidden="true" />} title="支持期"><p className="product-profile-text">{supportPeriod}</p></ProfileItem>}
          {requirements && <ProfileItem icon={<Cpu size={15} aria-hidden="true" />} title="技术要求"><p className="product-profile-text">{requirements}</p></ProfileItem>}
          <ProfileItem icon={<RefreshCw size={15} aria-hidden="true" />} title="更新策略"><p className="product-profile-update">{updatePolicy}</p></ProfileItem>
        </dl>
      </section>

      <section className="product-detail-section" aria-labelledby={`detail-includes-title-${sectionSuffix}`}>
        <div className="section-heading-row"><div><div className="eyebrow"><span className="eyebrow-line" /> DELIVERY NOTES</div><h2 id={`detail-includes-title-${sectionSuffix}`}>你将获得什么</h2></div><span className="section-side-note">兑换后显示真实文件信息</span></div>
        <div className="detail-points">
          <DetailPoint icon={<FileText size={17} />} title="商品交付说明" copy="当前页面展示商品简介；详细版本说明由运营者在交付时维护。" />
          <DetailPoint icon={<Download size={17} />} title="文件与更新" copy="兑换成功后查看文件名称、大小、SHA-256 和可用下载次数。" />
          <DetailPoint icon={<Check size={17} />} title="清晰的领取记录" copy="每次下载都会经过短期票据校验，权益状态可追踪。" />
        </div>
      </section>
    </section>
  )
}

function ProfileItem({ icon, title, children }: { icon: ReactNode; title: string; children: ReactNode }) {
  return <div className="product-profile-item"><dt>{icon}<span>{title}</span></dt><dd>{children}</dd></div>
}

function DetailPoint({ icon, title, copy }: { icon: ReactNode; title: string; copy: string }) {
  return <article className="detail-point"><span className="detail-point-icon">{icon}</span><div><h3>{title}</h3><p>{copy}</p></div></article>
}
