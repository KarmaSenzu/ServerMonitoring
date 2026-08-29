import { useState } from 'react'
import { FiBox, FiZap, FiFileText, FiGlobe } from 'react-icons/fi'
import CommandBox from '../components/CommandBox.jsx'
import { generator } from '../api/endpoints.js'
import { useToast } from '../ui/useToast.js'
import { describeError } from '../ui/errors.js'
import './Generator.css'

const TABS = [
  { id: 'docker', label: 'Docker Run', icon: <FiBox /> },
  { id: 'pm2', label: 'PM2', icon: <FiZap /> },
  { id: 'compose', label: 'Compose', icon: <FiFileText /> },
  { id: 'nginx', label: 'Nginx Proxy', icon: <FiGlobe /> },
]

export default function Generator() {
  const toast = useToast()
  const [activeTab, setActiveTab] = useState('docker')
  const [output, setOutput] = useState('')
  const [outputLang, setOutputLang] = useState('bash')
  const [loading, setLoading] = useState(false)

  const [dockerForm, setDockerForm] = useState({
    name: '',
    image: '',
    port: '',
    hostPort: '',
    restart: 'unless-stopped',
    network: '',
  })
  const [pm2Form, setPm2Form] = useState({
    name: '',
    file: '',
    interpreter: '',
    instances: '',
    watch: false,
  })
  const [composeForm, setComposeForm] = useState({
    name: '',
    image: '',
    ports: '',
    restart: 'unless-stopped',
  })
  const [nginxForm, setNginxForm] = useState({
    domain: '',
    proxyPort: '',
    ssl: false,
  })

  const generate = async () => {
    setLoading(true)
    setOutput('')
    try {
      let resp
      let lang = 'bash'
      switch (activeTab) {
        case 'docker': {
          const ports = []
          const host = parseInt(dockerForm.hostPort, 10)
          const container = parseInt(dockerForm.port, 10)
          if (host > 0 && container > 0) {
            ports.push({ host, container })
          }
          resp = await generator.docker({
            name: dockerForm.name,
            image: dockerForm.image,
            restart: dockerForm.restart,
            network: dockerForm.network,
            ports,
          })
          break
        }
        case 'pm2': {
          const instances = parseInt(pm2Form.instances, 10)
          resp = await generator.pm2({
            name: pm2Form.name,
            script: pm2Form.file,
            interpreter: pm2Form.interpreter,
            instances: Number.isFinite(instances) && instances > 0 ? instances : 0,
            watch: pm2Form.watch,
          })
          break
        }
        case 'compose': {
          const ports = composeForm.ports
            ? composeForm.ports
                .split(',')
                .map((p) => p.trim())
                .filter(Boolean)
                .map((p) => {
                  const [h, c] = p.split(':').map((n) => parseInt(n, 10))
                  if (h > 0 && c > 0) return { host: h, container: c }
                  return null
                })
                .filter(Boolean)
            : []
          resp = await generator.compose({
            services: [
              {
                name: composeForm.name,
                image: composeForm.image,
                ports,
                restart: composeForm.restart,
              },
            ],
          })
          lang = 'yaml'
          break
        }
        case 'nginx': {
          const upstream_port = parseInt(nginxForm.proxyPort, 10)
          resp = await generator.nginx({
            domain: nginxForm.domain,
            upstream_host: '127.0.0.1',
            upstream_port: Number.isFinite(upstream_port) ? upstream_port : 0,
            enable_ssl: nginxForm.ssl,
            ssl_cert_path: nginxForm.ssl
              ? `/etc/letsencrypt/live/${nginxForm.domain}/fullchain.pem`
              : '',
            ssl_key_path: nginxForm.ssl
              ? `/etc/letsencrypt/live/${nginxForm.domain}/privkey.pem`
              : '',
          })
          lang = 'nginx'
          break
        }
        default:
          resp = {}
      }
      const text = resp?.command || resp?.yaml || resp?.config || ''
      if (!text) {
        toast.push({ type: 'warning', message: 'Generator returned no output' })
      }
      setOutput(text)
      setOutputLang(lang)
    } catch (err) {
      toast.push({ type: 'error', message: describeError(err, 'Generator failed') })
      setOutput('')
    } finally {
      setLoading(false)
    }
  }

  const renderForm = () => {
    switch (activeTab) {
      case 'docker':
        return (
          <div className="form-grid">
            <div className="form-group">
              <label>Container Name *</label>
              <input
                type="text"
                placeholder="my-app"
                value={dockerForm.name}
                onChange={(e) => setDockerForm({ ...dockerForm, name: e.target.value })}
              />
            </div>
            <div className="form-group">
              <label>Image *</label>
              <input
                type="text"
                placeholder="nginx:latest"
                value={dockerForm.image}
                onChange={(e) => setDockerForm({ ...dockerForm, image: e.target.value })}
              />
            </div>
            <div className="form-group">
              <label>Container Port</label>
              <input
                type="text"
                placeholder="80"
                value={dockerForm.port}
                onChange={(e) => setDockerForm({ ...dockerForm, port: e.target.value })}
              />
            </div>
            <div className="form-group">
              <label>Host Port</label>
              <input
                type="text"
                placeholder="8080"
                value={dockerForm.hostPort}
                onChange={(e) => setDockerForm({ ...dockerForm, hostPort: e.target.value })}
              />
            </div>
            <div className="form-group">
              <label>Restart Policy</label>
              <select
                value={dockerForm.restart}
                onChange={(e) => setDockerForm({ ...dockerForm, restart: e.target.value })}
              >
                <option value="">None</option>
                <option value="always">Always</option>
                <option value="unless-stopped">Unless Stopped</option>
                <option value="on-failure">On Failure</option>
              </select>
            </div>
            <div className="form-group">
              <label>Network</label>
              <input
                type="text"
                placeholder="bridge"
                value={dockerForm.network}
                onChange={(e) => setDockerForm({ ...dockerForm, network: e.target.value })}
              />
            </div>
          </div>
        )
      case 'pm2':
        return (
          <div className="form-grid">
            <div className="form-group">
              <label>App Name *</label>
              <input
                type="text"
                placeholder="my-api"
                value={pm2Form.name}
                onChange={(e) => setPm2Form({ ...pm2Form, name: e.target.value })}
              />
            </div>
            <div className="form-group">
              <label>Entry File *</label>
              <input
                type="text"
                placeholder="server.js"
                value={pm2Form.file}
                onChange={(e) => setPm2Form({ ...pm2Form, file: e.target.value })}
              />
            </div>
            <div className="form-group">
              <label>Interpreter</label>
              <input
                type="text"
                placeholder="node"
                value={pm2Form.interpreter}
                onChange={(e) => setPm2Form({ ...pm2Form, interpreter: e.target.value })}
              />
            </div>
            <div className="form-group">
              <label>Instances</label>
              <input
                type="text"
                placeholder="max"
                value={pm2Form.instances}
                onChange={(e) => setPm2Form({ ...pm2Form, instances: e.target.value })}
              />
            </div>
            <div className="form-group checkbox-group">
              <label>
                <input
                  type="checkbox"
                  checked={pm2Form.watch}
                  onChange={(e) => setPm2Form({ ...pm2Form, watch: e.target.checked })}
                />
                Enable Watch Mode
              </label>
            </div>
          </div>
        )
      case 'compose':
        return (
          <div className="form-grid">
            <div className="form-group">
              <label>Service Name *</label>
              <input
                type="text"
                placeholder="web"
                value={composeForm.name}
                onChange={(e) => setComposeForm({ ...composeForm, name: e.target.value })}
              />
            </div>
            <div className="form-group">
              <label>Image *</label>
              <input
                type="text"
                placeholder="nginx:latest"
                value={composeForm.image}
                onChange={(e) => setComposeForm({ ...composeForm, image: e.target.value })}
              />
            </div>
            <div className="form-group">
              <label>Ports (comma separated)</label>
              <input
                type="text"
                placeholder="8080:80, 443:443"
                value={composeForm.ports}
                onChange={(e) => setComposeForm({ ...composeForm, ports: e.target.value })}
              />
            </div>
            <div className="form-group">
              <label>Restart Policy</label>
              <select
                value={composeForm.restart}
                onChange={(e) => setComposeForm({ ...composeForm, restart: e.target.value })}
              >
                <option value="">None</option>
                <option value="always">Always</option>
                <option value="unless-stopped">Unless Stopped</option>
                <option value="on-failure">On Failure</option>
              </select>
            </div>
          </div>
        )
      case 'nginx':
        return (
          <div className="form-grid">
            <div className="form-group">
              <label>Domain *</label>
              <input
                type="text"
                placeholder="app.example.com"
                value={nginxForm.domain}
                onChange={(e) => setNginxForm({ ...nginxForm, domain: e.target.value })}
              />
            </div>
            <div className="form-group">
              <label>Proxy Port *</label>
              <input
                type="text"
                placeholder="3000"
                value={nginxForm.proxyPort}
                onChange={(e) => setNginxForm({ ...nginxForm, proxyPort: e.target.value })}
              />
            </div>
            <div className="form-group checkbox-group">
              <label>
                <input
                  type="checkbox"
                  checked={nginxForm.ssl}
                  onChange={(e) => setNginxForm({ ...nginxForm, ssl: e.target.checked })}
                />
                Enable SSL (Let&apos;s Encrypt)
              </label>
            </div>
          </div>
        )
      default:
        return null
    }
  }

  return (
    <div className="generator">
      <div className="page-header">
        <h1>Command Generator</h1>
        <p>Generate copy-paste ready commands for your VPS</p>
      </div>

      <div className="gen-tabs glass">
        {TABS.map((tab) => (
          <button
            key={tab.id}
            type="button"
            className={`gen-tab ${activeTab === tab.id ? 'active' : ''}`}
            onClick={() => {
              setActiveTab(tab.id)
              setOutput('')
            }}
          >
            {tab.icon}
            {tab.label}
          </button>
        ))}
      </div>

      <div className="gen-form glass animate-in">
        {renderForm()}
        <button
          type="button"
          className="generate-btn"
          onClick={generate}
          disabled={loading}
        >
          {loading ? 'Generating...' : 'Generate Command'}
        </button>
      </div>

      {output && (
        <CommandBox
          command={output}
          label={`Generated ${activeTab.toUpperCase()} ${outputLang === 'bash' ? 'Command' : 'Output'}`}
        />
      )}
    </div>
  )
}
