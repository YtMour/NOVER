import { useState, type FormEvent, type ReactNode } from 'react'
import { ArrowDown, ArrowRight, ArrowUpRight, Download, KeyRound, ShieldCheck, ShoppingBag } from 'lucide-react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Link, useNavigate } from 'react-router-dom'
import { loadPublicProducts, redeemCode } from '../../lib/api'
import { getProductVisual } from '../../lib/productVisuals'
import { StatusMessage } from '../../components/StatusMessage'

export function RedeemPage() {
  const [code, setCode] = useState('')
  const navigate = useNavigate()
  const products = useQuery({ queryKey: ['public-products'], queryFn: loadPublicProducts })
  const mutation = useMutation({
    mutationFn: redeemCode,
    onSuccess: ({ access_token }) => {
      sessionStorage.setItem('nover.deliveryToken', access_token)
      navigate('/delivery')
    },
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    mutation.mutate(code)
  }

  return (
    <div className="home-page">
      <section className="home-hero">
        <div className="home-hero-copy">
          <div className="eyebrow"><span className="eyebrow-line" /> NOVER DIGITAL SPACE</div>
          <h1>探索数字，<br /><span>发现更多可能。</span></h1>
          <p className="home-lead">知屿是一个安静、可靠的数字产品空间。我们把独立软件、源码与创意资源整理成清晰的交付体验。</p>
          <div className="hero-actions">
            <a className="button button-primary" href="#catalog">浏览产品 <ArrowDown size={16} /></a>
            <a className="text-action" href="#redeem">已有兑换码 <ArrowRight size={15} /></a>
          </div>
          <div className="hero-assurance"><ShieldCheck size={17} /><span>一次购买，获得清晰、可追踪的数字交付。</span></div>
        </div>
        <div className="hero-product-strip" aria-label="NOVER 服务摘要"><span><strong>03</strong> 精选产品</span><span><strong>1:1</strong> 商品凭证</span><span><strong>24/7</strong> 自助领取</span></div>
      </section>

      <section id="catalog" className="catalog-section">
        <div className="section-heading-row">
          <div><div className="eyebrow"><span className="eyebrow-line" /> SELECTED COLLECTION</div><h2>正在交付的产品</h2></div>
          <span className="section-side-note">{String(products.data?.items.length ?? 0).padStart(2, '0')} PRODUCTS · 精选数字工具与创作资源</span>
        </div>
        {products.isError ? <StatusMessage kind="info">产品目录暂时无法加载，兑换入口仍可正常使用。</StatusMessage> : products.isLoading ? <div className="catalog-loading">正在整理产品目录…</div> : products.data?.items.length ? (
          <div className="product-grid">
            {products.data.items.map((product, index) => <ProductCard key={product.id} product={product} index={index} />)}
          </div>
        ) : <div className="catalog-empty">当前没有公开商品，稍后再来看看。</div>}
      </section>

      <section className="delivery-flow" aria-labelledby="delivery-flow-title">
        <div className="section-heading-row delivery-flow-heading">
          <div><div className="eyebrow"><span className="eyebrow-line" /> SIMPLE DELIVERY</div><h2 id="delivery-flow-title">从购买到领取</h2></div>
          <span className="section-side-note">三步进入你的专属交付空间</span>
        </div>
        <div className="flow-grid">
          <FlowStep index="01" icon={<ShoppingBag size={17} />} title="完成购买" copy="在你熟悉的平台选择需要的数字产品。" />
          <FlowStep index="02" icon={<KeyRound size={17} />} title="获得兑换码" copy="卖家发送一次性兑换码或取件凭证。" />
          <FlowStep index="03" icon={<Download size={17} />} title="进入交付空间" copy="输入兑换码，按需获取文件与后续更新。" />
        </div>
      </section>

      <section id="redeem" className="redeem-layout home-redeem-layout">
        <div className="redeem-copy">
          <div className="eyebrow"><span className="eyebrow-line" /> DIGITAL DELIVERY</div>
          <h2 className="redeem-title">领取你的<br /><span>数字内容。</span></h2>
          <p className="lead-copy">输入购买后获得的兑换码，前往专属交付空间。</p>
          <div className="quiet-note"><ShieldCheck size={18} strokeWidth={1.7} /><span>兑换后可在有效期内重新获取文件与更新。</span></div>
        </div>
        <div className="redeem-tool surface-panel">
          <div className="panel-heading"><div className="tool-icon"><KeyRound size={19} /></div><div><span className="section-kicker">ACCESS KEY</span><h2>输入兑换码</h2></div></div>
          <form onSubmit={submit} className="redeem-form">
            <label htmlFor="redeem-code">兑换码</label>
            <div className="input-wrap"><input id="redeem-code" name="code" value={code} onChange={(event) => setCode(event.target.value.toUpperCase())} placeholder="NVR-XXXX-XXXX-XXXX-XXXX-XXXX-XXXX-XX" autoCapitalize="characters" autoComplete="off" spellCheck={false} required /></div>
            <div className="input-hint">兑换码包含 26 个字符，请留意字母与数字。</div>
            {mutation.isError && <StatusMessage>{mutation.error.message}</StatusMessage>}
            <button className="button button-primary redeem-submit" type="submit" disabled={mutation.isPending}><span>{mutation.isPending ? '正在核验' : '继续领取'}</span><ArrowRight size={17} /></button>
          </form>
          <div className="panel-footnote"><span className="tiny-status-dot" /><span>你的文件由独立交付空间安全托管</span></div>
        </div>
      </section>
    </div>
  )
}

function ProductCard({ product, index }: { product: { id: string; name: string; description: string; images?: { url: string; alt_text: string }[] }; index: number }) {
  const cover = product.images?.[0]
  const fallback = getProductVisual(product.id, index)
  return <Link to={`/products/${product.id}`} className={`product-card product-card--${index % 3}`}>
    <div className="product-card-cover"><img src={cover?.url ?? fallback.src} alt={cover?.alt_text || fallback.alt} loading={index === 0 ? 'eager' : 'lazy'} /><span>数字产品</span></div>
    <div className="product-card-copy"><div className="product-card-top"><span className="product-index">0{index + 1}</span><ArrowUpRight size={16} /></div><h3>{product.name}</h3><p>{product.description || '数字产品交付资源。'}</p><div className="product-card-foot"><span>查看商品详情</span><span>NOVER / {product.id.slice(0, 6).toUpperCase()}</span></div></div>
  </Link>
}

function FlowStep({ index, icon, title, copy }: { index: string; icon: ReactNode; title: string; copy: string }) {
  return <article className="flow-step"><div className="flow-step-top"><span className="product-index">{index}</span><span className="flow-step-icon">{icon}</span></div><h3>{title}</h3><p>{copy}</p></article>
}
