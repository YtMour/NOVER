import { lazy, Suspense, useEffect, useState } from 'react'
import { NavLink, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Moon, Sun, UserPlus } from 'lucide-react'
import { BrandMark } from './components/BrandMark'
import { PageMotion } from './components/PageMotion'
import { RouteLoading } from './components/Skeleton'
import { DeliveryPage } from './features/delivery/DeliveryPage'
import { RedeemPage } from './features/redeem/RedeemPage'
import { loadMeta, loadServiceHealth } from './lib/api'

type Theme = 'light' | 'dark'
const ProductDetailPage = lazy(() => import('./features/redeem/ProductDetailPage').then((module) => ({ default: module.ProductDetailPage })))
const JoinPage = lazy(() => import('./features/join/JoinPage').then((module) => ({ default: module.JoinPage })))

// 构建元数据里的版本可能带或不带前缀 v，统一显示为 vX.Y.Z，避免出现 vv0.5.0；
// 未注入版本的开发构建保留后端原值 dev，不强行加 v 前缀。
function displayVersion(version?: string) {
  if (!version) return ''
  if (version === 'dev') return 'dev'
  return version.startsWith('v') ? version : `v${version}`
}

function resolveInitialTheme(): Theme {
  const saved = localStorage.getItem('nover.theme')
  if (saved === 'light' || saved === 'dark') return saved
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function Shell() {
  const navigate = useNavigate()
  const location = useLocation()
  const [theme, setTheme] = useState<Theme>(resolveInitialTheme)
  const health = useQuery({ queryKey: ['service-health'], queryFn: loadServiceHealth, refetchInterval: 30_000, retry: false })
  // 版本信息只做展示增强：请求失败或字段缺失时页脚保持原样，绝不阻塞页面。
  const meta = useQuery({ queryKey: ['meta'], queryFn: loadMeta, retry: false, staleTime: 5 * 60_000 })
  const version = displayVersion(meta.data?.version)
  const deliveryToken = sessionStorage.getItem('nover.deliveryToken') ?? ''
  const currentPath = location.pathname

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
      <SkipLink />
      <header className="topbar">
        <NavLink to={currentPath.startsWith('/spaces/') ? `/spaces/${currentPath.split('/')[2]}` : '/'} className="brand-link" aria-label="NOVER 首页"><BrandMark /></NavLink>
        <nav className="top-navigation" aria-label="主导航">
          <NavLink to={currentPath.startsWith('/spaces/') ? `/spaces/${currentPath.split('/')[2]}` : '/'} end className={({ isActive }) => `nav-link ${isActive ? 'is-active' : ''}`}>交付门户</NavLink>
        </nav>
        <NavLink to="/join" className={({ isActive }) => `join-link ${isActive ? 'is-active' : ''}`}><UserPlus size={15} aria-hidden="true" /><span>加入我们</span></NavLink>
        <button
          className="theme-toggle"
          type="button"
          aria-label={theme === 'dark' ? '切换到浅色模式' : '切换到暗色模式'}
          aria-pressed={theme === 'dark'}
          title={theme === 'dark' ? '切换到浅色模式' : '切换到暗色模式'}
          onClick={() => setTheme((current) => current === 'dark' ? 'light' : 'dark')}
        >
          {theme === 'dark' ? <Sun size={17} aria-hidden="true" /> : <Moon size={17} aria-hidden="true" />}
        </button>
      </header>
      <main className="main-content" id="main" tabIndex={-1}>
        <PageMotion>
          <Suspense fallback={<RouteLoading />}>
            <Routes>
              <Route path="/" element={<RedeemPage />} />
              <Route path="/spaces/:spaceSlug" element={<RedeemPage />} />
              <Route path="/delivery" element={<DeliveryPage token={deliveryToken} onLogout={logout} />} />
              <Route path="/products/:productID" element={<ProductDetailPage />} />
              <Route path="/spaces/:spaceSlug/products/:productID" element={<ProductDetailPage />} />
              <Route path="/join" element={<JoinPage />} />
              <Route path="*" element={<RedeemPage />} />
            </Routes>
          </Suspense>
        </PageMotion>
      </main>
      <footer className="site-footer">
        <div><span>© {new Date().getFullYear()} NOVER</span><span className="footer-dot" /><span>探索数字，发现更多可能。</span></div>
        <span className="footer-runtime">
          <span className={`footer-status ${health.isError ? 'footer-status--offline' : ''}`}><i />{health.isLoading ? '正在连接' : health.isError ? '服务暂不可用' : '交付服务在线'}</span>
          {version && <span className="footer-version" title={`当前交付版本 ${version}`} aria-label={`当前交付版本 ${version}`}>{version}</span>}
        </span>
      </footer>
    </div>
  )
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

export default function App() {
  return <Shell />
}
