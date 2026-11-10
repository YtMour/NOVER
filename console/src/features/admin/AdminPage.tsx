import { useEffect, useMemo, useState, type ClipboardEvent, type FormEvent } from 'react'
import { Archive, Ban, BarChart3, Building2, Check, Copy, Eye, FileUp, ImagePlus, KeyRound, LayoutDashboard, LockKeyhole, PackagePlus, Pause, Plus, RefreshCw, RotateCcw, Save, Send, ServerCog, Settings2, ShieldCheck, TicketCheck, Trash2, UserCheck, UsersRound, X } from 'lucide-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createProduct, deleteProductImage, issueCodes, loadActivity, loadJoinApplications, loadOperatorSession, loadPlatformSettings, loadProductImageBlob, loadProductImages, loadProducts, loadTenantMembers, operatorLogin, reviewJoinApplication, revokeOperatorSession, savePlatformSettings, updateCodeAction, updateProduct, updateProductStatus, uploadAsset, uploadProductImage } from '../../lib/api'
import { StatusMessage } from '../../components/StatusMessage'
import { ProductDetailView } from '../redeem/ProductDetailView'
import type { AdminActivity, AdminActivitySummary, JoinApplication, OperatorSession, PlatformSettings, Product, ProductDraftInput, ProductImage, TenantMember } from '../../types'

type AdminView = 'overview' | 'products' | 'codes' | 'applications' | 'members' | 'settings'

export function AdminPage() {
  const [adminToken, setAdminToken] = useState(() => sessionStorage.getItem('nover.operatorToken') ?? '')
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [selectedId, setSelectedId] = useState('')
  const [creatingDraft, setCreatingDraft] = useState(false)
  const [createdCodes, setCreatedCodes] = useState<string[]>([])
  const [view, setView] = useState<AdminView>('overview')
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')
  const queryClient = useQueryClient()
  const session = useQuery({ queryKey: ['operator-session', adminToken], queryFn: () => loadOperatorSession(adminToken), enabled: adminToken.length > 0, retry: false })
  const hasPermission = (permission: string) => session.data?.permissions.includes(permission) ?? false
  const products = useQuery({ queryKey: ['admin-products', adminToken], queryFn: () => loadProducts(adminToken), enabled: adminToken.length > 0 && hasPermission('product.read'), retry: false })
  const activity = useQuery({ queryKey: ['admin-activity', adminToken], queryFn: () => loadActivity(adminToken), enabled: adminToken.length > 0 && hasPermission('activity.read'), retry: false })

  useEffect(() => {
    if (products.data && !selectedId && !creatingDraft && products.data.items[0]) setSelectedId(products.data.items[0].id)
  }, [products.data, selectedId, creatingDraft])

  const selectedProduct = products.data?.items.find((item) => item.id === selectedId)

  const loginMutation = useMutation({
    mutationFn: () => operatorLogin(username, password),
    onSuccess: ({ access_token, session: identity }) => {
      sessionStorage.setItem('nover.operatorToken', access_token)
      setAdminToken(access_token)
      queryClient.setQueryData(['operator-session', access_token], identity)
      setPassword('')
      setError('')
    },
  })

  function connect(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    loginMutation.mutate()
  }

  async function disconnect() {
    if (adminToken) await revokeOperatorSession(adminToken).catch(() => undefined)
    sessionStorage.removeItem('nover.operatorToken')
    setAdminToken('')
    setPassword('')
    setSelectedId('')
    setCreatingDraft(false)
    setCreatedCodes([])
    queryClient.removeQueries({ queryKey: ['operator-session', adminToken] })
  }

  function selectProduct(productId: string) {
    setSelectedId(productId)
    setCreatingDraft(false)
    setCreatedCodes([])
    setNotice('')
    setError('')
  }

  function startDraft() {
    setView('products')
    setSelectedId('')
    setCreatingDraft(true)
    setCreatedCodes([])
    setNotice('')
    setError('')
  }

  if (adminToken && session.isLoading) {
    return <section className="admin-lock surface-panel"><div className="tool-icon"><LockKeyhole size={19} /></div><span className="section-kicker">OPERATOR SESSION</span><h1 className="page-title">正在验证会话</h1><p>正在读取账号、空间和角色权限。</p></section>
  }

  if (!adminToken || session.isError || products.isError) {
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
          {(session.isError || products.isError || loginMutation.isError) && <StatusMessage>{loginMutation.error?.message ?? session.error?.message ?? products.error?.message ?? '登录状态已失效，请重新登录。'}</StatusMessage>}
          <button className="button button-primary" type="submit" disabled={loginMutation.isPending}><LockKeyhole size={16} />{loginMutation.isPending ? '登录中…' : '进入工作台'}</button>
        </form>
      </section>
    )
  }

  const readOnlyApplicant = session.data?.membership_status === 'PENDING'

  if (readOnlyApplicant) {
    const applicant = session.data!
    return <section className="admin-lock applicant-status-view surface-panel">
      <div className="tool-icon"><UserCheck size={19} /></div>
      <span className="section-kicker">APPLICATION STATUS</span>
      <h1 className="page-title">申请状态</h1>
      <p>账号已创建，但当前仍在审核中。审核完成前只能查看账号和申请信息，商品、文件与兑换码操作暂不可用。</p>
      <div className="applicant-status-card"><span>当前账号</span><strong>{applicant.username}</strong><small>{applicant.tenant_name}</small><span>申请状态</span><strong>{applicationStatusLabel(applicant.application_status)}</strong><small>审核通过后请重新登录以刷新权限。</small></div>
      <button className="button button-secondary" type="button" onClick={() => void disconnect()}><LockKeyhole size={16} />退出账号</button>
    </section>
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
        <div className="operator-session"><div><span>{session.data?.username} · 当前空间</span><strong>{session.data?.tenant_name} · {roleLabel(session.data?.role)}</strong></div><button className="icon-button" title="退出运营账号" aria-label="退出运营账号" onClick={() => void disconnect()}><LockKeyhole size={18} /></button></div>
      </div>

      <nav className="admin-view-tabs" aria-label="运营功能">
        <span className="admin-nav-label">运营</span>
        <button className={view === 'overview' ? 'is-active' : ''} onClick={() => setView('overview')}><LayoutDashboard size={16} />数据总览</button>
        {hasPermission('product.write') && <button className={view === 'products' ? 'is-active' : ''} onClick={() => setView('products')}><PackagePlus size={16} />商品创建</button>}
        {hasPermission('code.issue') && <button className={view === 'codes' ? 'is-active' : ''} onClick={() => setView('codes')}><TicketCheck size={16} />兑换码管理</button>}
        <span className="admin-nav-label">组织</span>
        {hasPermission('application.read') && <button className={view === 'applications' ? 'is-active' : ''} onClick={() => setView('applications')}><UsersRound size={16} />成员申请</button>}
        {hasPermission('member.read') && <button className={view === 'members' ? 'is-active' : ''} onClick={() => setView('members')}><ShieldCheck size={16} />成员与权限</button>}
        <span className="admin-nav-label">系统</span>
        {hasPermission('settings.read') && <button className={view === 'settings' ? 'is-active' : ''} onClick={() => setView('settings')}><Settings2 size={16} />部署设置</button>}
        <div className="admin-nav-context"><Building2 size={15} /><div><strong>身份与 RBAC 已启用</strong><span>商品与交付按当前空间隔离</span></div></div>
      </nav>

      {view === 'overview' && <AdminOverview products={products.data?.items ?? []} summary={activity.data?.summary} loading={products.isLoading || activity.isLoading} />}

      {view === 'products' && <><section className="product-studio">
        <div className="product-studio-heading">
          <div className="admin-section-heading"><div className="tool-icon tool-icon--green"><PackagePlus size={18} /></div><div><span className="section-kicker">STEP 01 · CONTENT</span><h2>商品内容与实时预览</h2></div></div>
          <div className="studio-model-note"><span className="tiny-status-dot" />当前为单租户部署</div>
        </div>
        <div className="product-studio-layout">
          <aside className="product-studio-sidebar">
            <button className="button button-secondary studio-new-button" type="button" onClick={startDraft}><Plus size={15} />新建商品草稿</button>
            <div className="product-picker-head"><span>商品</span><button className="text-icon-button" onClick={() => void reloadProducts()} title="刷新商品"><RefreshCw size={14} />刷新</button></div>
            {products.isLoading ? <div className="empty-state">正在读取…</div> : products.data?.items.length ? (
              <div className="product-picker">
                {products.data.items.map((item) => <button type="button" key={item.id} className={`product-option ${selectedId === item.id && !creatingDraft ? 'is-selected' : ''}`} onClick={() => selectProduct(item.id)}><span>{item.name}</span><span className={`product-option-status product-option-status--${item.status.toLowerCase()}`}>{productStatusLabel(item.status)}</span></button>)}
              </div>
            ) : <div className="empty-state">创建第一个商品以开始交付。</div>}
          </aside>
          <ProductEditor
            key={creatingDraft ? 'new-product' : selectedProduct?.id ?? 'empty-product'}
            token={adminToken}
            product={creatingDraft ? undefined : selectedProduct}
            isCreating={creatingDraft}
            onSaved={async (product) => { selectProduct(product.id); await reloadProducts() }}
            onNotice={(message) => { setNotice(message); setError('') }}
            onError={handleError}
          />
        </div>
      </section>

      <div className="admin-grid admin-tools-grid product-workflow-tools">

        <section className="admin-section">
          <div className="admin-section-heading"><div className="tool-icon tool-icon--amber"><ImagePlus size={18} /></div><div><span className="section-kicker">STEP 02 · VISUALS</span><h2>商品效果图片</h2></div></div>
          <ProductImageManager key={selectedId} token={adminToken} productId={selectedId} productName={selectedProduct?.name ?? ''} productStatus={selectedProduct?.status} onUpdated={async () => { setNotice('商品效果图片已更新。'); setError(''); await reloadProducts() }} onError={handleError} />
        </section>

        <section className="admin-section">
          <div className="admin-section-heading"><div className="tool-icon tool-icon--blue"><FileUp size={18} /></div><div><span className="section-kicker">STEP 03 · DELIVERY</span><h2>上传交付文件</h2></div></div>
          <AssetUpload token={adminToken} productId={selectedId} enabled={selectedProduct?.status !== 'ARCHIVED'} onUploaded={async () => { setNotice('文件已上传并记录校验值。'); setError(''); await reloadProducts() }} onError={handleError} />
        </section>

        <section className="admin-section publish-product-section">
          <div className="admin-section-heading"><div className="tool-icon tool-icon--green"><Send size={18} /></div><div><span className="section-kicker">STEP 04 · PUBLISH</span><h2>检查并完成发布</h2></div></div>
          <ProductPublishControl token={adminToken} product={selectedProduct} onPublished={async () => { setNotice(`「${selectedProduct?.name ?? '商品'}」已发布，公开目录现在可以看到。`); setError(''); await reloadProducts() }} onError={handleError} />
        </section>

      </div></>}

      {view === 'codes' && <div className="code-management-layout">
        <section className="admin-section code-issuer-panel">
          <div className="admin-section-heading"><div className="tool-icon tool-icon--amber"><TicketCheck size={18} /></div><div><span className="section-kicker">CODE BINDING</span><h2>生成并绑定兑换码</h2></div></div>
          <label className="code-product-select"><span>绑定商品</span><select value={selectedId} onChange={(event) => selectProduct(event.target.value)}>{products.data?.items.map((item) => <option key={item.id} value={item.id}>{item.name} · {productStatusLabel(item.status)}</option>)}</select></label>
          <CodeIssuer key={selectedId} token={adminToken} productId={selectedId} productName={selectedProduct?.name ?? ''} productStatus={selectedProduct?.status} codes={createdCodes} onIssued={(codes) => { setCreatedCodes(codes); setNotice(`已为「${selectedProduct?.name ?? '当前商品'}」签发 ${codes.length} 个兑换码，请立即复制保存。`); setError('') }} onError={handleError} />
        </section>
      <section className="activity-section code-activity-section">
        <div className="admin-section-heading"><div className="tool-icon"><TicketCheck size={18} /></div><div><span className="section-kicker">RECENT DELIVERIES</span><h2>兑换与下载记录</h2></div><button className="text-icon-button activity-refresh" title="刷新记录" onClick={() => void reloadActivity()}><RefreshCw size={14} />刷新</button></div>
        {activity.isError && <StatusMessage>{activity.error.message}</StatusMessage>}
        {activity.isLoading ? <div className="empty-state">正在读取记录…</div> : activity.data?.items.length ? (
          <div className="activity-list">
            {activity.data.items.map((item) => <ActivityRow key={item.code_id} item={item} token={adminToken} onChanged={async () => { setNotice('兑换凭证状态已更新。'); setError(''); await reloadActivity() }} onError={handleError} />)}
          </div>
        ) : <div className="empty-state">签发兑换码后，记录会显示在这里。</div>}
      </section></div>}
      {view === 'applications' && <ApplicationsView token={adminToken} canReview={hasPermission('application.review')} />}
      {view === 'members' && <MembersView token={adminToken} session={session.data!} />}
      {view === 'settings' && <PlatformSettingsView token={adminToken} canManage={hasPermission('settings.manage')} />}
      {error && <StatusMessage>{error}</StatusMessage>}
      {notice && <StatusMessage kind="success">{notice}</StatusMessage>}
    </section>
  )
}

function ApplicationsView({ token, canReview }: { token: string; canReview: boolean }) {
  const queryClient = useQueryClient()
  const applications = useQuery({ queryKey: ['join-applications', token], queryFn: () => loadJoinApplications(token), retry: false })
  const mutation = useMutation({
    mutationFn: ({ id, action }: { id: string; action: 'REVIEW' | 'APPROVE' | 'REJECT' }) => reviewJoinApplication(token, id, action),
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: ['join-applications', token] }) },
  })
  const items = applications.data?.items ?? []
  const pending = items.filter((item) => item.status === 'PENDING' || item.status === 'REVIEWING').length
  return <section className="applications-view">
    <div className="overview-heading"><div><span className="section-kicker">ACCESS REQUESTS</span><h2>成员与租户申请</h2></div><span>{pending} 项待处理</span></div>
    <div className="application-policy-banner"><ShieldCheck size={19} /><div><strong>待审只读，通过后激活商品权限</strong><span>{canReview ? '申请会先创建隔离租户和只读账号；管理员通过后，成员关系切换为 OWNER 并开放商品、文件与兑换码操作。' : '当前角色拥有申请只读权限，审核操作由平台所有者完成。'}</span></div></div>
    {applications.isError && <StatusMessage>{applications.error.message}</StatusMessage>}
    {mutation.isError && <StatusMessage>{mutation.error.message}</StatusMessage>}
    {applications.isLoading ? <div className="empty-state">正在读取申请…</div> : items.length ? <div className="application-list">{items.map((item) => <ApplicationRow key={item.id} item={item} busy={mutation.isPending} canReview={canReview} onAction={(action) => mutation.mutate({ id: item.id, action })} />)}</div> : <div className="application-empty"><UsersRound size={24} /><strong>暂无加入申请</strong><span>公开申请提交后会显示在这里。</span></div>}
  </section>
}

function ApplicationRow({ item, busy, canReview, onAction }: { item: JoinApplication; busy: boolean; canReview: boolean; onAction: (action: 'REVIEW' | 'APPROVE' | 'REJECT') => void }) {
  const statusText = { PENDING: '待审核', REVIEWING: '审核中', APPROVED: '资格通过', REJECTED: '已拒绝' }[item.status]
  return <article className="application-row">
    <div className="application-identity"><span className="application-avatar">{item.organization_name.slice(0, 1).toUpperCase()}</span><div><strong>{item.organization_name}</strong><span>{item.contact_name} · {item.email}</span></div></div>
    <div className="application-request"><span>期望账号</span><code>{item.desired_username}</code><p>{item.use_case}</p></div>
    <div className="application-meta"><span className={`status-pill application-status--${item.status.toLowerCase()}`}>{statusText}</span><small>{new Date(item.created_at).toLocaleString('zh-CN')}</small></div>
    <div className="application-actions">
      {canReview && item.status === 'PENDING' && <button className="button button-secondary" type="button" disabled={busy} onClick={() => onAction('REVIEW')}><Eye size={15} />进入审核</button>}
      {canReview && (item.status === 'PENDING' || item.status === 'REVIEWING') && <button className="icon-button icon-button--small" type="button" title="通过资格审核" aria-label="通过资格审核" disabled={busy} onClick={() => onAction('APPROVE')}><UserCheck size={15} /></button>}
      {canReview && (item.status === 'PENDING' || item.status === 'REVIEWING') && <button className="icon-button icon-button--small icon-button--danger" type="button" title="拒绝申请" aria-label="拒绝申请" disabled={busy} onClick={() => onAction('REJECT')}><X size={15} /></button>}
    </div>
  </article>
}

function MembersView({ token, session }: { token: string; session: OperatorSession }) {
  const members = useQuery({ queryKey: ['tenant-members', token], queryFn: () => loadTenantMembers(token), retry: false })
  const capabilityRows = [
    ['OWNER', '空间、成员、商品、交付和兑换码的全部租户权限'],
    ['ADMIN', '日常运营与成员管理，不承担平台级配置职责'],
    ['EDITOR', '编辑商品内容与效果图，不直接发布或签发兑换码'],
    ['FULFILLMENT', '上传交付文件、签发和管理兑换凭证'],
    ['ANALYST', '只读查看商品与交付汇总'],
  ]
  return <section className="members-view">
    <div className="overview-heading"><div><span className="section-kicker">TENANT ACCESS</span><h2>成员与权限</h2></div><span>{members.isLoading ? '正在同步成员' : `${members.data?.items.length ?? 0} 位成员`}</span></div>
    <div className="member-security-banner"><ShieldCheck size={19} /><div><strong>权限由后端会话强制校验</strong><span>当前账号：{session.username} · {platformRoleLabel(session.platform_role)}。隐藏按钮只优化界面，API 仍会独立检查每个权限点。</span></div></div>
    {members.isError && <StatusMessage>{members.error.message}</StatusMessage>}
    <div className="members-layout">
      <div className="member-list-panel">
        <div className="member-list-head"><span>成员</span><span>角色</span><span>最近会话</span><span>状态</span></div>
        {members.isLoading ? <div className="empty-state">正在读取成员…</div> : members.data?.items.map((member) => <MemberRow key={member.user_id} member={member} />)}
      </div>
      <aside className="role-capability-panel">
        <span className="section-kicker">ROLE MATRIX</span><h3>租户角色边界</h3>
        <div className="role-capability-list">{capabilityRows.map(([role, description]) => <div key={role}><strong>{roleLabel(role as OperatorSession['role'])}</strong><span>{description}</span></div>)}</div>
        <p>平台角色与租户角色相互独立。申请审核和部署设置属于平台权限，不随租户管理员角色自动获得。</p>
      </aside>
    </div>
  </section>
}

function MemberRow({ member }: { member: TenantMember }) {
  return <article className="member-row">
    <div className="member-identity"><span className="application-avatar">{member.display_name.slice(0, 1).toUpperCase()}</span><div><strong>{member.display_name}</strong><span>@{member.username} · {platformRoleLabel(member.platform_role)}</span></div></div>
    <span className="role-badge">{roleLabel(member.role)}</span>
    <span className="member-last-active">{member.last_active_at ? `最近会话 ${new Date(member.last_active_at).toLocaleString('zh-CN')}` : '尚无会话'}</span>
    <span className={`status-pill member-status--${member.status.toLowerCase()}`}>{member.status === 'ACTIVE' ? '正常' : '已停用'}</span>
  </article>
}

function PlatformSettingsView({ token, canManage }: { token: string; canManage: boolean }) {
  const settings = useQuery({ queryKey: ['platform-settings', token], queryFn: () => loadPlatformSettings(token), retry: false })
  const [draft, setDraft] = useState<Omit<PlatformSettings, 'updated_at'> | null>(null)
  const [copied, setCopied] = useState(false)
  useEffect(() => {
    if (settings.data) {
      const { updated_at: _, ...nextDraft } = settings.data
      setDraft(nextDraft)
    }
  }, [settings.data])
  const mutation = useMutation({ mutationFn: () => savePlatformSettings(token, draft!), onSuccess: (result) => { const { updated_at: _, ...nextDraft } = result; setDraft(nextDraft) } })
  if (settings.isError) return <StatusMessage>{settings.error.message}</StatusMessage>
  if (settings.isLoading || !draft) return <div className="empty-state">正在读取部署设置…</div>
  const envSnippet = `STOREFRONT_URL=${draft.storefront_url}\nCONSOLE_URL=${draft.console_url}\nOPERATOR_WEB_PATH=${draft.operator_web_path}\nOPERATOR_API_PATH=${draft.operator_api_path}\nALLOWED_ORIGINS=${draft.storefront_url},${draft.console_url}`
  async function copyConfig() {
    await navigator.clipboard.writeText(envSnippet)
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1500)
  }
  return <section className="platform-settings-view">
    <div className="overview-heading"><div><span className="section-kicker">DEPLOYMENT CONTROL</span><h2>前后端与路由设置</h2></div><span>保存规划 · 部署时生效</span></div>
    <div className="settings-grid">
      <form className="admin-section settings-form stack-form" onSubmit={(event) => { event.preventDefault(); if (canManage) mutation.mutate() }}>
        <div className="admin-section-heading"><div className="tool-icon"><ServerCog size={18} /></div><div><span className="section-kicker">ORIGINS</span><h2>服务地址</h2></div></div>
        <label>部署模式<select value={draft.deployment_mode} disabled={!canManage} onChange={(event) => setDraft({ ...draft, deployment_mode: event.target.value as PlatformSettings['deployment_mode'] })}><option value="COLOCATED">同源部署</option><option value="SPLIT_ORIGIN">前端与 API 分端口/域名</option><option value="SPLIT_APPS">用户端与控制台独立构建</option></select></label>
        <label>用户端地址<input type="url" value={draft.storefront_url} disabled={!canManage} onChange={(event) => setDraft({ ...draft, storefront_url: event.target.value })} required /></label>
        <label>控制台地址<input type="url" value={draft.console_url} disabled={!canManage} onChange={(event) => setDraft({ ...draft, console_url: event.target.value })} required /></label>
        <label>API 地址<input type="url" value={draft.api_base_url} disabled={!canManage} onChange={(event) => setDraft({ ...draft, api_base_url: event.target.value })} required /></label>
        <div className="settings-path-grid"><label>控制台路由<input value={draft.operator_web_path} disabled={!canManage} onChange={(event) => setDraft({ ...draft, operator_web_path: event.target.value.toLowerCase() })} pattern="/[a-z0-9][a-z0-9_\-]{3,63}" required /></label><label>运营 API 段<input value={draft.operator_api_path} disabled={!canManage} onChange={(event) => setDraft({ ...draft, operator_api_path: event.target.value.toLowerCase() })} pattern="[a-z0-9][a-z0-9_\-]{3,63}" required /></label></div>
        {mutation.isError && <StatusMessage>{mutation.error.message}</StatusMessage>}
        {mutation.isSuccess && <StatusMessage kind="success">设置规划已保存。重新构建 Web/API 后应用新路由。</StatusMessage>}
        {canManage ? <button className="button button-primary" type="submit" disabled={mutation.isPending}><Save size={15} />{mutation.isPending ? '保存中…' : '保存部署规划'}</button> : <StatusMessage kind="success">当前平台角色拥有只读访问。</StatusMessage>}
      </form>
      <aside className="settings-deployment-panel">
        <div className="tool-icon tool-icon--blue"><ShieldCheck size={18} /></div><span className="section-kicker">APPLY CONFIGURATION</span><h3>部署配置</h3><p>运行路由由构建参数和 API 启动环境决定。数据库保存的是目标配置，不会在请求处理中动态改写路由。</p>
        <pre>{envSnippet}</pre>
        <button className="button button-secondary" type="button" onClick={() => void copyConfig()}><Copy size={15} />{copied ? '已复制' : '复制环境变量'}</button>
        <div className="settings-security-note"><ShieldCheck size={16} /><span>非标路径只降低随机扫描噪声，身份认证、RBAC、限流和审计仍是访问边界。</span></div>
      </aside>
    </div>
  </section>
}

function AdminOverview({ products, summary, loading }: { products: Product[]; summary?: AdminActivitySummary; loading: boolean }) {
  const activeProducts = products.filter((item) => item.status === 'ACTIVE').length
  const redeemed = summary?.redeemed_codes ?? 0
  const totalCodes = summary?.total_codes ?? 0
  const downloadEvents = summary?.download_events ?? 0
  const productRows = products.map((product) => {
    const row = summary?.products.find((item) => item.product_id === product.id)
    return { product, codes: row?.codes ?? 0, redeemed: row?.redeemed ?? 0, downloads: row?.download_events ?? 0 }
  })
  const maxCodes = Math.max(1, ...productRows.map((row) => row.codes))
  return <section className="admin-overview">
    <div className="overview-heading"><div><span className="section-kicker">OPERATIONS SNAPSHOT</span><h2>商品与交付总览</h2></div><span>{loading ? '正在同步数据' : '当前部署实时汇总'}</span></div>
    <div className="metric-grid">
      <article><PackagePlus size={17} /><span>商品</span><strong>{products.length}</strong><small>{activeProducts} 个已发布</small></article>
      <article><KeyRound size={17} /><span>兑换码</span><strong>{totalCodes}</strong><small>{totalCodes - redeemed} 个未兑换或已停用</small></article>
      <article><Check size={17} /><span>已兑换</span><strong>{redeemed}</strong><small>{totalCodes ? Math.round(redeemed / totalCodes * 100) : 0}% 兑换率</small></article>
      <article><BarChart3 size={17} /><span>文件请求</span><strong>{downloadEvents}</strong><small>累计下载事件</small></article>
    </div>
    <div className="product-performance">
      <div className="product-performance-head"><span>商品</span><span>兑换码分布</span><span>已兑换</span><span>下载</span></div>
      {productRows.map((row) => <article key={row.product.id}><div><strong>{row.product.name}</strong><small>{productStatusLabel(row.product.status)}</small></div><div className="performance-bar"><i style={{ width: `${Math.max(4, row.codes / maxCodes * 100)}%` }} /><span>{row.codes}</span></div><strong>{row.redeemed}</strong><strong>{row.downloads}</strong></article>)}
      {!productRows.length && <div className="empty-state">创建商品后将在这里显示交付图谱。</div>}
    </div>
  </section>
}

function ActivityRow({ item, token, onChanged, onError }: { item: AdminActivity; token: string; onChanged: () => void; onError: (error: Error) => void }) {
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

function ProductEditor({ token, product, isCreating, onSaved, onNotice, onError }: { token: string; product?: Product; isCreating: boolean; onSaved: (product: Product) => Promise<void>; onNotice: (message: string) => void; onError: (error: Error) => void }) {
  const queryClient = useQueryClient()
  const [name, setName] = useState(product?.name ?? '')
  const [description, setDescription] = useState(product?.description ?? '')
  const [detail, setDetail] = useState(product?.detail ?? '')
  const [sessionImages, setSessionImages] = useState(product?.images ?? [])
  const savedDraft = useMemo<ProductDraftInput>(() => ({ name: product?.name ?? '', description: product?.description ?? '', detail: product?.detail ?? '' }), [product])
  const currentDraft = useMemo<ProductDraftInput>(() => ({ name, description, detail }), [name, description, detail])
  const dirty = currentDraft.name !== savedDraft.name || currentDraft.description !== savedDraft.description || currentDraft.detail !== savedDraft.detail
  const imageURLs = usePrivateProductImageURLs(token, product?.id ?? '', sessionImages)
  const pasteImageMutation = useMutation({
    mutationFn: (file: File) => uploadProductImage(token, product!.id, file, file.name.replace(/\.[^.]+$/, '') || '粘贴图片'),
    onSuccess: (image) => {
      setSessionImages((current) => [...current, image])
      setDetail((current) => `${current.trimEnd()}\n\n![${image.alt_text || '商品效果图'}](${image.url})\n`)
      void queryClient.invalidateQueries({ queryKey: ['product-images', token, product?.id] })
      onNotice('剪贴板图片已上传，并以 Markdown 图片语法插入详细介绍。')
    },
    onError,
  })
  const productImageKey = product?.images?.map((image) => image.id).join(',') ?? ''
  useEffect(() => { setSessionImages(product?.images ?? []) }, [product?.id, productImageKey])

  const mutation = useMutation({
    mutationFn: async (targetStatus?: Product['status']) => {
      let item = product
        ? await updateProduct(token, product.id, currentDraft)
        : await createProduct(token, currentDraft.name, currentDraft.description, currentDraft.detail, targetStatus === 'ACTIVE' ? 'ACTIVE' : 'DRAFT')
      if (product && targetStatus && item.status !== targetStatus) item = await updateProductStatus(token, item.id, targetStatus)
      return item
    },
    onSuccess: async (item, targetStatus) => {
      await onSaved(item)
      if (targetStatus === 'ACTIVE') onNotice(`「${item.name}」已发布，公开目录现在可以看到。`)
      else if (targetStatus === 'DRAFT') onNotice(`「${item.name}」已转为草稿，公开目录已隐藏。`)
      else if (targetStatus === 'ARCHIVED') onNotice(`「${item.name}」已归档。`)
      else onNotice(isCreating ? `「${item.name}」已保存为草稿。` : '商品内容已保存。')
    },
    onError,
  })

  const previewProduct: Product = {
    id: product?.id ?? 'draft',
    name,
    description,
    detail,
    status: product?.status ?? 'DRAFT',
    asset_count: product?.asset_count ?? 0,
    images: sessionImages,
  }
  const canSave = name.trim().length > 0 && (isCreating || dirty)
  const busy = mutation.isPending

  function archiveProduct() {
    if (product && window.confirm(`归档商品「${product.name}」？归档后将停止公开展示，并暂停新增素材与兑换码。`)) mutation.mutate('ARCHIVED')
  }

  function pasteDetailImage(event: ClipboardEvent<HTMLTextAreaElement>) {
    const item = Array.from(event.clipboardData.items).find((entry) => entry.kind === 'file' && entry.type.startsWith('image/'))
    const file = item?.getAsFile()
    if (!file) return
    event.preventDefault()
    if (!product) {
      onNotice('先保存商品草稿，再把剪贴板图片粘贴到详细介绍中。')
      return
    }
    pasteImageMutation.mutate(new File([file], `clipboard-${Date.now()}.${file.type.split('/')[1] || 'png'}`, { type: file.type }))
  }

  if (!product && !isCreating) {
    return <div className="product-studio-empty"><Eye size={22} /><strong>选择一个商品开始编辑</strong><span>也可以新建草稿，并在发布前检查最终详情页。</span></div>
  }

  return (
    <div className="product-editor-layout">
      <form className="product-editor-form stack-form" onSubmit={(event) => { event.preventDefault(); if (canSave) mutation.mutate(undefined) }}>
        <div className="editor-state-row">
          <span className={`status-pill status-pill--${previewProduct.status.toLowerCase()}`}>{productStatusLabel(previewProduct.status)}</span>
          <span className={dirty || isCreating ? 'editor-save-state is-dirty' : 'editor-save-state'}>{dirty || isCreating ? '有未保存内容' : '内容已保存'}</span>
        </div>
        {product && <div className="product-ownership"><span>商品归属</span><strong>{product.tenant_name || '当前空间'}</strong><small>{product.owner_name ? `负责人：${product.owner_name}` : '负责人信息待补充'}</small></div>}
        <label htmlFor="editor-product-name">商品名称</label>
        <input id="editor-product-name" value={name} onChange={(event) => setName(event.target.value)} maxLength={120} required placeholder="例如：NOVER Starter Kit" />
        <div className="editor-field-meta"><label htmlFor="editor-product-description">商品简介</label><span>{description.length} / 2000</span></div>
        <textarea id="editor-product-description" value={description} onChange={(event) => setDescription(event.target.value)} maxLength={2000} rows={4} placeholder="用一到两句话说明内容与适用对象" />
        <div className="editor-field-meta"><label htmlFor="editor-product-detail">详细介绍 · Markdown</label><span>{detail.length} / 6000</span></div>
        <textarea id="editor-product-detail" value={detail} onChange={(event) => setDetail(event.target.value)} onPaste={pasteDetailImage} maxLength={6000} rows={10} placeholder={'支持 Markdown：标题、列表、链接、代码块。\n\n保存草稿后，可直接把剪贴板图片粘贴到这里。'} />
        <p className="field-note">右侧实时渲染 Markdown。粘贴的图片会上传为当前商品效果图，并自动插入图片语法。</p>
        <div className="editor-actions">
          <button className="button button-secondary" type="submit" disabled={!canSave || busy}><Save size={15} />{isCreating ? '保存草稿' : '保存修改'}</button>
          {!isCreating && product?.status === 'ACTIVE' && <button className="button button-secondary" type="button" disabled={busy} onClick={() => mutation.mutate('DRAFT')}><RotateCcw size={15} />下架为草稿</button>}
          {product && product.status !== 'ARCHIVED' && <button className="text-icon-button editor-archive" type="button" disabled={busy} onClick={archiveProduct}><Archive size={14} />归档</button>}
        </div>
      </form>
      <div className="product-editor-preview">
        <div className="preview-toolbar"><div><Eye size={14} /><strong>买家页面预览</strong></div><span>实时同步 · 响应式</span></div>
        <div className="preview-viewport"><ProductDetailView product={previewProduct} preview imageURLs={imageURLs} /></div>
      </div>
    </div>
  )
}

function usePrivateProductImageURLs(token: string, productId: string, images: ProductImage[]) {
  const [urls, setURLs] = useState<Record<string, string>>({})
  const imageKey = images.map((image) => image.id).join(',')
  useEffect(() => {
    let active = true
    const createdURLs: string[] = []
    setURLs({})
    if (!productId || !imageKey) return () => undefined
    void Promise.all(images.map(async (image) => {
      const blob = await loadProductImageBlob(token, productId, image.id)
      const url = URL.createObjectURL(blob)
      createdURLs.push(url)
      return [image.id, url] as const
    })).then((entries) => { if (active) setURLs(Object.fromEntries(entries)) }).catch(() => { if (active) setURLs({}) })
    return () => {
      active = false
      createdURLs.forEach((url) => URL.revokeObjectURL(url))
    }
  }, [token, productId, imageKey]) // Image IDs are the stable cache identity for the saved gallery.
  return urls
}

function productStatusLabel(status: Product['status']) {
  return { DRAFT: '草稿', ACTIVE: '已发布', ARCHIVED: '已归档' }[status]
}

function roleLabel(role?: OperatorSession['role']) {
  if (!role) return '未分配角色'
  return { OWNER: '空间所有者', ADMIN: '管理员', EDITOR: '内容编辑', FULFILLMENT: '交付运营', ANALYST: '数据分析' }[role]
}

function applicationStatusLabel(status?: OperatorSession['application_status']) {
  return { PENDING: '审核中', REVIEWING: '复核中', APPROVED: '已通过', REJECTED: '未通过' }[status ?? 'PENDING']
}

function platformRoleLabel(role: OperatorSession['platform_role']) {
  if (!role) return '租户成员'
  return { PLATFORM_OWNER: '平台所有者', PLATFORM_SUPPORT: '平台支持', PLATFORM_AUDITOR: '平台审计' }[role]
}

function AssetUpload({ token, productId, enabled, onUploaded, onError }: { token: string; productId: string; enabled: boolean; onUploaded: () => Promise<void> | void; onError: (error: Error) => void }) {
  const [file, setFile] = useState<File | null>(null)
  const mutation = useMutation({ mutationFn: () => uploadAsset(token, productId, file!), onSuccess: async () => { setFile(null); await onUploaded() }, onError })
  return (
    <form className="stack-form compact-form" onSubmit={(event) => { event.preventDefault(); if (file) mutation.mutate() }}>
      <label htmlFor="asset-file">选择交付文件</label>
      <input id="asset-file" className="file-input" type="file" onChange={(event) => setFile(event.target.files?.[0] ?? null)} disabled={!productId || !enabled} />
      <p className="field-note">草稿阶段即可上传；文件保存于私有目录并记录 SHA-256，发布前不会出现在公开页面。</p>
      <button className="button button-secondary" type="submit" disabled={!file || !productId || !enabled || mutation.isPending}><FileUp size={16} />{mutation.isPending ? '上传中…' : '上传到当前商品'}</button>
    </form>
  )
}

function ProductPublishControl({ token, product, onPublished, onError }: { token: string; product?: Product; onPublished: () => Promise<void> | void; onError: (error: Error) => void }) {
  const mutation = useMutation({ mutationFn: () => updateProductStatus(token, product!.id, 'ACTIVE'), onSuccess: onPublished, onError })
  const hasContent = Boolean(product?.name.trim() && product.description.trim())
  const hasDelivery = (product?.asset_count ?? 0) > 0
  const published = product?.status === 'ACTIVE'
  return (
    <div className="publish-product-control">
      <div className="publish-checklist" aria-label="发布检查">
        <span className={hasContent ? 'is-ready' : ''}><Check size={15} />商品内容已保存</span>
        <span className={(product?.images?.length ?? 0) > 0 ? 'is-ready' : ''}><Check size={15} />效果图片 <small>建议</small></span>
        <span className={hasDelivery ? 'is-ready' : ''}><Check size={15} />交付文件 <small>必需</small></span>
      </div>
      <div className="publish-product-action">
        <div><strong>{published ? '商品已在公开目录展示' : '完成前三步后发布商品'}</strong><span>{!product ? '先创建并保存商品草稿。' : !hasDelivery ? '上传至少一个交付文件后才可发布。' : '发布后即可为该商品签发兑换码。'}</span></div>
        <button className="button button-primary" type="button" disabled={!product || !hasContent || !hasDelivery || published || mutation.isPending} onClick={() => mutation.mutate()}><Send size={16} />{mutation.isPending ? '发布中…' : published ? '已发布' : product?.status === 'ARCHIVED' ? '恢复并发布' : '完成并发布'}</button>
      </div>
    </div>
  )
}

function CodeIssuer({ token, productId, productName, productStatus, codes, onIssued, onError }: { token: string; productId: string; productName: string; productStatus?: Product['status']; codes: string[]; onIssued: (codes: string[]) => void; onError: (error: Error) => void }) {
  const [count, setCount] = useState(1)
  const [orderRef, setOrderRef] = useState('')
  const [copied, setCopied] = useState(false)
  const mutation = useMutation({
    mutationFn: () => issueCodes(token, productId, { count, code_ttl_days: 90, access_days: 365, max_downloads: 25, external_order_ref: orderRef }),
    onSuccess: ({ codes: nextCodes }) => onIssued(nextCodes),
    onError,
  })
  const active = productStatus === 'ACTIVE'
  async function copyAll() {
    await navigator.clipboard.writeText(codes.join('\n'))
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1600)
  }
  return (
    <div className="stack-form compact-form">
      <div className="code-product-binding"><span className="section-kicker">本批兑换码绑定商品</span><strong>{productName || '请先选择商品'}</strong><span>每个兑换码只创建该商品的领取权益</span></div>
      {productId && !active && <StatusMessage kind="info">先发布当前商品，再签发可交付的兑换码。</StatusMessage>}
      <label htmlFor="order-ref">外部订单编号 <span className="optional-label">可选</span></label>
      <input id="order-ref" value={orderRef} onChange={(event) => setOrderRef(event.target.value)} maxLength={200} placeholder="用于售后对账" />
      <label htmlFor="code-count">生成数量</label>
      <div className="number-field"><button type="button" aria-label="减少数量" onClick={() => setCount((value) => Math.max(1, value - 1))}>−</button><input id="code-count" type="number" min={1} max={100} value={count} onChange={(event) => setCount(Number(event.target.value))} /><button type="button" aria-label="增加数量" onClick={() => setCount((value) => Math.min(100, value + 1))}>+</button></div>
      <div className="policy-summary"><span>兑换有效期 <strong>90 天</strong></span><span>领取后可访问 <strong>365 天</strong></span><span>下载额度 <strong>25 次</strong></span></div>
      <button className="button button-primary" type="button" disabled={!productId || !active || mutation.isPending} onClick={() => mutation.mutate()}><KeyRound size={16} />{mutation.isPending ? '签发中…' : '签发兑换码'}</button>
      {codes.length > 0 && <div className="issued-codes"><div className="issued-codes-heading"><span>新签发凭证</span><button className="text-icon-button" onClick={() => void copyAll()}><Copy size={14} />{copied ? '已复制' : '复制全部'}</button></div><pre>{codes.join('\n')}</pre><div className="code-warning"><Check size={14} />兑换码仅在当前后台会话显示，请及时保存。</div></div>}
    </div>
  )
}

function ProductImageManager({ productId, productName, productStatus, token, onUpdated, onError }: { productId: string; productName: string; productStatus?: Product['status']; token: string; onUpdated: () => void; onError: (error: Error) => void }) {
  const [file, setFile] = useState<File | null>(null)
  const [altText, setAltText] = useState('')
  const queryClient = useQueryClient()
  const queryKey = ['product-images', token, productId]
  const images = useQuery({ queryKey, queryFn: () => loadProductImages(token, productId), enabled: Boolean(productId), retry: false })
  const uploadMutation = useMutation({
    mutationFn: () => uploadProductImage(token, productId, file!, altText),
    onSuccess: async () => { setFile(null); setAltText(''); await queryClient.invalidateQueries({ queryKey }); onUpdated() },
    onError,
  })
  const deleteMutation = useMutation({
    mutationFn: (imageId: string) => deleteProductImage(token, productId, imageId),
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey }); onUpdated() },
    onError,
  })
  const imageURLs = usePrivateProductImageURLs(token, productId, images.data?.items ?? [])
  const editable = Boolean(productId) && productStatus !== 'ARCHIVED'

  function removeImage(imageId: string, imageAlt: string) {
    if (window.confirm(`删除效果图「${imageAlt || productName}」？公开详情页也会立即移除。`)) deleteMutation.mutate(imageId)
  }

  return (
    <div className="stack-form compact-form">
      <div className="image-product-binding"><span>当前商品</span><strong>{productName || '请先选择商品'}</strong></div>
      <label htmlFor="product-preview-file">选择效果图片</label>
      <input id="product-preview-file" className="file-input" type="file" accept="image/jpeg,image/png" onChange={(event) => setFile(event.target.files?.[0] ?? null)} disabled={!editable} />
      <label htmlFor="product-preview-alt">图片说明 <span className="optional-label">可选</span></label>
      <input id="product-preview-alt" value={altText} onChange={(event) => setAltText(event.target.value)} maxLength={240} disabled={!editable} placeholder="例如：订单与下载页面预览" />
      <p className="field-note">支持 JPEG / PNG，单张最大 8 MB。草稿图仅在工作台预览，发布后才会公开展示。</p>
      <button className="button button-secondary" type="button" disabled={!editable || !file || uploadMutation.isPending} onClick={() => uploadMutation.mutate()}><ImagePlus size={16} />{uploadMutation.isPending ? '上传中…' : '添加效果图'}</button>
      {!productId ? <div className="empty-state">先从商品目录选择一个商品。</div> : images.isLoading ? <div className="empty-state">正在读取效果图…</div> : images.isError ? <StatusMessage>{images.error.message}</StatusMessage> : images.data?.items.length ? (
        <div className="managed-image-grid">{images.data.items.map((image) => <article className="managed-image" key={image.id}>{imageURLs[image.id] ? <img src={imageURLs[image.id]} alt={image.alt_text || productName} /> : <div className="managed-image-loading">读取中…</div>}<div><span>{image.alt_text || productName}</span><button className="icon-button icon-button--small icon-button--danger" type="button" aria-label="删除效果图" title="删除效果图" disabled={!editable || deleteMutation.isPending} onClick={() => removeImage(image.id, image.alt_text)}><Trash2 size={14} /></button></div></article>)}</div>
      ) : <div className="empty-state">此商品还没有效果图。</div>}
    </div>
  )
}
