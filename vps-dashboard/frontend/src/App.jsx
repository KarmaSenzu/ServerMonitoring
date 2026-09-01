import { Routes, Route, Navigate } from 'react-router-dom'
import './App.css'
import AppLayout from './ui/AppLayout.jsx'
import RequireAuth from './auth/RequireAuth.jsx'
import RequireRole from './auth/RequireRole.jsx'
import LoginPage from './pages/Login.jsx'
import DashboardPage from './pages/Dashboard.jsx'
import ProjectsPage from './pages/Projects.jsx'
import ServersPage from './pages/Servers.jsx'
import SSHKeysPage from './pages/SSHKeys.jsx'
import ContainersPage from './pages/Containers.jsx'
import CommandsPage from './pages/Commands.jsx'
import TunnelsPage from './pages/Tunnels.jsx'
import CloudDiscoveryPage from './pages/CloudDiscovery.jsx'
import DiscoveryPage from './pages/Discovery.jsx'
import GeneratorPage from './pages/Generator.jsx'
import UsersPage from './pages/Users.jsx'
import PM2Page from './pages/PM2.jsx'
import EventsPage from './pages/Events.jsx'
import NotificationsPage from './pages/Notifications.jsx'
import BackupsPage from './pages/Backups.jsx'
import EnvironmentsPage from './pages/Environments.jsx'
import NotFoundPage from './pages/NotFound.jsx'

function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        element={
          <RequireAuth>
            <AppLayout />
          </RequireAuth>
        }
      >
        <Route path="/" element={<DashboardPage />} />
        <Route path="/projects" element={<ProjectsPage />} />
        <Route path="/servers" element={<ServersPage />} />
        <Route path="/ssh-keys" element={<SSHKeysPage />} />
        <Route path="/containers" element={<ContainersPage />} />
        <Route path="/commands" element={<CommandsPage />} />
        <Route path="/tunnels" element={<TunnelsPage />} />
        <Route path="/cloud" element={<CloudDiscoveryPage />} />
        <Route path="/discovery" element={<DiscoveryPage />} />
        <Route path="/pm2" element={<PM2Page />} />
        <Route path="/events" element={<EventsPage />} />
        <Route path="/notifications" element={<NotificationsPage />} />
        <Route path="/backups" element={<BackupsPage />} />
        <Route path="/environments" element={<EnvironmentsPage />} />
        <Route path="/generator" element={<GeneratorPage />} />
        <Route
          path="/users"
          element={
            <RequireRole role="admin">
              <UsersPage />
            </RequireRole>
          }
        />
        <Route path="/404" element={<NotFoundPage />} />
        <Route path="*" element={<Navigate to="/404" replace />} />
      </Route>
    </Routes>
  )
}

export default App
