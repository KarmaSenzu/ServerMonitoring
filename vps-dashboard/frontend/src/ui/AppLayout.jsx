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
  FiServer,
  FiKey,
  FiShuffle,
  FiCloud,
  FiGrid,
} from 'react-icons/fi'
import { useAuth } from '../auth/useAuth.js'
import './AppLayout.css'

import GlobalSearch from './GlobalSearch.jsx'

export default function AppLayout() {
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const { user, logout } = useAuth()

  const isAdmin = user?.role === 'admin'

  const closeSidebar = () => setSidebarOpen(false)

  return (
    <div className="app-layout">
      {/* Top Navigation Bar */}
      <header className="top-nav">
        <button
          className="top-nav-icon-btn mobile-only"
          onClick={() => setSidebarOpen((v) => !v)}
          aria-label="Toggle sidebar"
        >
          {sidebarOpen ? <FiX /> : <FiMenu />}
        </button>

        <div className="top-nav-brand">
          <span className="top-nav-brand-text">Infrastructure<span className="brand-dot" /></span>
          <span className="brand-tag">COMMAND CENTER</span>
        </div>

        <div className="top-nav-spacer" />

        <div className="top-nav-actions">
          <GlobalSearch />
          <button className="top-nav-icon-btn" title="Notifications">
            <FiBell />
            <span className="top-nav-badge">3</span>
          </button>
          <div className="top-nav-user">
            <div className="top-nav-user-avatar">
              {(user?.username || '?').slice(0, 1).toUpperCase()}
            </div>
            <div className="top-nav-user-info">
              <span className="top-nav-user-name">{user?.username || 'unknown'}</span>
              {user?.role && (
                <span className={`role-badge role-${user.role}`}>{user.role}</span>
              )}
            </div>
          </div>
          <button className="top-nav-icon-btn" onClick={logout} title="Logout">
            <FiLogOut />
          </button>
        </div>
      </header>

      {/* Sidebar */}
      <aside className={`sidebar ${sidebarOpen ? 'open' : ''}`}>
        <div className="sidebar-brand">
          <div className="brand-icon">◉</div>
          <div>
            <h2>Infrastructure<span className="brand-dot" /></h2>
            <span className="brand-tag">COMMAND CENTER</span>
          </div>
        </div>

        <nav className="sidebar-nav">
          <span className="nav-section-label">Overview</span>
          <NavLink
            to="/"
            end
            className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}
            onClick={closeSidebar}
          >
            <span className="nav-icon"><FiGrid /></span>
            Dashboard
          </NavLink>

          <span className="nav-section-label">Monitoring</span>
          <NavLink
            to="/servers"
            className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}
            onClick={closeSidebar}
          >
            <span className="nav-icon"><FiServer /></span>
            Servers
          </NavLink>
          <NavLink
            to="/containers"
            className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}
            onClick={closeSidebar}
          >
            <span className="nav-icon"><FiBox /></span>
            Containers
          </NavLink>

          <span className="nav-section-label">Operations</span>
          <NavLink
            to="/ssh-keys"
            className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}
            onClick={closeSidebar}
          >
            <span className="nav-icon"><FiKey /></span>
            SSH Keys
          </NavLink>
          <NavLink
            to="/commands"
            className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}
            onClick={closeSidebar}
          >
            <span className="nav-icon"><FiTerminal /></span>
            Commands
          </NavLink>
          <NavLink
            to="/tunnels"
            className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}
            onClick={closeSidebar}
          >
            <span className="nav-icon"><FiShuffle /></span>
            SSH Tunnels
          </NavLink>

          <span className="nav-section-label">Infrastructure</span>
          <NavLink
            to="/cloud"
            className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}
            onClick={closeSidebar}
          >
            <span className="nav-icon"><FiCloud /></span>
            Cloud Discovery
          </NavLink>
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

          <span className="nav-section-label">System</span>
          <NavLink
            to="/database"
            className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}
            onClick={closeSidebar}
          >
            <span className="nav-icon"><FiDatabase /></span>
            Database
          </NavLink>
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

      {/* Main Content */}
      <main className="main-content">
        <Outlet />
      </main>
    </div>
  )
}
