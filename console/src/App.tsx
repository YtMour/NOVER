import { lazy, Suspense, useEffect, useState } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Moon, Server, Sun } from 'lucide-react'
import { BrandMark } from './components/BrandMark'
import { loadMeta, loadServiceHealth } from './lib/api'
import { OPERATOR_PATH } from './lib/routes'

type Theme = 'light' | 'dark'
const AdminPage = lazy(() => import('./features/admin/AdminPage').then((module) => ({ default: module.AdminPage })))
const AcceptInvitationPage = lazy(() => import('./features/admin/AcceptInvitationView').then((module) => ({ default: module.AcceptInvitationPage })))

// 构建元数据的版本可能已带 v 前缀，统一显示为 vX.Y.Z，避免 vv0.5.0；
// 未注入版本的开发构建保留后端原值 dev。
function displayVersion(version?: string) {
  if (!version) return ''
  if (version === 'dev') return 'dev'
  return version.startsWith('v') ? version : `v${version}`
}

function initialTheme(): Theme {
  const saved = localStorage.getItem('nover.console.theme')
  if (saved === 'light' || saved === 'dark') return saved
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export default function App() {
  const [theme, setTheme] = useState<Theme>(initialTheme)
  const health = useQuery({ queryKey: ['console-service-health'], queryFn: loadServiceHealth, refetchInterval: 30_000, retry: false })
  // 顶栏版本只是展示增强：元数据请求失败时保留原有 API 连接状态，不显示版本。
  const meta = useQuery({ queryKey: ['console-meta'], queryFn: loadMeta, retry: false, staleTime: 5 * 60_000 })
  const apiVersion = displayVersion(meta.data?.version)

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    document.documentElement.style.colorScheme = theme
    document.querySelector('meta[name="theme-color"]')?.setAttribute('content', theme === 'dark' ? '#111516' : '#f6f6f3')
    localStorage.setItem('nover.console.theme', theme)
  }, [theme])

  return <div className="console-shell">
    <SkipLink />
    <header className="console-topbar">
      <div className="console-brand"><BrandMark /><span>运营控制台</span></div>
      <div className="console-runtime">
        <span className={health.isError ? 'is-offline' : ''}><Server size={14} aria-hidden="true" />{health.isLoading ? '正在连接 API' : health.isError ? 'API 连接异常' : 'API 已连接'}</span>
        {apiVersion && <span className="console-api-version" title={`当前 API 版本 ${apiVersion}`} aria-label={`当前 API 版本 ${apiVersion}`}>API {apiVersion}</span>}
        <button className="theme-toggle" type="button" aria-label={theme === 'dark' ? '切换到浅色模式' : '切换到暗色模式'} aria-pressed={theme === 'dark'} title={theme === 'dark' ? '切换到浅色模式' : '切换到暗色模式'} onClick={() => setTheme((current) => current === 'dark' ? 'light' : 'dark')}>
          {theme === 'dark' ? <Sun size={17} aria-hidden="true" /> : <Moon size={17} aria-hidden="true" />}
        </button>
      </div>
    </header>
    <main className="console-content" id="main" tabIndex={-1}>
      <Suspense fallback={<div className="loading-line">正在打开运营控制台…</div>}>
        <Routes>
          <Route path={`${OPERATOR_PATH}/accept`} element={<AcceptInvitationPage />} />
          <Route path={OPERATOR_PATH} element={<AdminPage />} />
          <Route path="/" element={<Navigate to={OPERATOR_PATH} replace />} />
          <Route path="*" element={<Navigate to={OPERATOR_PATH} replace />} />
        </Routes>
      </Suspense>
    </main>
  </div>
}

function SkipLink() {
  return (
    <a
      className="skip-link"
      href="#main"
      onClick={(event) => {
        event.preventDefault()
        const main = document.getElementById('main')
        main?.focus()
        main?.scrollIntoView({ block: 'start' })
      }}
    >
      跳到主要内容
    </a>
  )
}
