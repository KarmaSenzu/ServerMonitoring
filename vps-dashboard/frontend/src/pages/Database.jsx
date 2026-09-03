import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { FiDatabase, FiRefreshCw, FiCheckCircle, FiAlertTriangle } from 'react-icons/fi'
import { databaseApi } from '../api/endpoints.js'
import { useAuth } from '../auth/useAuth.js'
import { useToast } from '../ui/useToast.js'
import Spinner from '../ui/Spinner.jsx'
import { describeError } from '../ui/errors.js'
import './Database.css'

const DB_TYPES = [
  { value: 'sqlite', label: 'SQLite (embedded, no setup)' },
  { value: 'postgres', label: 'PostgreSQL (self-hosted)' },
  { value: 'supabase', label: 'Supabase (managed PostgreSQL)' },
]

export default function DatabasePage() {
  const { user } = useAuth()
  const isAdmin = user?.role === 'admin'
  const queryClient = useQueryClient()
  const toast = useToast()

  const [dbType, setDbType] = useState('supabase')
  const [form, setForm] = useState({
    host: '',
    port: 5432,
    database: 'postgres',
    username: '',
    password: '',
    ssl_mode: 'require',
    project_ref: '',
    project_url: '',
  })
  const [testResult, setTestResult] = useState(null)
  const [configured, setConfigured] = useState(null)

  const statusQ = useQuery({
    queryKey: ['database-status'],
    queryFn: databaseApi.status,
  })

  const configQ = useQuery({
    queryKey: ['database-config'],
    queryFn: databaseApi.config,
  })

  const testM = useMutation({
    mutationFn: () => databaseApi.test({ type: dbType, ...form, port: Number(form.port) || undefined }),
    onSuccess: (data) => {
      setTestResult(data)
      if (data?.ok) {
        toast.push({ type: 'success', message: `Connected in ${data.latency}ms` })
      } else {
        toast.push({ type: 'error', message: data?.error || 'Connection failed' })
      }
    },
    onError: (err) => {
      setTestResult({ ok: false, error: describeError(err, 'Test failed') })
      toast.push({ type: 'error', message: describeError(err, 'Test failed') })
    },
  })

  const configureM = useMutation({
    mutationFn: () => databaseApi.configure({ type: dbType, ...form, port: Number(form.port) || undefined }),
    onSuccess: (data) => {
      setConfigured(data)
      queryClient.invalidateQueries({ queryKey: ['database-status'] })
      queryClient.invalidateQueries({ queryKey: ['database-config'] })
      toast.push({ type: 'success', message: 'Configuration saved. Restart required.' })
    },
    onError: (err) => {
      toast.push({ type: 'error', message: describeError(err, 'Save failed') })
    },
  })

  const set = (key) => (e) => setForm((f) => ({ ...f, [key]: e.target.value }))

  const current = statusQ.data
  const isRemote = current?.type === 'supabase' || current?.type === 'postgres'

  return (
    <div className="database-page">
      <div className="page-header">
        <div>
          <h1>Database</h1>
          <p>Connect the backend to SQLite, PostgreSQL, or Supabase</p>
        </div>
      </div>

      {/* Current status */}
      <div className="db-status-card glass animate-in">
        <div className="db-status-header">
          <span className="db-status-icon"><FiDatabase /></span>
          <div>
            <h3>Current Database</h3>
            {statusQ.isLoading ? (
              <Spinner size={14} />
            ) : (
              <span className="db-status-type">{current?.type || 'unknown'}</span>
            )}
          </div>
        </div>
        {current?.connection && (
          <code className="db-conn-string">{current.connection}</code>
        )}
      </div>

      {/* Configure form — admin only */}
      {isAdmin ? (
        <div className="db-config-card glass animate-in">
          <h3>Switch Database</h3>

          <div className="db-form">
            <div className="form-group">
              <label>Database type</label>
              <select value={dbType} onChange={(e) => setDbType(e.target.value)}>
                {DB_TYPES.map((t) => (
                  <option key={t.value} value={t.value}>{t.label}</option>
                ))}
              </select>
            </div>

            {dbType !== 'sqlite' && (
              <>
                <div className="form-group">
                  <label>Host</label>
                  <input
                    type="text"
                    className="mono"
                    placeholder="aws-0-ap-southeast-1.pooler.supabase.com"
                    value={form.host}
                    onChange={set('host')}
                  />
                </div>

                <div className="db-form-row">
                  <div className="form-group">
                    <label>Port</label>
                    <input
                      type="number"
                      value={form.port}
                      onChange={set('port')}
                      placeholder="5432"
                    />
                  </div>
                  <div className="form-group">
                    <label>Database name</label>
                    <input
                      type="text"
                      value={form.database}
                      onChange={set('database')}
                      placeholder="postgres"
                    />
                  </div>
                </div>

                <div className="form-group">
                  <label>Username</label>
                  <input
                    type="text"
                    className="mono"
                    placeholder="postgres.abcdefghijklm"
                    value={form.username}
                    onChange={set('username')}
                  />
                </div>

                <div className="form-group">
                  <label>Password</label>
                  <input
                    type="password"
                    value={form.password}
                    onChange={set('password')}
                    placeholder="Database password (encrypted at rest)"
                  />
                </div>

                <div className="form-group">
                  <label>SSL mode</label>
                  <select value={form.ssl_mode} onChange={set('ssl_mode')}>
                    <option value="require">require (recommended)</option>
                    <option value="verify-full">verify-full</option>
                    <option value="disable">disable</option>
                  </select>
                </div>

                {dbType === 'supabase' && (
                  <div className="form-group">
                    <label>Project ref (optional)</label>
                    <input
                      type="text"
                      className="mono"
                      placeholder="abcdefghijklm"
                      value={form.project_ref}
                      onChange={set('project_ref')}
                    />
                    <p className="field-help">
                      From https://<code>&lt;ref&gt;</code>.supabase.co — only for future API features.
                    </p>
                  </div>
                )}
              </>
            )}

            <div className="db-form-actions">
              <button
                type="button"
                className="ghost-btn"
                onClick={() => testM.mutate()}
                disabled={testM.isPending || (dbType !== 'sqlite' && (!form.host || !form.username || !form.password))}
              >
                {testM.isPending ? <Spinner size={14} /> : <FiRefreshCw />}
                Test Connection
              </button>
              <button
                type="button"
                className="primary-btn"
                onClick={() => configureM.mutate()}
                disabled={configureM.isPending || (dbType !== 'sqlite' && (!form.host || !form.username || !form.password))}
              >
                {configureM.isPending ? <Spinner size={14} /> : <FiDatabase />}
                Save &amp; Switch
              </button>
            </div>

            {testResult && (
              <div className={`db-test-result ${testResult.ok ? 'ok' : 'fail'}`}>
                {testResult.ok ? <FiCheckCircle /> : <FiAlertTriangle />}
                <span>
                  {testResult.ok
                    ? `Connection successful (${testResult.latency}ms)`
                    : testResult.error || 'Connection failed'}
                </span>
              </div>
            )}

            {configured?.restart_required && (
              <div className="db-restart-notice">
                <FiAlertTriangle />
                <span>
                  Configuration saved. <strong>Restart the server</strong> to apply
                  (run <code>vpsdash restart</code>).
                </span>
              </div>
            )}
          </div>
        </div>
      ) : (
        <div className="db-readonly glass animate-in">
          <p>Only administrators can change the database backend.</p>
        </div>
      )}

      {isRemote && (
        <div className="db-note glass animate-in">
          <p className="field-help">
            <strong>Note:</strong> Switching the database does <em>not</em> migrate
            existing data. Data in the previous database stays where it is.
          </p>
        </div>
      )}
    </div>
  )
}
