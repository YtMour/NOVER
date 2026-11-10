import { lazy, Suspense, useEffect, useState } from 'react'
import { NavLink, Route, Routes, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Moon, Sun, UserPlus } from 'lucide-react'
import { BrandMark } from './components/BrandMark'
import { PageMotion } from './components/PageMotion'
import { DeliveryPage } from './features/delivery/DeliveryPage'
import { RedeemPage } from './features/redeem/RedeemPage'
import { loadServiceHealth } from './lib/api'
import { OPERATOR_PATH } from './lib/routes'

type Theme = 'light' | 'dark'
const AdminPage = lazy(() => import('./features/admin/AdminPage').then((module) => ({ default: module.AdminPage })))
const ProductDetailPage = lazy(() => import('./features/redeem/ProductDetailPage').then((module) => ({ default: module.ProductDetailPage })))
const JoinPage = lazy(() => import('./features/join/JoinPage').then((module) => ({ default: module.JoinPage })))

function resolveInitialTheme(): Theme {
  const saved = localStorage.getItem('nover.theme')
  if (saved === 'light' || saved === 'dark') return saved
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function Shell() {
  const navigate = useNavigate()
  const [theme, setTheme] = useState<Theme>(resolveInitialTheme)
  const health = useQuery({ queryKey: ['service-health'], queryFn: loadServiceHealth, refetchInterval: 30_000, retry: false })
  const deliveryToken = sessionStorage.getItem('nover.deliveryToken') ?? ''
  const currentPath = window.location.pathname
  const operatorView = currentPath === OPERATOR_PATH

  useEffect(() => {
    if (deliveryToken && currentPath === '/') navigate('/delivery', { replace: true })
  }, [currentPath, deliveryToken, navigate])

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    document.documentElement.style.colorScheme = theme
    document.querySelector('meta[name="theme-color"]')?.setAttribute('content', theme === 'dark' ? '#111516' : '#f6f6f3')
    localStorage.setItem('nover.theme', theme)
  }, [theme])

  function logout() {
    sessionStorage.removeItem('nover.deliveryToken')
    navigate('/', { replace: true })
  }

  return (
    <div className="app-shell">
      <header className="topbar">
        <NavLink to="/" className="brand-link" aria-label="NOVER 首页"><BrandMark /></NavLink>
        <nav className="top-navigation" aria-label="主导航">
          <NavLink to="/" end className={({ isActive }) => `nav-link ${isActive ? 'is-active' : ''}`}>交付门户</NavLink>
          {operatorView && <span className="nav-link is-active">运营控制台</span>}
        </nav>
        {!operatorView && <NavLink to="/join" className={({ isActive }) => `join-link ${isActive ? 'is-active' : ''}`}><UserPlus size={15} /><span>加入我们</span></NavLink>}
        <button
          className="theme-toggle"
          type="button"
          aria-label={theme === 'dark' ? '切换到浅色模式' : '切换到暗色模式'}
          title={theme === 'dark' ? '切换到浅色模式' : '切换到暗色模式'}
          onClick={() => setTheme((current) => current === 'dark' ? 'light' : 'dark')}
        >
          {theme === 'dark' ? <Sun size={17} /> : <Moon size={17} />}
        </button>
      </header>
      <main className="main-content">
        <PageMotion>
          <Suspense fallback={<div className="loading-line">正在打开页面…</div>}>
            <Routes>
              <Route path="/" element={<RedeemPage />} />
              <Route path="/delivery" element={<DeliveryPage token={deliveryToken} onLogout={logout} />} />
              <Route path="/products/:productID" element={<ProductDetailPage />} />
              <Route path="/join" element={<JoinPage />} />
              <Route path={OPERATOR_PATH} element={<AdminPage />} />
              <Route path="*" element={<RedeemPage />} />
            </Routes>
          </Suspense>
        </PageMotion>
      </main>
      <footer className="site-footer">
        <div><span>© {new Date().getFullYear()} NOVER</span><span className="footer-dot" /><span>探索数字，发现更多可能。</span></div>
        <span className={`footer-status ${health.isError ? 'footer-status--offline' : ''}`}><i />{health.isLoading ? '正在连接' : health.isError ? '服务暂不可用' : '交付服务在线'}</span>
      </footer>
    </div>
  )
}

export default function App() {
  return <Shell />
}
