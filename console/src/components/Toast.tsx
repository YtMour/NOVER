import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { AlertCircle, CheckCircle2, Info, X } from 'lucide-react'

export type ToastKind = 'success' | 'error' | 'info'

type ToastItem = { id: number; kind: ToastKind; message: string }

type ToastApi = {
  push: (kind: ToastKind, message: string) => number
  success: (message: string) => number
  error: (message: string) => number
  info: (message: string) => number
  dismiss: (id: number) => void
}

const ToastContext = createContext<ToastApi | null>(null)
const MAX_TOASTS = 4
const DURATION: Record<ToastKind, number> = { success: 4200, info: 5200, error: 8000 }

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([])
  const nextId = useRef(0)
  const timers = useRef(new Map<number, number>())

  const dismiss = useCallback((id: number) => {
    setItems((current) => current.filter((item) => item.id !== id))
    const timer = timers.current.get(id)
    if (timer !== undefined) {
      window.clearTimeout(timer)
      timers.current.delete(id)
    }
  }, [])

  const push = useCallback((kind: ToastKind, message: string) => {
    const trimmed = message.trim()
    if (!trimmed) return 0
    const id = ++nextId.current
    setItems((current) => [...current, { id, kind, message: trimmed }].slice(-MAX_TOASTS))
    const timer = window.setTimeout(() => dismiss(id), DURATION[kind])
    timers.current.set(id, timer)
    return id
  }, [dismiss])

  useEffect(() => {
    const pending = timers.current
    return () => {
      pending.forEach((timer) => window.clearTimeout(timer))
      pending.clear()
    }
  }, [])

  // Escape dismisses every visible toast, matching the keyboard close affordance
  // on each card even when focus is still inside the page form that raised it.
  useEffect(() => {
    if (!items.length) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      items.forEach((item) => dismiss(item.id))
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [items, dismiss])

  const api = useMemo<ToastApi>(() => ({
    push,
    dismiss,
    success: (message) => push('success', message),
    error: (message) => push('error', message),
    info: (message) => push('info', message),
  }), [push, dismiss])

  return <ToastContext.Provider value={api}>
    {children}
    <div className="toast-region" aria-label="操作提示">
      {items.map((item) => <ToastCard item={item} key={item.id} onDismiss={dismiss} />)}
    </div>
  </ToastContext.Provider>
}

export function useToast() {
  const api = useContext(ToastContext)
  if (!api) throw new Error('useToast 必须在 ToastProvider 内使用。')
  return api
}

function ToastCard({ item, onDismiss }: { item: ToastItem; onDismiss: (id: number) => void }) {
  const Icon = item.kind === 'success' ? CheckCircle2 : item.kind === 'info' ? Info : AlertCircle
  const role = item.kind === 'error' ? 'alert' : 'status'
  return <div className={`toast toast--${item.kind}`} role={role} aria-live={item.kind === 'error' ? 'assertive' : 'polite'} aria-atomic="true">
    <Icon size={17} aria-hidden="true" />
    <span className="toast-message">{item.message}</span>
    <button className="toast-close" type="button" aria-label="关闭提示" title="关闭提示（Esc）" onClick={() => onDismiss(item.id)}>
      <X size={15} aria-hidden="true" />
    </button>
  </div>
}
