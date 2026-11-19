import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pause, Plus, RotateCcw, ShieldCheck, UserCheck } from 'lucide-react'
import { StatusMessage } from '../../components/StatusMessage'
import { createTenantMember, loadTenantMembers, updateTenantMember } from '../../lib/api'
import { describeError } from '../../lib/errors'
import type { MemberCreateInput, MemberPatchInput, OperatorSession, TenantMember } from '../../types'
import { platformRoleLabel, roleLabel } from './labels'

const CREATABLE_ROLES: Array<Exclude<OperatorSession['role'], 'OWNER'>> = ['ADMIN', 'EDITOR', 'FULFILLMENT', 'ANALYST']

const CAPABILITY_ROWS: Array<[OperatorSession['role'], string]> = [
  ['OWNER', '空间、成员、商品、交付和兑换码的全部租户权限'],
  ['ADMIN', '日常运营与成员管理，不承担平台级配置职责'],
  ['EDITOR', '编辑商品内容与效果图，不直接发布或签发兑换码'],
  ['FULFILLMENT', '上传交付文件、签发和管理兑换凭证'],
  ['ANALYST', '只读查看商品与交付汇总'],
]

export function MembersAdminView({ token, session, canManage }: { token: string; session: OperatorSession; canManage: boolean }) {
  const queryClient = useQueryClient()
  const members = useQuery({ queryKey: ['tenant-members', token], queryFn: () => loadTenantMembers(token), retry: false })
  const [notice, setNotice] = useState('')
  const [formError, setFormError] = useState('')
  const [username, setUsername] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState<MemberCreateInput['role']>('EDITOR')

  async function reloadMembers() {
    await queryClient.invalidateQueries({ queryKey: ['tenant-members', token] })
  }

  const createMutation = useMutation({
    mutationFn: () => createTenantMember(token, { username, display_name: displayName, password, role }),
    onSuccess: async () => {
      setUsername('')
      setDisplayName('')
      setPassword('')
      setRole('EDITOR')
      setFormError('')
      setNotice('成员已创建，请将初始密码线下告知成员。')
      await reloadMembers()
    },
    onError: (error) => {
      setNotice('')
      setFormError(describeError(error))
    },
  })

  const updateMutation = useMutation({
    mutationFn: ({ membershipId, patch }: { membershipId: string; patch: MemberPatchInput }) => updateTenantMember(token, membershipId, patch),
    onSuccess: async (_member, variables) => {
      setFormError('')
      setNotice(variables.patch.status === 'SUSPENDED' ? '成员已停用，其登录会话将被清理。' : variables.patch.status === 'ACTIVE' ? '成员已恢复启用。' : '成员角色已更新，权限立即生效。')
      await reloadMembers()
    },
    onError: (error) => {
      setNotice('')
      setFormError(describeError(error))
    },
  })

  function submitMember(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!/^[a-z][a-z0-9_-]{3,31}$/.test(username.trim().toLowerCase())) {
      setNotice('')
      setFormError('用户名需以小写字母开头，4-32 位，只能包含小写字母、数字、下划线或连字符。')
      return
    }
    if (displayName.trim().length < 2) {
      setNotice('')
      setFormError('显示名至少 2 个字符，便于成员列表与审计记录识别。')
      return
    }
    if (password.length < 8) {
      setNotice('')
      setFormError('初始密码至少 8 位。')
      return
    }
    setFormError('')
    createMutation.mutate()
  }

  const items = members.data?.items ?? []
  const busy = createMutation.isPending || updateMutation.isPending

  function isLocked(member: TenantMember) {
    return member.role === 'OWNER' || member.membership_id === session.membership_id || member.user_id === session.user_id
  }

  return <section className="members-admin-view">
    <div className="overview-heading"><div><span className="section-kicker">TENANT ACCESS</span><h2>成员与权限</h2></div><span>{members.isLoading ? '正在同步成员' : `${items.length} 位成员`}</span></div>
    <div className="member-security-banner"><ShieldCheck size={19} /><div><strong>权限由后端会话强制校验</strong><span>当前账号：{session.username} · {platformRoleLabel(session.platform_role)}。隐藏按钮只优化界面，API 仍会独立检查每个权限点。{canManage ? ' 当前角色可创建成员并调整角色与状态。' : ' 当前角色只读查看成员；创建与变更需要 member.manage 权限。'}</span></div></div>
    {formError && <StatusMessage>{formError}</StatusMessage>}
    {notice && <StatusMessage kind="success">{notice}</StatusMessage>}

    {canManage && <section className="admin-section member-create-section">
      <div className="admin-section-heading"><div className="tool-icon tool-icon--blue"><UserCheck size={18} /></div><div><span className="section-kicker">INVITE MEMBER</span><h2>创建成员</h2></div></div>
      <form className="member-create-form" onSubmit={submitMember}>
        <div className="member-create-grid">
          <label>用户名<input value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="off" maxLength={32} placeholder="例如：ops_editor" required /></label>
          <label>显示名<input value={displayName} onChange={(event) => setDisplayName(event.target.value)} maxLength={80} placeholder="例如：内容编辑" required /></label>
          <label>初始密码<input type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="new-password" minLength={8} maxLength={200} placeholder="至少 8 位" required /></label>
          <label>角色<select value={role} onChange={(event) => setRole(event.target.value as MemberCreateInput['role'])}>{CREATABLE_ROLES.map((item) => <option value={item} key={item}>{roleLabel(item)}</option>)}</select></label>
        </div>
        <p className="field-note">OWNER 不能通过此表单创建。创建成功后请将初始密码线下告知成员，并提醒其首次登录后修改密码。</p>
        <button className="button button-primary" type="submit" disabled={createMutation.isPending}><Plus size={15} />{createMutation.isPending ? '创建中…' : '创建成员'}</button>
      </form>
    </section>}

    {members.isError && <StatusMessage>{describeError(members.error)}</StatusMessage>}
    <div className="members-layout">
      <div className="member-list-panel">
        <div className="member-list-head"><span>成员</span><span>角色</span><span>最近会话</span><span>状态</span><span>操作</span></div>
        {members.isLoading ? <div className="empty-state">正在读取成员…</div> : items.map((member) => {
          const locked = isLocked(member)
          return <article className="member-row" key={member.membership_id}>
            <div className="member-identity"><span className="application-avatar">{member.display_name.slice(0, 1).toUpperCase()}</span><div><strong>{member.display_name}</strong><span>@{member.username} · {platformRoleLabel(member.platform_role)}</span></div></div>
            <div className="member-role-cell">{locked ? <span className="role-badge">{roleLabel(member.role)}</span> : <select className="member-role-select" aria-label={`修改 ${member.display_name} 的角色`} value={member.role} disabled={!canManage || busy} onChange={(event) => updateMutation.mutate({ membershipId: member.membership_id, patch: { role: event.target.value as MemberPatchInput['role'] } })}>{CREATABLE_ROLES.map((item) => <option value={item} key={item}>{roleLabel(item)}</option>)}</select>}</div>
            <span className="member-last-active">{member.last_active_at ? `最近会话 ${new Date(member.last_active_at).toLocaleString('zh-CN')}` : '尚无会话'}</span>
            <span className={`status-pill member-status--${member.status.toLowerCase()}`}>{member.status === 'ACTIVE' ? '正常' : '已停用'}</span>
            <div className="member-row-actions">
              {locked ? <span className="member-locked-label">{member.role === 'OWNER' ? '所有者只读' : '当前账号'}</span> : canManage ? <button className="text-icon-button" type="button" disabled={busy || updateMutation.isPending} onClick={() => updateMutation.mutate({ membershipId: member.membership_id, patch: { status: member.status === 'ACTIVE' ? 'SUSPENDED' : 'ACTIVE' } })}>{member.status === 'ACTIVE' ? <><Pause size={14} />停用</> : <><RotateCcw size={14} />启用</>}</button> : <span className="member-locked-label">只读</span>}
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
