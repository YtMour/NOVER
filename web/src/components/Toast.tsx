import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { AlertCircle, CheckCircle2, Info, X } from 'lucide-react'

export type ToastKind = 'success' | 'error' | 'info'

export type ToastInput = {
  kind?: ToastKind
  message: string
  title?: string
  duration?: number
}

type ToastItem = {
  id: number
  kind: ToastKind
  message: string
  title?: string
  duration: number
}

type ToastContextValue = {
  notify: (input: ToastInput) => number
  dismiss: (id: number) => void
}

const ToastContext = createContext<ToastContextValue | null>(null)

const DEFAULT_DURATION: Record<ToastKind, number> = { success: 3000, info: 3800, error: 6500 }
const MAX_VISIBLE_TOASTS = 4

/** 轻量通知：不引入新依赖，复制、下载失败与网络错误等短反馈优先走这里。 */
export function useToast() {
  const context = useContext(ToastContext)
  if (!context) throw new Error('useToast 必须在 ToastProvider 内使用。')
  return context
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([])
  const nextID = useRef(1)

  const dismiss = useCallback((id: number) => {
    setToasts((current) => current.filter((toast) => toast.id !== id))
  }, [])

  const notify = useCallback((input: ToastInput) => {
    const kind = input.kind ?? 'info'
    const id = nextID.current++
    const item: ToastItem = {
      id,
      kind,
      message: input.message,
      title: input.title,
      duration: Math.max(1600, input.duration ?? DEFAULT_DURATION[kind]),
    }
    setToasts((current) => {
      // 相同消息只保留一条，避免连续点击把通知堆成列表。
      const remaining = current.filter((toast) => !(toast.kind === item.kind && toast.message === item.message))
      return [...remaining, item].slice(-MAX_VISIBLE_TOASTS)
    })
    return id
  }, [])

  const value = useMemo(() => ({ notify, dismiss }), [notify, dismiss])

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="toast-viewport" role="region" aria-label="操作通知">
        {toasts.map((toast) => <ToastCard key={toast.id} toast={toast} onDismiss={dismiss} />)}
      </div>
    </ToastContext.Provider>
  )
}

function ToastCard({ toast, onDismiss }: { toast: ToastItem; onDismiss: (id: number) => void }) {
  const [paused, setPaused] = useState(false)
  const remaining = useRef(toast.duration)

  useEffect(() => {
    // 悬停或聚焦（例如键盘 Tab 到关闭按钮）时暂停倒计时，离开后按剩余时间继续。
    if (paused) return
    const startedAt = Date.now()
    const timer = window.setTimeout(() => onDismiss(toast.id), remaining.current)
    return () => {
      window.clearTimeout(timer)
      remaining.current = Math.max(800, remaining.current - (Date.now() - startedAt))
    }
  }, [onDismiss, paused, toast.id])

  const Icon = toast.kind === 'success' ? CheckCircle2 : toast.kind === 'error' ? AlertCircle : Info

  return (
    <div
      className={`toast toast--${toast.kind}`}
      role={toast.kind === 'error' ? 'alert' : 'status'}
      aria-live={toast.kind === 'error' ? 'assertive' : 'polite'}
      aria-atomic="true"
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onFocus={() => setPaused(true)}
      onBlur={() => setPaused(false)}
    >
      <Icon className="toast-icon" size={17} aria-hidden="true" />
      <div className="toast-copy">
        {toast.title && <strong>{toast.title}</strong>}
        <span>{toast.message}</span>
      </div>
      <button className="toast-close" type="button" aria-label="关闭此通知" title="关闭通知" onClick={() => onDismiss(toast.id)}>
        <X size={14} aria-hidden="true" />
      </button>
    </div>
  )
}
