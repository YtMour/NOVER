import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Copy, Download, KeyRound, RefreshCw, ShieldCheck, ShieldOff, Smartphone } from 'lucide-react'
import { StatusMessage } from '../../components/StatusMessage'
import { SkeletonLines } from '../../components/Skeleton'
import { useToast } from '../../components/Toast'
import { disableMfa, enableMfa, loadMfaStatus, setupMfa } from '../../lib/api'
import { copyText } from '../../lib/clipboard'
import { describeError } from '../../lib/errors'
import type { MfaSetupResponse } from '../../types'

function formatTime(value?: string | null) {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN')
}

export function MfaCard({ token }: { token: string }) {
  const toast = useToast()
  const queryClient = useQueryClient()
  const status = useQuery({ queryKey: ['operator-mfa', token], queryFn: () => loadMfaStatus(token), retry: false })
  const [setup, setSetup] = useState<MfaSetupResponse | null>(null)
  const [enableCode, setEnableCode] = useState('')
  const [disablePassword, setDisablePassword] = useState('')
  const [disableCode, setDisableCode] = useState('')
  const [formError, setFormError] = useState('')
  const [copied, setCopied] = useState('')

  const refreshStatus = () => queryClient.invalidateQueries({ queryKey: ['operator-mfa', token] })

  function flashCopied(key: string) {
    setCopied(key)
    window.setTimeout(() => setCopied((current) => current === key ? '' : current), 1800)
  }

  async function copyValue(value: string, key: string, label: string) {
    const ok = await copyText(value)
    if (ok) {
      flashCopied(key)
      toast.success(`${label}已复制。`)
    } else {
      toast.error('复制失败，请手动选择文本复制。')
    }
  }

  const setupMutation = useMutation({
    mutationFn: () => setupMfa(token),
    onSuccess: (data) => {
      setSetup(data)
      setEnableCode('')
      setFormError('')
      toast.success('已生成 TOTP 密钥与 10 个恢复码，请立即保存。')
    },
    onError: (error) => toast.error(describeError(error)),
  })

  const enableMutation = useMutation({
    mutationFn: () => enableMfa(token, enableCode),
    onSuccess: async () => {
      setSetup(null)
      setEnableCode('')
      setFormError('')
      toast.success('两步验证已启用，下次登录需要输入动态验证码。')
      await refreshStatus()
    },
    onError: (error) => toast.error(describeError(error)),
  })

  const disableMutation = useMutation({
    mutationFn: () => disableMfa(token, disablePassword, disableCode),
    onSuccess: async () => {
      setDisablePassword('')
      setDisableCode('')
      setFormError('')
      toast.success('两步验证已关闭，恢复码同时清除。')
      await refreshStatus()
    },
    onError: (error) => toast.error(describeError(error)),
  })

  function submitEnable(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!/^\d{6}$/.test(enableCode.trim())) {
      setFormError('请输入身份验证器显示的 6 位数字验证码。')
      return
    }
    setFormError('')
    enableMutation.mutate()
  }

  function submitDisable(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!disablePassword) {
      setFormError('请输入当前登录密码。')
      return
    }
    if (disableCode.trim().length < 6) {
      setFormError('请输入 6 位动态验证码，或一个恢复码。')
      return
    }
    setFormError('')
    disableMutation.mutate()
  }

  function downloadRecoveryCodes() {
    if (!setup) return
    const text = [
      '知屿 NOVER 两步验证恢复码',
      `生成时间：${new Date().toLocaleString('zh-CN')}`,
      `账号密钥：${setup.secret}`,
      'otpauth：' + setup.otpauth_url,
      '',
      ...setup.recovery_codes,
      '',
      '每个恢复码只能使用一次。请离线保存，不要通过聊天工具明文发送。',
    ].join('\n')
    const blob = new Blob([text], { type: 'text/plain;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = 'nover-mfa-recovery-codes.txt'
    document.body.appendChild(anchor)
    anchor.click()
    document.body.removeChild(anchor)
    window.setTimeout(() => URL.revokeObjectURL(url), 1000)
    toast.success('恢复码文本已开始下载。')
  }

  const enabled = status.data?.enabled ?? false
  const remaining = status.data?.recovery_codes_remaining ?? 0

  return <section className="admin-section account-card mfa-card">
    <div className="admin-section-heading">
      <div className="tool-icon tool-icon--green"><Smartphone size={18} /></div>
      <div><span className="section-kicker">TWO-FACTOR AUTH</span><h2>两步验证（TOTP）</h2></div>
      <button className="text-icon-button account-refresh" title="刷新两步验证状态" onClick={() => void status.refetch()} disabled={status.isFetching}><RefreshCw size={14} />刷新</button>
    </div>

    {status.isError && <StatusMessage>{describeError(status.error)}</StatusMessage>}
    {status.isLoading ? <SkeletonLines lines={3} label="正在读取两步验证状态" /> : enabled ? <div className="mfa-enabled-panel">
      <div className="mfa-status-row"><ShieldCheck size={17} aria-hidden="true" /><strong>两步验证已启用</strong><span>启用时间 {formatTime(status.data?.enabled_at)}</span></div>
      <p className="field-note">登录时除密码外还需要输入身份验证器生成的 6 位动态验证码，或使用一次性恢复码。剩余恢复码 <strong>{remaining}</strong> 个{remaining <= 3 ? '，建议关闭后重新启用并重新保存恢复码。' : '。'}</p>
      <form className="stack-form compact-form mfa-disable-form" onSubmit={submitDisable}>
        <label htmlFor="mfa-disable-password">当前密码</label>
        <input id="mfa-disable-password" type="password" autoComplete="current-password" value={disablePassword} onChange={(event) => setDisablePassword(event.target.value)} maxLength={200} required />
        <label htmlFor="mfa-disable-code">动态验证码或恢复码</label>
        <input id="mfa-disable-code" value={disableCode} onChange={(event) => setDisableCode(event.target.value)} autoComplete="one-time-code" maxLength={64} placeholder="6 位数字，或 XXXX-XXXX-XXXX" required />
        <p className="field-note">如果身份验证器不可用，可输入一个恢复码代替动态验证码，恢复码使用后立即失效。关闭两步验证会同时清除全部恢复码。</p>
        {formError && <StatusMessage>{formError}</StatusMessage>}
        <button className="button button-secondary mfa-danger-button" type="submit" disabled={disableMutation.isPending}><ShieldOff size={15} />{disableMutation.isPending ? '正在关闭…' : '关闭两步验证'}</button>
      </form>
    </div> : setup ? <div className="mfa-setup-panel">
      <p className="field-note">用身份验证器（如 1Password、Authy、Google Authenticator）扫描或手动添加下列密钥，然后输入设备显示的一次性验证码完成启用。</p>
      <div className="mfa-secret-grid">
        <div><span>手动输入密钥</span><code>{setup.secret}</code></div>
        <div><span>otpauth 链接</span><code className="mfa-otpauth">{setup.otpauth_url}</code></div>
      </div>
      <div className="mfa-copy-actions">
        <button className="button button-secondary button-small" type="button" onClick={() => void copyValue(setup.secret, 'secret', '密钥')}><Copy size={14} />{copied === 'secret' ? '已复制' : '复制密钥'}</button>
        <button className="button button-secondary button-small" type="button" onClick={() => void copyValue(setup.otpauth_url, 'otpauth', 'otpauth 链接')}><Copy size={14} />{copied === 'otpauth' ? '已复制' : '复制 otpauth 链接'}</button>
      </div>
      <div className="mfa-recovery-heading"><span className="section-kicker">RECOVERY CODES</span><strong>恢复码（仅显示一次）</strong><button className="text-icon-button" type="button" onClick={downloadRecoveryCodes}><Download size={14} />下载文本</button></div>
      <div className="mfa-recovery-grid">{setup.recovery_codes.map((recoveryCode) => <code key={recoveryCode}>{recoveryCode}</code>)}</div>
      <div className="mfa-copy-actions">
        <button className="button button-secondary button-small" type="button" onClick={() => void copyValue(setup.recovery_codes.join('\n'), 'all', '全部恢复码')}><Copy size={14} />{copied === 'all' ? '已复制' : '复制全部恢复码'}</button>
      </div>
      <form className="stack-form compact-form mfa-enable-form" onSubmit={submitEnable}>
        <label htmlFor="mfa-enable-code">身份验证器验证码</label>
        <input id="mfa-enable-code" value={enableCode} onChange={(event) => setEnableCode(event.target.value)} inputMode="numeric" pattern="[0-9]{6}" maxLength={6} autoComplete="one-time-code" placeholder="6 位数字" required />
        {formError && <StatusMessage>{formError}</StatusMessage>}
        <div className="mfa-form-actions">
          <button className="button button-primary" type="submit" disabled={enableMutation.isPending}><Check size={15} />{enableMutation.isPending ? '正在启用…' : '输入验证码并启用'}</button>
          <button className="button button-secondary" type="button" onClick={() => { setSetup(null); setFormError('') }}>取消设置</button>
        </div>
      </form>
    </div> : <div className="mfa-disabled-panel">
      <p className="field-note">两步验证尚未启用。启用后登录需要额外的动态验证码，可显著降低密码泄露带来的风险。</p>
      <button className="button button-primary" type="button" disabled={setupMutation.isPending} onClick={() => setupMutation.mutate()}><KeyRound size={15} />{setupMutation.isPending ? '正在生成…' : '开始设置两步验证'}</button>
    </div>}
  </section>
}
