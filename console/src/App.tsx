import { lazy, Suspense, useEffect, useState } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Moon, Server, Sun } from 'lucide-react'
import { BrandMark } from './components/BrandMark'
import { loadServiceHealth } from './lib/api'
import { OPERATOR_PATH } from './lib/routes'

type Theme = 'light' | 'dark'
const AdminPage = lazy(() => import('./features/admin/AdminPage').then((module) => ({ default: module.AdminPage })))

function initialTheme(): Theme {
  const saved = localStorage.getItem('nover.console.theme')
  if (saved === 'light' || saved === 'dark') return saved
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export default function App() {
  const [theme, setTheme] = useState<Theme>(initialTheme)
  const health = useQuery({ queryKey: ['console-service-health'], queryFn: loadServiceHealth, refetchInterval: 30_000, retry: false })

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    document.documentElement.style.colorScheme = theme
    document.querySelector('meta[name="theme-color"]')?.setAttribute('content', theme === 'dark' ? '#111516' : '#f6f6f3')
    localStorage.setItem('nover.console.theme', theme)
  }, [theme])

  return <div className="console-shell">
    <header className="console-topbar">
      <div className="console-brand"><BrandMark /><span>运营控制台</span></div>
      <div className="console-runtime">
        <span className={health.isError ? 'is-offline' : ''}><Server size={14} />{health.isLoading ? '正在连接 API' : health.isError ? 'API 连接异常' : 'API 已连接'}</span>
        <button className="theme-toggle" type="button" aria-label={theme === 'dark' ? '切换到浅色模式' : '切换到暗色模式'} title={theme === 'dark' ? '切换到浅色模式' : '切换到暗色模式'} onClick={() => setTheme((current) => current === 'dark' ? 'light' : 'dark')}>
          {theme === 'dark' ? <Sun size={17} /> : <Moon size={17} />}
        </button>
      </div>
    </header>
    <main className="console-content">
      <Suspense fallback={<div className="loading-line">正在打开运营控制台…</div>}>
        <Routes>
          <Route path={OPERATOR_PATH} element={<AdminPage />} />
          <Route path="/" element={<Navigate to={OPERATOR_PATH} replace />} />
          <Route path="*" element={<Navigate to={OPERATOR_PATH} replace />} />
        </Routes>
      </Suspense>
    </main>
  </div>
}
