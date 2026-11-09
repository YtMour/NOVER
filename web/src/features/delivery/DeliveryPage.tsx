import { useEffect, useState } from 'react'
import { ArrowDownToLine, Clipboard, FileArchive, FileCode2, FileText, LogOut, ShieldCheck } from 'lucide-react'
import anime from 'animejs'
import { useMutation, useQuery } from '@tanstack/react-query'
import { createDownloadTicket, loadDelivery } from '../../lib/api'
import { StatusMessage } from '../../components/StatusMessage'
import type { Asset } from '../../types'

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

function fileIcon(fileName: string) {
  const extension = fileName.split('.').pop()?.toLowerCase()
  if (['zip', '7z', 'rar', 'tar', 'gz'].includes(extension ?? '')) return FileArchive
  if (['ts', 'tsx', 'js', 'jsx', 'go', 'py', 'html', 'css'].includes(extension ?? '')) return FileCode2
  return FileText
}

export function DeliveryPage({ token, onLogout }: { token: string; onLogout: () => void }) {
  const delivery = useQuery({ queryKey: ['delivery', token], queryFn: () => loadDelivery(token) })
  const [downloadError, setDownloadError] = useState('')
  const downloadMutation = useMutation({
    mutationFn: (assetId: string) => createDownloadTicket(token, assetId),
    onSuccess: ({ url }) => {
      window.location.assign(url)
      setDownloadError('')
    },
    onError: (error: Error) => setDownloadError(error.message),
  })

  useEffect(() => {
    if (delivery.data && !window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      anime({ targets: '.asset-row', opacity: [0, 1], translateY: [5, 0], delay: anime.stagger(45), duration: 260, easing: 'easeOutQuad' })
    }
  }, [delivery.data])

  if (delivery.isLoading) return <div className="loading-line">正在打开交付空间…</div>
  if (delivery.isError) {
    return (
      <div className="delivery-error surface-panel">
        <StatusMessage>{delivery.error.message}</StatusMessage>
        <button className="button button-secondary" onClick={onLogout}>返回兑换</button>
      </div>
    )
  }
  const data = delivery.data!

  return (
    <section className="delivery-page">
      <div className="delivery-title-row">
        <div>
          <div className="eyebrow"><span className="eyebrow-line" /> DELIVERY SPACE</div>
          <h1 className="page-title">交付空间</h1>
        </div>
        <button className="icon-button" title="退出交付空间" aria-label="退出交付空间" onClick={onLogout}><LogOut size={18} /></button>
      </div>

      <section className="product-overview">
        <div className="product-symbol" aria-hidden="true"><span /><span /><span /></div>
        <div className="product-copy">
          <span className="section-kicker">YOUR DIGITAL PRODUCT</span>
          <h2>{data.product.name}</h2>
          {data.product.description && <p>{data.product.description}</p>}
        </div>
        <div className="access-state"><span className="state-dot" /> 可访问至 {new Date(data.valid_until).toLocaleDateString('zh-CN')}</div>
      </section>

      <div className="content-section-heading">
        <div>
          <span className="section-kicker">FILES & RESOURCES</span>
          <h2>文件与资源 <span className="count-label">{data.assets.length}</span></h2>
        </div>
      </div>

      {downloadError && <StatusMessage>{downloadError}</StatusMessage>}
      {data.assets.length === 0 ? (
        <div className="empty-state">此商品暂时没有可下载的文件。</div>
      ) : (
        <div className="asset-list">
          {data.assets.map((asset) => <AssetRow key={asset.id} asset={asset} onDownload={() => downloadMutation.mutate(asset.id)} pending={downloadMutation.isPending} />)}
        </div>
      )}
      <div className="delivery-assurance"><ShieldCheck size={16} /> 下载链接短时有效；需要重新下载时可再次生成。</div>
    </section>
  )
}

function AssetRow({ asset, onDownload, pending }: { asset: Asset; onDownload: () => void; pending: boolean }) {
  const [copied, setCopied] = useState(false)
  const Icon = fileIcon(asset.file_name)

  async function copyHash() {
    await navigator.clipboard.writeText(asset.sha256)
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1500)
  }

  return (
    <article className="asset-row">
      <div className="asset-file-icon"><Icon size={19} strokeWidth={1.8} /></div>
      <div className="asset-details">
        <h3>{asset.file_name}</h3>
        <div className="asset-meta"><span>{formatBytes(asset.size_bytes)}</span><span className="meta-separator" /><span>SHA-256</span><code>{asset.sha256.slice(0, 12)}…</code></div>
      </div>
      <button className="hash-copy" title="复制 SHA-256 校验值" aria-label="复制 SHA-256 校验值" onClick={copyHash}>
        <Clipboard size={15} /> <span>{copied ? '已复制' : '校验值'}</span>
      </button>
      <button className="button button-download" onClick={onDownload} disabled={pending}>
        <ArrowDownToLine size={16} /><span>{pending ? '准备中' : '下载'}</span>
      </button>
    </article>
  )
}
