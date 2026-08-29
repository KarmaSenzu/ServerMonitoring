import { useState } from 'react'
import { Outlet, NavLink } from 'react-router-dom'
import {
  FiCpu,
  FiTerminal,
  FiMenu,
  FiX,
  FiBox,
  FiSearch,
  FiUsers,
  FiLogOut,
  FiActivity,
  FiList,
  FiBell,
  FiDatabase,
  FiSettings,
} from 'react-icons/fi'
import { useAuth } from '../auth/useAuth.js'
import './AppLayout.css'

export default function AppLayout() {
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const { user, logout } = useAuth()

  const isAdmin = user?.role === 'admin'

  const closeSidebar = () => setSidebarOpen(false)

  return (
    <div className="app-layout">
      <button
        className="mobile-toggle"
        onClick={() => setSidebarOpen((v) => !v)}
        aria-label="Toggle sidebar"
      >
        {sidebarOpen ? <FiX /> : <FiMenu />}
      </button>

      <aside className={`sidebar ${sidebarOpen ? 'open' : ''}`}>
        <div className="sidebar-brand">
          <div className="brand-icon">V</div>
          <div>
            <h2>VPS Dashboard</h2>
            <span className="brand-tag">Monitoring &amp; Control</span>
          </div>
        </div>

        <nav className="sidebar-nav">
          <span className="nav-section-label">Monitoring</span>
          <NavLink
            to="/"
            end
            className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}
            onClick={closeSidebar}
          >
            <span className="nav-icon"><FiCpu /></span>
            Dashboard
          </NavLink>

          <span className="nav-section-label">Registry</span>
          <NavLink
            to="/projects"
            className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}
            onClick={closeSidebar}
          >
            <span className="nav-icon"><FiBox /></span>
            Projects
          </NavLink>
          <NavLink
            to="/discovery"
            className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}
            onClick={closeSidebar}
          >
            <span className="nav-icon"><FiSearch /></span>
            Discovery
          </NavLink>

          <span className="nav-section-label">Processes</span>
          <NavLink
            to="/pm2"
            className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}
            onClick={closeSidebar}
          >
            <span className="nav-icon"><FiActivity /></span>
            PM2
          </NavLink>

          <span className="nav-section-label">Observability</span>
          <NavLink
            to="/events"
            className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}
            onClick={closeSidebar}
          >
            <span className="nav-icon"><FiList /></span>
            Events
          </NavLink>
          <NavLink
            to="/notifications"
            className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}
            onClick={closeSidebar}
          >
            <span className="nav-icon"><FiBell /></span>
            Notifications
          </NavLink>

          <span className="nav-section-label">Operations</span>
          <NavLink
            to="/backups"
            className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}
            onClick={closeSidebar}
          >
            <span className="nav-icon"><FiDatabase /></span>
            Backups
          </NavLink>
          <NavLink
            to="/environments"
            className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}
            onClick={closeSidebar}
          >
            <span className="nav-icon"><FiSettings /></span>
            Environments
          </NavLink>

          <span className="nav-section-label">Tools</span>
          <NavLink
            to="/generator"
            className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}
            onClick={closeSidebar}
          >
            <span className="nav-icon"><FiTerminal /></span>
            Command Generator
          </NavLink>

          {isAdmin && (
            <>
              <span className="nav-section-label">Administration</span>
              <NavLink
                to="/users"
                className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}
                onClick={closeSidebar}
              >
                <span className="nav-icon"><FiUsers /></span>
                Users
              </NavLink>
            </>
          )}
        </nav>

        <div className="sidebar-footer">
          <div className="auth-summary">
            <div className="auth-avatar">{(user?.username || '?').slice(0, 1).toUpperCase()}</div>
            <div className="auth-meta">
              <span className="auth-username">{user?.username || 'unknown'}</span>
              {user?.role && (
                <span className={`role-badge role-${user.role}`}>{user.role}</span>
              )}
            </div>
          </div>
          <button className="logout-btn" onClick={logout} type="button">
            <FiLogOut />
            Logout
          </button>
        </div>
      </aside>

      <main className="main-content">
        <Outlet />
      </main>
    </div>
  )
}
