
import { useCallback, useEffect, useState } from 'react'
import { Activity, Database, Radio, Server, RefreshCw } from 'lucide-react'
import { fetchSystemHealth } from '../services/app'
import type { SystemHealth, HealthStatus } from '../types'

const components = [
  { key: 'postgres', name: 'PostgreSQL', description: 'Database connectivity', icon: Database },
  { key: 'redis', name: 'Redis', description: 'Cache and incident-window state', icon: Server },
  { key: 'kafka', name: 'Kafka', description: 'Message broker connectivity', icon: Radio },
] as const

function StatusBadge({ status }: { status: HealthStatus }) {
  return (
    <span className={`health-status ${status}`}>
      <span className="health-status-dot" />
      {status.toUpperCase()}
    </span>
  )
}

export default function SystemHealthPage() {
  const [health, setHealth] = useState<SystemHealth | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [lastChecked, setLastChecked] = useState('')

  const loadHealth = useCallback(async () => {
    try {
      const result = await fetchSystemHealth()
      setHealth(result)
      setError('')
      setLastChecked(result.timestamp)
    } catch {
      setError('Unable to retrieve system health. The API may be unavailable.')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    let cancelled = false

    const load = async () => {
      try {
        const result = await fetchSystemHealth()
        if (!cancelled) {
          setHealth(result)
          setError('')
          setLastChecked(result.timestamp)
        }
      } catch {
        if (!cancelled) {
          setError('Unable to retrieve system health. The API may be unavailable.')
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void load()
    const interval = window.setInterval(() => void load(), 15000)

    return () => {
      cancelled = true
      window.clearInterval(interval)
    }
  }, [])

  const overallStatus = health?.status ?? 'unknown'

  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">INFRASTRUCTURE</div>
          <h2>System Health</h2>
          <p>Live connectivity checks for OpsPilot dependencies.</p>
        </div>

        <button
          className="filter-button"
          type="button"
          onClick={() => {
            setLoading(true)
            void loadHealth()
          }}
          disabled={loading}
        >
          <RefreshCw size={16} />
          Refresh
        </button>
      </div>

      <section className="panel health-overview">
        <div>
          <span className="health-label">OVERALL READINESS</span>
          <h3><StatusBadge status={overallStatus} /></h3>
          <p>
            {health
              ? 'Latest dependency readiness report'
              : 'Waiting for health information'}
          </p>
        </div>
        <Activity size={30} />
      </section>

      {error && <div className="api-error">{error}</div>}

      <section className="health-grid">
        {components.map(({ key, name, description, icon: Icon }) => {
          const status = health?.components[key]?.status ?? 'unknown'

          return (
            <article className="panel health-card" key={key}>
              <div className="health-card-heading">
                <div className="health-icon"><Icon size={21} /></div>
                <StatusBadge status={status} />
              </div>
              <h3>{name}</h3>
              <p>{description}</p>
              <div className="health-card-footer">
                Dependency connectivity
              </div>
            </article>
          )
        })}
      </section>

      <section className="panel health-meta">
        <h3>Health check information</h3>
        <p>
          Last checked:{' '}
          {lastChecked ? new Date(lastChecked).toLocaleString() : 'Not available'}
        </p>
        <p>Automatic refresh interval: 15 seconds</p>
        <p>Health endpoint: GET /api/v1/health</p>
        {loading && <p>Checking dependencies...</p>}
      </section>
    </>
  )
}
