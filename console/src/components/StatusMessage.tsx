import { AlertCircle, CheckCircle2, Info } from 'lucide-react'

export function StatusMessage({
  kind = 'error',
  children,
}: {
  kind?: 'error' | 'success' | 'info'
  children: string
}) {
  const Icon = kind === 'success' ? CheckCircle2 : kind === 'info' ? Info : AlertCircle
  return (
    <div className={`status-message status-message--${kind}`} role={kind === 'error' ? 'alert' : 'status'}>
      <Icon size={17} aria-hidden="true" />
      <span>{children}</span>
    </div>
  )
}
