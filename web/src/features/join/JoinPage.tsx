import { useState, type FormEvent } from 'react'
import { ArrowRight, Building2, KeyRound, Send, ShieldCheck, UserPlus } from 'lucide-react'
import { useMutation } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { StatusMessage } from '../../components/StatusMessage'
import { operatorLogin, submitJoinApplication } from '../../lib/api'
import { OPERATOR_PATH } from '../../lib/routes'

type JoinMode = 'apply' | 'login'

export function JoinPage() {
  const navigate = useNavigate()
  const [mode, setMode] = useState<JoinMode>('apply')
  const [submitted, setSubmitted] = useState(false)
  const [application, setApplication] = useState({ organization_name: '', contact_name: '', email: '', desired_username: '', use_case: '' })
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const applyMutation = useMutation({ mutationFn: () => submitJoinApplication(application), onSuccess: () => setSubmitted(true) })
  const loginMutation = useMutation({
    mutationFn: () => operatorLogin(username, password),
    onSuccess: ({ access_token }) => {
      sessionStorage.setItem('nover.operatorToken', access_token)
      navigate(OPERATOR_PATH)
    },
  })

  function submitApplication(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    applyMutation.mutate()
  }

  function submitLogin(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    loginMutation.mutate()
  }

  return (
    <section className="join-page">
      <div className="join-intro">
        <div className="eyebrow"><span className="eyebrow-line" /> NOVER PARTNER ACCESS</div>
        <h1>建立你的<br /><span>数字交付空间。</span></h1>
        <p>提交团队与业务信息，由平台运营者完成准入审核。已有运营账号可直接进入控制台。</p>
        <div className="join-trust-list">
          <span><ShieldCheck size={17} />申请不会直接获得商品或订单权限</span>
          <span><Building2 size={17} />账号将按组织与角色分配访问范围</span>
        </div>
      </div>

      <div className="join-workspace surface-panel">
        <div className="join-mode-tabs" role="tablist" aria-label="加入方式">
          <button className={mode === 'apply' ? 'is-active' : ''} type="button" onClick={() => setMode('apply')}><UserPlus size={16} />申请加入</button>
          <button className={mode === 'login' ? 'is-active' : ''} type="button" onClick={() => setMode('login')}><KeyRound size={16} />账号登录</button>
        </div>

        {mode === 'apply' && (submitted ? <div className="join-success"><div className="tool-icon tool-icon--green"><Send size={19} /></div><span className="section-kicker">APPLICATION RECEIVED</span><h2>申请已进入审核队列</h2><p>运营人员会根据提交信息处理准入。账号开通时将通过独立邀请流程设置密码。</p><button className="button button-secondary" type="button" onClick={() => { setSubmitted(false); setApplication({ organization_name: '', contact_name: '', email: '', desired_username: '', use_case: '' }) }}>提交其他申请</button></div> : (
          <form className="stack-form join-form" onSubmit={submitApplication}>
            <div className="panel-heading"><div className="tool-icon"><UserPlus size={19} /></div><div><span className="section-kicker">PARTNER APPLICATION</span><h2>申请运营账号</h2></div></div>
            <div className="join-field-grid">
              <label>组织或团队名称<input value={application.organization_name} onChange={(event) => setApplication((current) => ({ ...current, organization_name: event.target.value }))} minLength={2} maxLength={120} required /></label>
              <label>联系人<input value={application.contact_name} onChange={(event) => setApplication((current) => ({ ...current, contact_name: event.target.value }))} minLength={2} maxLength={80} required /></label>
              <label>联系邮箱<input type="email" value={application.email} onChange={(event) => setApplication((current) => ({ ...current, email: event.target.value }))} maxLength={254} required /></label>
              <label>期望账号<input value={application.desired_username} onChange={(event) => setApplication((current) => ({ ...current, desired_username: event.target.value.toLowerCase() }))} pattern="[a-z][a-z0-9_\-]{3,31}" placeholder="team_operator" required /></label>
            </div>
            <label>计划交付的内容<textarea value={application.use_case} onChange={(event) => setApplication((current) => ({ ...current, use_case: event.target.value }))} minLength={10} maxLength={1200} rows={5} placeholder="产品类型、预计数量和团队使用方式" required /></label>
            {applyMutation.isError && <StatusMessage>{applyMutation.error.message}</StatusMessage>}
            <button className="button button-primary" type="submit" disabled={applyMutation.isPending}>{applyMutation.isPending ? '正在提交' : '提交申请'}<ArrowRight size={16} /></button>
          </form>
        ))}

        {mode === 'login' && <form className="stack-form join-form" onSubmit={submitLogin}>
          <div className="panel-heading"><div className="tool-icon"><KeyRound size={19} /></div><div><span className="section-kicker">OPERATOR SIGN IN</span><h2>进入运营控制台</h2></div></div>
          <label htmlFor="join-login-username">账号</label>
          <input id="join-login-username" autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} required />
          <label htmlFor="join-login-password">密码</label>
          <input id="join-login-password" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} required />
          {loginMutation.isError && <StatusMessage>{loginMutation.error.message}</StatusMessage>}
          <button className="button button-primary" type="submit" disabled={loginMutation.isPending}>{loginMutation.isPending ? '登录中' : '登录控制台'}<ArrowRight size={16} /></button>
        </form>}
      </div>
    </section>
  )
}
