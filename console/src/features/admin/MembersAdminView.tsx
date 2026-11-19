import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, KeyRound, Link2, Pause, Plus, RefreshCw, RotateCcw, ShieldCheck, UserCheck, UserPlus, X } from 'lucide-react'
import { StatusMessage } from '../../components/StatusMessage'
import { SkeletonRows } from '../../components/Skeleton'
import { useToast } from '../../components/Toast'
import {
  createMemberInvitation,
  createMemberPasswordReset,
  createTenantMember,
  loadMemberInvitations,
  loadTenantMembers,
  revokeMemberInvitation,
  updateTenantMember,
} from '../../lib/api'
import { copyText } from '../../lib/clipboard'
import { describeError } from '../../lib/errors'
import { OPERATOR_PATH } from '../../lib/routes'
import type { MemberCreateInput, MemberInvitation, MemberInvitationCreated, MemberPasswordResetCreated, MemberPatchInput, OperatorSession, TenantMember } from '../../types'
import { platformRoleLabel, roleLabel } from './labels'

const CREATABLE_ROLES: Array<Exclude<OperatorSession['role'], 'OWNER'>> = ['ADMIN', 'EDITOR', 'FULFILLMENT', 'ANALYST']

const CAPABILITY_ROWS: Array<[OperatorSession['role'], string]> = [
  ['OWNER', '空间、成员、商品、交付和兑换码的全部租户权限'],
  ['ADMIN', '日常运营与成员管理，不承担平台级配置职责'],
  ['EDITOR', '编辑商品内容与效果图，不直接发布或签发兑换码'],
  ['FULFILLMENT', '上传交付文件、签发和管理兑换凭证'],
  ['ANALYST', '只读查看商品与交付汇总'],
]

const INVITATION_STATUS_LABELS: Record<MemberInvitation['status'], string> = {
  PENDING: '待接受',
  USED: '已使用',
  EXPIRED: '已过期',
  REVOKED: '已撤销',
}

function acceptLink(token: string) {
  return `${window.location.origin}${OPERATOR_PATH}/accept?token=${encodeURIComponent(token)}`
}

function formatTime(value: string) {
  if (!value) return '未知'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN')
}

function memberStatusLabel(status: TenantMember['status']) {
  return status === 'ACTIVE' ? '正常' : status === 'PENDING' ? '待激活' : '已停用'
}

export function MembersAdminView({ token, session, canManage }: { token: string; session: OperatorSession; canManage: boolean }) {
  const toast = useToast()
  const queryClient = useQueryClient()
  const members = useQuery({ queryKey: ['tenant-members', token], queryFn: () => loadTenantMembers(token), retry: false })
  const invitations = useQuery({ queryKey: ['member-invitations', token], queryFn: () => loadMemberInvitations(token), enabled: canManage, retry: false })
  const [inviteUsername, setInviteUsername] = useState('')
  const [inviteDisplayName, setInviteDisplayName] = useState('')
  const [inviteRole, setInviteRole] = useState<MemberCreateInput['role']>('EDITOR')
  const [inviteError, setInviteError] = useState('')
  const [inviteResult, setInviteResult] = useState<MemberInvitationCreated | null>(null)
  const [resetResult, setResetResult] = useState<MemberPasswordResetCreated | null>(null)
  const [directOpen, setDirectOpen] = useState(false)
  const [username, setUsername] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState<MemberCreateInput['role']>('EDITOR')
  const [createError, setCreateError] = useState('')

  async function reloadMembers() {
    await queryClient.invalidateQueries({ queryKey: ['tenant-members', token] })
  }

  async function reloadInvitations() {
    await queryClient.invalidateQueries({ queryKey: ['member-invitations', token] })
  }

  const inviteMutation = useMutation({
    mutationFn: () => createMemberInvitation(token, { username: inviteUsername, display_name: inviteDisplayName, role: inviteRole }),
    onSuccess: async (invitation) => {
      setInviteUsername('')
      setInviteDisplayName('')
      setInviteRole('EDITOR')
      setInviteError('')
      setInviteResult(invitation)
      toast.success(`已为 ${invitation.username} 生成一次性邀请链接。`)
      await Promise.all([reloadInvitations(), reloadMembers()])
    },
    onError: (error) => toast.error(describeError(error)),
  })

  const revokeInvitationMutation = useMutation({
    mutationFn: (invitationId: string) => revokeMemberInvitation(token, invitationId),
    onSuccess: async () => {
      toast.success('待处理邀请已撤销。')
      await reloadInvitations()
    },
    onError: (error) => toast.error(describeError(error)),
  })

  const resetMutation = useMutation({
    mutationFn: (membershipId: string) => createMemberPasswordReset(token, membershipId),
    onSuccess: async (reset) => {
      setResetResult(reset)
      toast.success(`已为 ${reset.username} 生成一次性密码重置链接。`)
      await reloadInvitations()
    },
    onError: (error) => toast.error(describeError(error)),
  })

  const createMutation = useMutation({
    mutationFn: () => createTenantMember(token, { username, display_name: displayName, password, role }),
    onSuccess: async () => {
      setUsername('')
      setDisplayName('')
      setPassword('')
      setRole('EDITOR')
      setCreateError('')
      toast.success('成员已创建，请将初始密码线下告知成员。')
      await reloadMembers()
    },
    onError: (error) => toast.error(describeError(error)),
  })

  const updateMutation = useMutation({
    mutationFn: ({ membershipId, patch }: { membershipId: string; patch: MemberPatchInput }) => updateTenantMember(token, membershipId, patch),
    onSuccess: async (_member, variables) => {
      toast.success(variables.patch.status === 'SUSPENDED' ? '成员已停用，其登录会话将被清理。' : variables.patch.status === 'ACTIVE' ? '成员已恢复启用。' : '成员角色已更新，权限立即生效。')
      await reloadMembers()
    },
    onError: (error) => toast.error(describeError(error)),
  })

  function submitInvitation(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!/^[a-z][a-z0-9_-]{3,31}$/.test(inviteUsername.trim().toLowerCase())) {
      setInviteError('用户名需以小写字母开头，4-32 位，只能包含小写字母、数字、下划线或连字符。')
      return
    }
    if (inviteDisplayName.trim().length < 2) {
      setInviteError('显示名至少 2 个字符，便于成员列表与审计记录识别。')
      return
    }
    setInviteError('')
    inviteMutation.mutate()
  }

  function submitMember(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!/^[a-z][a-z0-9_-]{3,31}$/.test(username.trim().toLowerCase())) {
      setCreateError('用户名需以小写字母开头，4-32 位，只能包含小写字母、数字、下划线或连字符。')
      return
    }
    if (displayName.trim().length < 2) {
      setCreateError('显示名至少 2 个字符，便于成员列表与审计记录识别。')
      return
    }
    if (password.length < 8) {
      setCreateError('初始密码至少 8 位。')
      return
    }
    setCreateError('')
    createMutation.mutate()
  }

  const items = members.data?.items ?? []
  const pendingInvitations = (invitations.data?.items ?? []).filter((invitation) => invitation.status === 'PENDING')
  const busy = createMutation.isPending || updateMutation.isPending || inviteMutation.isPending || resetMutation.isPending || revokeInvitationMutation.isPending

  function isLocked(member: TenantMember) {
    return member.role === 'OWNER' || member.membership_id === session.membership_id || member.user_id === session.user_id
  }

  function askRevokeInvitation(invitation: MemberInvitation) {
    if (window.confirm(`撤销发给 ${invitation.display_name}（@${invitation.username}）的待处理邀请？链接将立即失效。`)) revokeInvitationMutation.mutate(invitation.id)
  }

  return <section className="members-admin-view">
    <div className="overview-heading"><div><span className="section-kicker">TENANT ACCESS</span><h2>成员与权限</h2></div><span>{members.isLoading ? '正在同步成员' : `${items.length} 位成员`}</span></div>
    <div className="member-security-banner"><ShieldCheck size={19} /><div><strong>权限由后端会话强制校验</strong><span>当前账号：{session.username} · {platformRoleLabel(session.platform_role)}。隐藏按钮只优化界面，API 仍会独立检查每个权限点。{canManage ? ' 邀请链接与密码重置链接均为一次性凭证，只在生成时显示。' : ' 当前角色只读查看成员；邀请与变更需要 member.manage 权限。'}</span></div></div>

    {inviteResult && <SecretLinkPanel title="一次性邀请链接" kindLabel="邀请" username={inviteResult.username} link={acceptLink(inviteResult.token)} expiresAt={inviteResult.expires_at} note="成员接受前请通过安全渠道发送；接受后账号立即激活，链接随之失效。" onDismiss={() => setInviteResult(null)} />}
    {resetResult && <SecretLinkPanel title="一次性密码重置链接" kindLabel="密码重置" username={resetResult.username} link={acceptLink(resetResult.token)} expiresAt={resetResult.expires_at} note="成员用它设置新密码后，该账号在其他设备上的会话会被下线。" onDismiss={() => setResetResult(null)} />}

    {canManage && <section className="admin-section member-invite-section">
      <div className="admin-section-heading"><div className="tool-icon tool-icon--blue"><UserPlus size={18} /></div><div><span className="section-kicker">INVITE MEMBER</span><h2>邀请成员</h2></div><span className="member-invite-badge">推荐 · 一次性链接</span></div>
      <form className="member-create-form" onSubmit={submitInvitation}>
        <div className="member-invite-grid">
          <label>用户名<input value={inviteUsername} onChange={(event) => setInviteUsername(event.target.value)} autoComplete="off" maxLength={32} placeholder="例如：ops_editor" required /></label>
          <label>显示名<input value={inviteDisplayName} onChange={(event) => setInviteDisplayName(event.target.value)} maxLength={80} placeholder="例如：内容编辑" required /></label>
          <label>角色<select value={inviteRole} onChange={(event) => setInviteRole(event.target.value as MemberCreateInput['role'])}>{CREATABLE_ROLES.map((item) => <option value={item} key={item}>{roleLabel(item)}</option>)}</select></label>
        </div>
        <p className="field-note">生成一次性链接发给成员，由其自行设置密码。链接 72 小时内有效，且只能接受一次；OWNER 不能通过此表单邀请。</p>
        {inviteError && <StatusMessage>{inviteError}</StatusMessage>}
        <button className="button button-primary" type="submit" disabled={inviteMutation.isPending}><Link2 size={15} />{inviteMutation.isPending ? '正在生成…' : '生成邀请链接'}</button>
      </form>
    </section>}

    {canManage && <section className="admin-section member-pending-section">
      <div className="admin-section-heading"><div className="tool-icon tool-icon--amber"><Link2 size={18} /></div><div><span className="section-kicker">PENDING INVITATIONS</span><h2>待处理邀请</h2></div><button className="text-icon-button account-refresh" title="刷新待处理邀请" onClick={() => void invitations.refetch()} disabled={invitations.isFetching}><RefreshCw size={14} />刷新</button></div>
      {invitations.isError && <StatusMessage>{describeError(invitations.error)}</StatusMessage>}
      {invitations.isLoading ? <SkeletonRows rows={2} columns={4} label="正在读取待处理邀请" /> : pendingInvitations.length ? <div className="invitation-list">
        {pendingInvitations.map((invitation) => <article className="invitation-row" key={invitation.id}>
          <div className="member-identity"><span className="application-avatar">{invitation.display_name.slice(0, 1).toUpperCase()}</span><div><strong>{invitation.display_name}</strong><span>@{invitation.username} · {roleLabel(invitation.role)} · {invitation.kind === 'PASSWORD_RESET' ? '密码重置' : '成员邀请'}</span></div></div>
          <span className={`invitation-status invitation-status--${invitation.status.toLowerCase()}`}>{INVITATION_STATUS_LABELS[invitation.status]}</span>
          <span className="invitation-expiry">有效至 {formatTime(invitation.expires_at)}</span>
          <button className="text-icon-button" type="button" disabled={busy} onClick={() => askRevokeInvitation(invitation)}><X size={14} />撤销</button>
        </article>)}
      </div> : <p className="field-note member-empty-note">当前没有待处理邀请。生成邀请后，链接的接受状态会显示在这里。</p>}
    </section>}

    {canManage && <div className="member-create-secondary">
      <button className="text-icon-button" type="button" aria-expanded={directOpen} onClick={() => setDirectOpen((current) => !current)}>{directOpen ? <X size={14} /> : <Plus size={14} />}{directOpen ? '收起直接创建成员' : '直接创建成员（设置初始密码）'}</button>
      {directOpen && <section className="admin-section member-create-section">
        <div className="admin-section-heading"><div className="tool-icon tool-icon--blue"><UserCheck size={18} /></div><div><span className="section-kicker">CREATE WITH PASSWORD</span><h2>直接创建成员</h2></div></div>
        <form className="member-create-form" onSubmit={submitMember}>
          <div className="member-create-grid">
            <label>用户名<input value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="off" maxLength={32} placeholder="例如：ops_editor" required /></label>
            <label>显示名<input value={displayName} onChange={(event) => setDisplayName(event.target.value)} maxLength={80} placeholder="例如：内容编辑" required /></label>
            <label>初始密码<input type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="new-password" minLength={8} maxLength={200} placeholder="至少 8 位" required /></label>
            <label>角色<select value={role} onChange={(event) => setRole(event.target.value as MemberCreateInput['role'])}>{CREATABLE_ROLES.map((item) => <option value={item} key={item}>{roleLabel(item)}</option>)}</select></label>
          </div>
          <p className="field-note">OWNER 不能通过此表单创建。创建成功后请将初始密码线下告知成员，并提醒其首次登录后修改密码；邀请链接是更安全的默认方式。</p>
          {createError && <StatusMessage>{createError}</StatusMessage>}
          <button className="button button-secondary" type="submit" disabled={createMutation.isPending}><Plus size={15} />{createMutation.isPending ? '创建中…' : '直接创建成员'}</button>
        </form>
      </section>}
    </div>}

    {members.isError && <StatusMessage>{describeError(members.error)}</StatusMessage>}
    <div className="members-layout">
      <div className="member-list-panel">
        <div className="member-list-head"><span>成员</span><span>角色</span><span>最近会话</span><span>状态</span><span>操作</span></div>
        {members.isLoading ? <SkeletonRows rows={4} columns={5} label="正在读取成员" /> : items.map((member) => {
          const locked = isLocked(member)
          return <article className="member-row" key={member.membership_id}>
            <div className="member-identity"><span className="application-avatar">{member.display_name.slice(0, 1).toUpperCase()}</span><div><strong>{member.display_name}</strong><span>@{member.username} · {platformRoleLabel(member.platform_role)}</span></div></div>
            <div className="member-role-cell">{locked ? <span className="role-badge">{roleLabel(member.role)}</span> : <select className="member-role-select" aria-label={`修改 ${member.display_name} 的角色`} value={member.role} disabled={!canManage || busy} onChange={(event) => updateMutation.mutate({ membershipId: member.membership_id, patch: { role: event.target.value as MemberPatchInput['role'] } })}>{CREATABLE_ROLES.map((item) => <option value={item} key={item}>{roleLabel(item)}</option>)}</select>}</div>
            <span className="member-last-active">{member.last_active_at ? `最近会话 ${formatTime(member.last_active_at)}` : '尚无会话'}</span>
            <span className={`status-pill member-status--${member.status.toLowerCase()}`}>{memberStatusLabel(member.status)}</span>
            <div className="member-row-actions">
              {locked ? <span className="member-locked-label">{member.role === 'OWNER' ? '所有者只读' : '当前账号'}</span> : member.status === 'PENDING' ? <span className="member-locked-label">待成员接受邀请</span> : canManage ? <>
                <button className="text-icon-button" type="button" disabled={busy} title="生成一次性密码重置链接" onClick={() => resetMutation.mutate(member.membership_id)}><KeyRound size={14} />重置密码</button>
                <button className="text-icon-button" type="button" disabled={busy} onClick={() => updateMutation.mutate({ membershipId: member.membership_id, patch: { status: member.status === 'ACTIVE' ? 'SUSPENDED' : 'ACTIVE' } })}>{member.status === 'ACTIVE' ? <><Pause size={14} />停用</> : <><RotateCcw size={14} />启用</>}</button>
              </> : <span className="member-locked-label">只读</span>}
            </div>
          </article>
        })}
        {!members.isLoading && !items.length && <div className="empty-state">当前空间还没有成员。</div>}
      </div>
      <aside className="role-capability-panel">
        <span className="section-kicker">ROLE MATRIX</span><h3>租户角色边界</h3>
        <div className="role-capability-list">{CAPABILITY_ROWS.map(([capabilityRole, description]) => <div key={capabilityRole}><strong>{roleLabel(capabilityRole)}</strong><span>{description}</span></div>)}</div>
        <p>平台角色与租户角色相互独立。申请审核和部署设置属于平台权限，不随租户管理员角色自动获得。</p>
      </aside>
    </div>
  </section>
}

function SecretLinkPanel({ title, kindLabel, username, link, expiresAt, note, onDismiss }: { title: string; kindLabel: string; username: string; link: string; expiresAt: string; note: string; onDismiss: () => void }) {
  const toast = useToast()
  const [copied, setCopied] = useState(false)

  async function copyLink() {
    const ok = await copyText(link)
    if (!ok) {
      toast.error('复制失败，请手动选择链接文本复制。')
      return
    }
    setCopied(true)
    toast.success(`${kindLabel}链接已复制。`)
    window.setTimeout(() => setCopied(false), 1800)
  }

  return <section className="admin-section secret-link-panel">
    <div className="admin-section-heading"><div className="tool-icon tool-icon--amber"><Link2 size={18} /></div><div><span className="section-kicker">ONE-TIME LINK</span><h2>{title}</h2></div><button className="icon-button icon-button--small" type="button" title="关闭链接面板" aria-label="关闭链接面板" onClick={onDismiss}><X size={15} aria-hidden="true" /></button></div>
    <p className="field-note">为 <strong>{username}</strong> 生成的{kindLabel}链接，有效期至 {formatTime(expiresAt)}。{note} 面板关闭后无法再次查看。</p>
    <div className="secret-link-row">
      <code>{link}</code>
      <button className="button button-secondary button-small" type="button" onClick={() => void copyLink()}><Copy size={14} />{copied ? '已复制' : '复制链接'}</button>
    </div>
  </section>
}
