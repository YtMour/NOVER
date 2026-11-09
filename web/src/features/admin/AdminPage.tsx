import { useEffect, useState, type FormEvent } from 'react'
import { Ban, Check, Copy, FileUp, KeyRound, LockKeyhole, PackagePlus, Pause, Plus, RefreshCw, RotateCcw, TicketCheck } from 'lucide-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createProduct, issueCodes, loadActivity, loadProducts, operatorLogin, updateCodeAction, uploadAsset } from '../../lib/api'
import { StatusMessage } from '../../components/StatusMessage'
import type { Product } from '../../types'

export function AdminPage() {
  const [adminToken, setAdminToken] = useState(() => sessionStorage.getItem('nover.operatorToken') ?? '')
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [selectedId, setSelectedId] = useState('')
  const [createdCodes, setCreatedCodes] = useState<string[]>([])
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')
  const queryClient = useQueryClient()
  const products = useQuery({ queryKey: ['admin-products', adminToken], queryFn: () => loadProducts(adminToken), enabled: adminToken.length > 0, retry: false })
  const activity = useQuery({ queryKey: ['admin-activity', adminToken], queryFn: () => loadActivity(adminToken), enabled: adminToken.length > 0, retry: false })

  useEffect(() => {
    if (products.data && !selectedId && products.data.items[0]) setSelectedId(products.data.items[0].id)
  }, [products.data, selectedId])

  const loginMutation = useMutation({
    mutationFn: () => operatorLogin(username, password),
    onSuccess: ({ access_token }) => {
      sessionStorage.setItem('nover.operatorToken', access_token)
      setAdminToken(access_token)
      setPassword('')
      setError('')
    },
  })

  function connect(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    loginMutation.mutate()
  }

  function disconnect() {
    sessionStorage.removeItem('nover.operatorToken')
    setAdminToken('')
    setPassword('')
    setSelectedId('')
    setCreatedCodes([])
  }

  if (!adminToken || products.isError) {
    return (
      <section className="admin-lock surface-panel">
        <div className="tool-icon"><LockKeyhole size={19} /></div>
        <span className="section-kicker">OPERATOR WORKSPACE</span>
        <h1 className="page-title">运营工作台</h1>
        <p>使用运营账号登录，管理商品、交付文件和兑换凭证。</p>
        <form className="stack-form" onSubmit={connect}>
          <label htmlFor="operator-username">账号</label>
          <input id="operator-username" autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} required />
          <label htmlFor="operator-password">密码</label>
          <input id="operator-password" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} required />
          {(products.isError || loginMutation.isError) && <StatusMessage>{loginMutation.error?.message ?? products.error?.message ?? '登录状态已失效，请重新登录。'}</StatusMessage>}
          <button className="button button-primary" type="submit" disabled={loginMutation.isPending}><LockKeyhole size={16} />{loginMutation.isPending ? '登录中…' : '进入工作台'}</button>
        </form>
      </section>
    )
  }

  async function reloadProducts() {
    await queryClient.invalidateQueries({ queryKey: ['admin-products', adminToken] })
  }

  async function reloadActivity() {
    await queryClient.invalidateQueries({ queryKey: ['admin-activity', adminToken] })
  }

  function handleError(error: Error) {
    setError(error.message)
    setNotice('')
  }

  return (
    <section className="admin-page">
      <div className="delivery-title-row">
        <div>
          <div className="eyebrow"><span className="eyebrow-line" /> OPERATOR CONSOLE</div>
          <h1 className="page-title">运营工作台</h1>
        </div>
        <button className="icon-button" title="断开后台会话" aria-label="断开后台会话" onClick={disconnect}><LockKeyhole size={18} /></button>
      </div>

      <div className="admin-grid">
        <section className="admin-section">
          <div className="admin-section-heading"><div className="tool-icon tool-icon--green"><PackagePlus size={18} /></div><div><span className="section-kicker">CATALOG</span><h2>商品目录</h2></div></div>
          <ProductCreate token={adminToken} onCreated={async (product) => { setSelectedId(product.id); setNotice('商品已创建。'); setError(''); await reloadProducts() }} onError={handleError} />
          <div className="product-picker-head"><span>现有商品</span><button className="text-icon-button" onClick={() => void reloadProducts()} title="刷新商品"><RefreshCw size={14} />刷新</button></div>
          {products.isLoading ? <div className="empty-state">正在读取…</div> : products.data?.items.length ? (
            <div className="product-picker">
              {products.data.items.map((item) => <button key={item.id} className={`product-option ${selectedId === item.id ? 'is-selected' : ''}`} onClick={() => setSelectedId(item.id)}><span>{item.name}</span><span className="product-option-status">{item.status === 'ACTIVE' ? '可用' : '已归档'}</span></button>)}
            </div>
          ) : <div className="empty-state">创建第一个商品以开始交付。</div>}
        </section>

        <section className="admin-section">
          <div className="admin-section-heading"><div className="tool-icon tool-icon--blue"><FileUp size={18} /></div><div><span className="section-kicker">PRIVATE ASSETS</span><h2>上传交付文件</h2></div></div>
          <AssetUpload token={adminToken} productId={selectedId} onUploaded={() => { setNotice('文件已上传并记录校验值。'); setError('') }} onError={handleError} />
        </section>

        <section className="admin-section admin-section--codes">
          <div className="admin-section-heading"><div className="tool-icon tool-icon--amber"><TicketCheck size={18} /></div><div><span className="section-kicker">REDEEM CODES</span><h2>签发兑换码</h2></div></div>
          <CodeIssuer token={adminToken} productId={selectedId} codes={createdCodes} onIssued={(codes) => { setCreatedCodes(codes); setNotice(`已签发 ${codes.length} 个兑换码，请立即复制保存。`); setError('') }} onError={handleError} />
        </section>
      </div>
      <section className="activity-section">
        <div className="admin-section-heading"><div className="tool-icon"><TicketCheck size={18} /></div><div><span className="section-kicker">RECENT DELIVERIES</span><h2>兑换与下载记录</h2></div><button className="text-icon-button activity-refresh" title="刷新记录" onClick={() => void reloadActivity()}><RefreshCw size={14} />刷新</button></div>
        {activity.isError && <StatusMessage>{activity.error.message}</StatusMessage>}
        {activity.isLoading ? <div className="empty-state">正在读取记录…</div> : activity.data?.items.length ? (
          <div className="activity-list">
            {activity.data.items.map((item) => <ActivityRow key={item.code_id} item={item} token={adminToken} onChanged={async () => { setNotice('兑换凭证状态已更新。'); setError(''); await reloadActivity() }} onError={handleError} />)}
          </div>
        ) : <div className="empty-state">签发兑换码后，记录会显示在这里。</div>}
      </section>
      {error && <StatusMessage>{error}</StatusMessage>}
      {notice && <StatusMessage kind="success">{notice}</StatusMessage>}
    </section>
  )
}

function ActivityRow({ item, token, onChanged, onError }: { item: { code_id: string; product_name: string; code_suffix: string; code_status: string; external_order_ref: string; code_expires_at: string; redeemed_at: string | null; entitlement_until: string | null; downloads_issued: number; download_events: number }; token: string; onChanged: () => void; onError: (error: Error) => void }) {
  const mutation = useMutation({
    mutationFn: (action: 'SUSPEND' | 'RESUME' | 'REVOKE') => updateCodeAction(token, item.code_id, action),
    onSuccess: onChanged,
    onError,
  })
  const suspended = item.code_status === 'SUSPENDED'
  const terminal = ['REVOKED', 'REFUNDED', 'EXPIRED'].includes(item.code_status)
  function revoke() {
    if (window.confirm(`撤销凭证尾号 ${item.code_suffix}？已生成的交付权限也会立即失效。`)) mutation.mutate('REVOKE')
  }
  return (
    <article className="activity-row">
      <div className="activity-code"><code>•••• {item.code_suffix}</code><span className={`status-pill status-pill--${item.code_status.toLowerCase()}`}>{statusLabel(item.code_status)}</span></div>
      <div className="activity-product"><strong>{item.product_name}</strong><span>{item.external_order_ref ? `订单 ${item.external_order_ref}` : '未关联外部订单'}</span></div>
      <div className="activity-dates"><span>{item.redeemed_at ? `已兑换 ${new Date(item.redeemed_at).toLocaleDateString('zh-CN')}` : `兑换截止 ${new Date(item.code_expires_at).toLocaleDateString('zh-CN')}`}</span><span>{item.entitlement_until ? `权益至 ${new Date(item.entitlement_until).toLocaleDateString('zh-CN')}` : '尚未建立权益'}</span></div>
      <div className="activity-downloads"><strong>{item.downloads_issued}</strong><span>已签发下载票据</span><small>{item.download_events} 次文件请求</small></div>
      <div className="activity-actions">
        {!terminal && <button className="icon-button icon-button--small" title={suspended ? '恢复兑换/交付' : '暂停兑换/交付'} aria-label={suspended ? '恢复兑换/交付' : '暂停兑换/交付'} disabled={mutation.isPending} onClick={() => mutation.mutate(suspended ? 'RESUME' : 'SUSPEND')}>{suspended ? <RotateCcw size={15} /> : <Pause size={15} />}</button>}
        {!terminal && <button className="icon-button icon-button--small icon-button--danger" title="撤销兑换凭证" aria-label="撤销兑换凭证" disabled={mutation.isPending} onClick={revoke}><Ban size={15} /></button>}
      </div>
    </article>
  )
}

function statusLabel(status: string) {
  const labels: Record<string, string> = { ASSIGNED: '待兑换', REDEEMED: '已兑换', SUSPENDED: '已暂停', REVOKED: '已撤销', REFUNDED: '已退款', EXPIRED: '已过期' }
  return labels[status] ?? status
}

function ProductCreate({ token, onCreated, onError }: { token: string; onCreated: (product: Product) => void; onError: (error: Error) => void }) {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const mutation = useMutation({ mutationFn: () => createProduct(token, name, description), onSuccess: onCreated, onError })
  return (
    <form className="stack-form compact-form" onSubmit={(event) => { event.preventDefault(); mutation.mutate() }}>
      <label htmlFor="product-name">商品名称</label>
      <input id="product-name" value={name} onChange={(event) => setName(event.target.value)} maxLength={120} required placeholder="例如：NOVER Starter Kit" />
      <label htmlFor="product-description">交付说明 <span className="optional-label">可选</span></label>
      <textarea id="product-description" value={description} onChange={(event) => setDescription(event.target.value)} maxLength={2000} rows={3} placeholder="写给购买者的简短说明" />
      <button className="button button-secondary" type="submit" disabled={mutation.isPending}><Plus size={16} />创建商品</button>
    </form>
  )
}

function AssetUpload({ token, productId, onUploaded, onError }: { token: string; productId: string; onUploaded: () => void; onError: (error: Error) => void }) {
  const [file, setFile] = useState<File | null>(null)
  const mutation = useMutation({ mutationFn: () => uploadAsset(token, productId, file!), onSuccess: () => { setFile(null); onUploaded() }, onError })
  return (
    <form className="stack-form compact-form" onSubmit={(event) => { event.preventDefault(); if (file) mutation.mutate() }}>
      <label htmlFor="asset-file">选择交付文件</label>
      <input id="asset-file" className="file-input" type="file" onChange={(event) => setFile(event.target.files?.[0] ?? null)} disabled={!productId} />
      <p className="field-note">单文件上限由服务端配置；文件保存于私有目录，数据库记录 SHA-256。</p>
      <button className="button button-secondary" type="submit" disabled={!file || !productId || mutation.isPending}><FileUp size={16} />{mutation.isPending ? '上传中…' : '上传到当前商品'}</button>
    </form>
  )
}

function CodeIssuer({ token, productId, codes, onIssued, onError }: { token: string; productId: string; codes: string[]; onIssued: (codes: string[]) => void; onError: (error: Error) => void }) {
  const [count, setCount] = useState(1)
  const [orderRef, setOrderRef] = useState('')
  const [copied, setCopied] = useState(false)
  const mutation = useMutation({
    mutationFn: () => issueCodes(token, productId, { count, code_ttl_days: 90, access_days: 365, max_downloads: 25, external_order_ref: orderRef }),
    onSuccess: ({ codes: nextCodes }) => onIssued(nextCodes),
    onError,
  })
  async function copyAll() {
    await navigator.clipboard.writeText(codes.join('\n'))
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1600)
  }
  return (
    <div className="stack-form compact-form">
      <label htmlFor="order-ref">外部订单编号 <span className="optional-label">可选</span></label>
      <input id="order-ref" value={orderRef} onChange={(event) => setOrderRef(event.target.value)} maxLength={200} placeholder="用于售后对账" />
      <label htmlFor="code-count">生成数量</label>
      <div className="number-field"><button type="button" aria-label="减少数量" onClick={() => setCount((value) => Math.max(1, value - 1))}>−</button><input id="code-count" type="number" min={1} max={100} value={count} onChange={(event) => setCount(Number(event.target.value))} /><button type="button" aria-label="增加数量" onClick={() => setCount((value) => Math.min(100, value + 1))}>+</button></div>
      <div className="policy-summary"><span>兑换有效期 <strong>90 天</strong></span><span>领取后可访问 <strong>365 天</strong></span><span>下载额度 <strong>25 次</strong></span></div>
      <button className="button button-primary" type="button" disabled={!productId || mutation.isPending} onClick={() => mutation.mutate()}><KeyRound size={16} />{mutation.isPending ? '签发中…' : '签发兑换码'}</button>
      {codes.length > 0 && <div className="issued-codes"><div className="issued-codes-heading"><span>新签发凭证</span><button className="text-icon-button" onClick={() => void copyAll()}><Copy size={14} />{copied ? '已复制' : '复制全部'}</button></div><pre>{codes.join('\n')}</pre><div className="code-warning"><Check size={14} />兑换码仅在当前后台会话显示，请及时保存。</div></div>}
    </div>
  )
}
