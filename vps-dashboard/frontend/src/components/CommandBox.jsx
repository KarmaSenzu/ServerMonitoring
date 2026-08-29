import { useState } from 'react'
import { FiCopy, FiCheck } from 'react-icons/fi'
import './CommandBox.css'

export default function CommandBox({ command, label }) {
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(command)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // Fallback
      const textarea = document.createElement('textarea')
      textarea.value = command
      document.body.appendChild(textarea)
      textarea.select()
      document.execCommand('copy')
      document.body.removeChild(textarea)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    }
  }

  if (!command) return null

  return (
    <div className="command-box glass animate-in">
      {label && <div className="command-label">{label}</div>}
      <div className="command-content">
        <pre className="command-text">{command}</pre>
        <button
          className={`copy-btn ${copied ? 'copied' : ''}`}
          onClick={handleCopy}
          title="Copy to clipboard"
        >
          {copied ? <FiCheck /> : <FiCopy />}
          {copied ? 'Copied!' : 'Copy'}
        </button>
      </div>
    </div>
  )
}
