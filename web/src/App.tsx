import { useEffect, useState } from 'react'
import { NavLink, Route, Routes, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { ArrowUpRight, CircleHelp } from 'lucide-react'
import { BrandMark } from './components/BrandMark'
import { PageMotion } from './components/PageMotion'
import { AdminPage } from './features/admin/AdminPage'
import { DeliveryPage } from './features/delivery/DeliveryPage'
import { RedeemPage } from './features/redeem/RedeemPage'
import { loadServiceHealth } from './lib/api'

function Shell() {
  const navigate = useNavigate()
  const [helpOpen, setHelpOpen] = useState(false)
  const health = useQuery({ queryKey: ['service-health'], queryFn: loadServiceHealth, refetchInterval: 30_000, retry: false })
  const deliveryToken = sessionStorage.getItem('nover.deliveryToken') ?? ''
  const currentPath = window.location.pathname

  useEffect(() => {
    if (deliveryToken && currentPath === '/') navigate('/delivery', { replace: true })
  }, [currentPath, deliveryToken, navigate])

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
          <NavLink to="/workspace" className={({ isActive }) => `nav-link ${isActive ? 'is-active' : ''}`}>运营工作台</NavLink>
        </nav>
        <div className="help-wrap">
          <button className="help-link" aria-expanded={helpOpen} onClick={() => setHelpOpen((open) => !open)}><CircleHelp size={16} /><span>帮助</span><ArrowUpRight size={13} className="help-arrow" /></button>
          {helpOpen && <div className="help-popover" role="dialog" aria-label="领取帮助"><strong>兑换与领取</strong><p>兑换码由你购买商品的平台卖家发送。遇到代码状态或文件问题，请从原购买渠道联系卖家。</p><button onClick={() => setHelpOpen(false)}>知道了</button></div>}
        </div>
      </header>
      <main className="main-content">
        <PageMotion>
          <Routes>
            <Route path="/" element={<RedeemPage />} />
            <Route path="/delivery" element={<DeliveryPage token={deliveryToken} onLogout={logout} />} />
            <Route path="/workspace" element={<AdminPage />} />
            <Route path="*" element={<RedeemPage />} />
          </Routes>
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
