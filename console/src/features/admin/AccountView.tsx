import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, LockKeyhole, MonitorSmartphone, RefreshCw, ShieldCheck, Smartphone } from 'lucide-react'
import { StatusMessage } from '../../components/StatusMessage'
import { useToast } from '../../components/Toast'
import { changeOperatorPassword, loadOperatorSessions, revokeOperatorSessionById, revokeOtherOperatorSessions } from '../../lib/api'
import { describeError } from '../../lib/errors'
import { MfaCard } from './MfaCard'

export function AccountView({ token }: { token: string }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const sessions = useQuery({ queryKey: ['operator-sessions', token], queryFn: () => loadOperatorSessions(token), retry: false })
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [formError, setFormError] = useState('')

  async function reloadSessions() {
    await queryClient.invalidateQueries({ queryKey: ['operator-sessions', token] })
  }

  const passwordMutation = useMutation({
    mutationFn: () => changeOperatorPassword(token, currentPassword, newPassword),
    onSuccess: async () => {
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
      setFormError('')
      toast.success('密码已更新，其他设备已下线。')
      await reloadSessions()
    },
    onError: (error) => {
      setFormError('')
      toast.error(describeError(error))
    },
  })

  const revokeMutation = useMutation({
    mutationFn: (sessionId: string) => revokeOperatorSessionById(token, sessionId),
    onSuccess: async () => {
      setFormError('')
      toast.success('该设备的登录会话已撤销。')
      await reloadSessions()
    },
    onError: (error) => {
      setFormError('')
      toast.error(describeError(error))
    },
  })

  const revokeOthersMutation = useMutation({
    mutationFn: () => revokeOtherOperatorSessions(token),
    onSuccess: async ({ revoked }) => {
      setFormError('')
      toast.success(`已撤销 ${revoked} 个其他设备会话。`)
      await reloadSessions()
    },
    onError: (error) => {
      setFormError('')
      toast.error(describeError(error))
    },
  })

  function submitPassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (newPassword.length < 8) {
      setFormError('新密码至少 8 位。')
      return
    }
    if (newPassword !== confirmPassword) {
      setFormError('两次输入的新密码不一致。')
      return
    }
    if (newPassword === currentPassword) {
      setFormError('新密码不能与当前密码相同。')
      return
    }
    setFormError('')
    passwordMutation.mutate()
  }

  const items = sessions.data?.items ?? []
  const currentSessionId = sessions.data?.current_session_id
  const isCurrentSession = (sessionId: string, current: boolean) => current || (currentSessionId ? sessionId === currentSessionId : false)
  const otherSessions = items.filter((item) => !isCurrentSession(item.id, item.current))

  return <section className="account-view">
    <div className="overview-heading"><div><span className="section-kicker">ACCOUNT SECURITY</span><h2>账户与安全</h2></div><span>{sessions.isLoading ? '正在同步会话' : `${items.length} 个活跃会话`}</span></div>
    <div className="member-security-banner"><ShieldCheck size={19} /><div><strong>修改密码会立即下线其他设备</strong><span>会话列表展示当前账号在各设备上的登录记录。撤销会话会立即让其访问令牌失效；密码更新后所有其他设备的会话会被清理。建议同时启用两步验证。</span></div></div>
    {formError && <StatusMessage>{formError}</StatusMessage>}
    <div className="account-grid">
      <section className="admin-section account-card">
        <div className="admin-section-heading"><div className="tool-icon tool-icon--blue"><KeyRound size={18} /></div><div><span className="section-kicker">PASSWORD</span><h2>修改登录密码</h2></div></div>
        <form className="stack-form compact-form" onSubmit={submitPassword}>
          <label htmlFor="account-current-password">当前密码</label>
          <input id="account-current-password" type="password" autoComplete="current-password" value={currentPassword} onChange={(event) => setCurrentPassword(event.target.value)} maxLength={200} required />
          <label htmlFor="account-new-password">新密码</label>
          <input id="account-new-password" type="password" autoComplete="new-password" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} minLength={8} maxLength={200} required />
          <label htmlFor="account-confirm-password">确认新密码</label>
          <input id="account-confirm-password" type="password" autoComplete="new-password" value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} minLength={8} maxLength={200} required />
          <p className="field-note">新密码至少 8 位，建议混合大小写、数字与符号。修改成功后其他设备会立即退出登录。</p>
          <button className="button button-primary" type="submit" disabled={passwordMutation.isPending}><LockKeyhole size={15} />{passwordMutation.isPending ? '更新中…' : '更新密码'}</button>
        </form>
      </section>

      <section className="admin-section account-card">
        <div className="admin-section-heading"><div className="tool-icon tool-icon--amber"><MonitorSmartphone size={18} /></div><div><span className="section-kicker">DEVICE SESSIONS</span><h2>登录设备与会话</h2></div><button className="text-icon-button account-refresh" title="刷新会话" onClick={() => void sessions.refetch()}><RefreshCw size={14} />刷新</button></div>
        {sessions.isError && <StatusMessage>{describeError(sessions.error)}</StatusMessage>}
        {sessions.isLoading ? <div className="empty-state">正在读取会话…</div> : items.length ? <div className="session-list">{items.map((item) => {
          const current = isCurrentSession(item.id, item.current)
          return <article className="session-row" key={item.id}>
            <span className="session-device"><Smartphone size={16} aria-hidden="true" /></span>
            <div className="session-meta"><strong>{item.tenant_name || '当前空间'}{current && <span className="session-current-badge">当前设备</span>}</strong><span>登录于 {formatTime(item.created_at)} · 最近活动 {formatTime(item.last_seen_at)}</span><span>有效期至 {formatTime(item.expires_at)}</span></div>
            <div className="session-actions">{current ? <span className="session-locked-label">当前会话</span> : <button className="button button-secondary button-small" type="button" disabled={revokeMutation.isPending} onClick={() => { if (window.confirm('撤销该设备的登录会话？该设备需要重新登录。')) revokeMutation.mutate(item.id) }}>撤销</button>}</div>
          </article>
        })}</div> : <div className="empty-state">没有读取到活跃会话。</div>}
        <div className="session-footer">
          <button className="button button-secondary" type="button" disabled={!otherSessions.length || revokeOthersMutation.isPending} onClick={() => { if (window.confirm('撤销除当前设备外的所有会话？其他设备需要重新登录。')) revokeOthersMutation.mutate() }}><MonitorSmartphone size={15} />{revokeOthersMutation.isPending ? '撤销中…' : `撤销其他设备（${otherSessions.length}）`}</button>
        </div>
      </section>
    </div>
    <MfaCard token={token} />
  </section>
}

function formatTime(value: string) {
  if (!value) return '未知'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN')
}
