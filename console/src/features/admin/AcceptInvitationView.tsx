import { useEffect, useState, type FormEvent } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useMutation } from '@tanstack/react-query'
import { CheckCircle2, KeyRound, LockKeyhole, ShieldAlert } from 'lucide-react'
import { StatusMessage } from '../../components/StatusMessage'
import { ApiError, acceptInvitation } from '../../lib/api'
import { describeError } from '../../lib/errors'
import { OPERATOR_PATH } from '../../lib/routes'

const ACCEPT_ERROR_MESSAGES: Record<string, string> = {
  invalid_invitation: '邀请链接无效或已被替换，请向管理员索取新的链接。',
  invalid_password: '密码不符合要求：长度需为 8–200 个字符。',
  invitation_expired: '邀请链接已过期或已被撤销，请联系管理员重新生成。',
  invitation_used: '邀请链接已被使用。如果这是密码重置链接，请让管理员重新生成。',
  rate_limited: '尝试次数过多，请稍后重试。',
}

function acceptErrorMessage(error: unknown) {
  if (error instanceof ApiError && ACCEPT_ERROR_MESSAGES[error.code]) return ACCEPT_ERROR_MESSAGES[error.code]
  return describeError(error)
}

export function AcceptInvitationPage() {
  const location = useLocation()
  const token = new URLSearchParams(location.search).get('token')?.trim() ?? ''
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [formError, setFormError] = useState('')
  const mutation = useMutation({ mutationFn: () => acceptInvitation(token, password) })

  useEffect(() => {
    document.title = '设置登录密码 · 知屿 NOVER'
  }, [])

  function submitPassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (password.trim().length < 8) {
      setFormError('密码至少需要 8 个字符。')
      return
    }
    if (password !== confirmPassword) {
      setFormError('两次输入的密码不一致，请重新输入。')
      return
    }
    setFormError('')
    mutation.mutate()
  }

  return <section className="accept-page">
    <div className="accept-card surface-panel">
      <div className="tool-icon tool-icon--blue"><KeyRound size={19} aria-hidden="true" /></div>
      <span className="section-kicker">SECURE INVITATION</span>
      <h1 className="page-title">设置登录密码</h1>

      {!token ? <>
        <p className="accept-lede">这个链接缺少邀请凭证，无法完成设置。</p>
        <StatusMessage kind="error">邀请链接缺少 token 参数，请复制完整链接，或向管理员索取新的邀请。</StatusMessage>
        <Link className="button button-secondary accept-back-link" to={OPERATOR_PATH}>返回运营控制台登录</Link>
      </> : mutation.isSuccess ? <>
        <div className="accept-success" role="status" aria-live="polite">
          <CheckCircle2 size={26} aria-hidden="true" />
          <strong>密码已设置</strong>
          <p>{mutation.data.kind === 'PASSWORD_RESET'
            ? `账号 ${mutation.data.username} 的密码已重置，该账号在其他设备上的登录会话已失效。`
            : `账号 ${mutation.data.username} 已激活，现在可以使用新密码登录运营控制台。`}</p>
        </div>
        <Link className="button button-primary accept-back-link" to={OPERATOR_PATH}>前往登录</Link>
      </> : <>
        <p className="accept-lede">这是一个一次性邀请链接。请为账号设置一个不少于 8 位的登录密码，设置完成后该链接立即失效。</p>
        <form className="stack-form accept-form" onSubmit={submitPassword}>
          <label htmlFor="accept-password">新密码</label>
          <input id="accept-password" type="password" autoComplete="new-password" value={password} onChange={(event) => setPassword(event.target.value)} minLength={8} maxLength={200} required autoFocus />
          <label htmlFor="accept-confirm-password">确认新密码</label>
          <input id="accept-confirm-password" type="password" autoComplete="new-password" value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} minLength={8} maxLength={200} required />
          <p className="field-note">建议混合大小写字母、数字与符号。密码重置链接会同时下线该账号的其他设备会话。</p>
          {formError && <StatusMessage>{formError}</StatusMessage>}
          {mutation.isError && <StatusMessage>{acceptErrorMessage(mutation.error)}</StatusMessage>}
          <button className="button button-primary" type="submit" disabled={mutation.isPending}>
            <LockKeyhole size={15} aria-hidden="true" />{mutation.isPending ? '正在设置…' : '设置密码'}
          </button>
        </form>
        <p className="accept-footnote"><ShieldAlert size={14} aria-hidden="true" />请勿把此链接转发给他人；链接只能使用一次，有效期由管理员生成时决定。</p>
        <Link className="accept-text-link" to={OPERATOR_PATH}>已有密码？返回登录</Link>
      </>}
    </div>
  </section>
}
