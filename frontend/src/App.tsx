
import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'

import {
  Activity,
  AlertCircle,
  AlertTriangle,
  ArrowUpRight,
  BookOpen,
  Cable,
  CheckCircle2,
  Clock3,
  Command,
  HeartPulse,
  LayoutDashboard,
  Menu,
  X,
  Search,
  Siren,
  Zap,
} from 'lucide-react'

import type { Incident, DashboardSummary } from './types'

import {
  fetchIncidents,
  fetchDashboardSummary,
} from './services/app'

import IncidentDetails from './components/IncidentDetails'
import ActionsPage from './components/ActionsPage'
import SystemHealthPage from './components/SystemHealthPage'

import './App.css'
import IntegrationsPage from './components/IntegrationsPage'

// --------------------------------------------------
// NAVIGATION
// --------------------------------------------------

const navigation = [
  { label: 'Overview', icon: LayoutDashboard },
  { label: 'Incidents', icon: AlertCircle },
  { label: 'Actions', icon: Zap },
  { label: 'Integrations', icon: Cable },
  { label: 'Help & Guide', icon: BookOpen },
  { label: 'System Health', icon: HeartPulse },
]

// --------------------------------------------------
// MAIN APPLICATION
// --------------------------------------------------

function App() {
  const [activePage, setActivePage] = useState('Overview')
  const [mobileNavOpen, setMobileNavOpen] = useState(false)

  const [incidents, setIncidents] = useState<Incident[]>([])
  const [summary, setSummary] = useState<DashboardSummary | null>(null)

  const [selectedIncidentId, setSelectedIncidentId] =
    useState<number | null>(null)

  const [loading, setLoading] = useState(true)
  const [incidentsError, setIncidentsError] = useState('')
  const [summaryError, setSummaryError] = useState('')

  // Prefer the API summary. Fall back to the loaded incident records.
  // The badge represents OPEN incidents, not all historical incidents.
  const openIncidentCount =
    summary?.open_incidents ??
    incidents.filter((incident) => incident.status === 'OPEN').length

  // --------------------------------------------------
  // REFRESH DATA
  // --------------------------------------------------

  async function refreshIncidents() {
    const [incidentResult, summaryResult] = await Promise.allSettled([
      fetchIncidents(),
      fetchDashboardSummary(),
    ])

    if (incidentResult.status === 'fulfilled') {
      setIncidents(incidentResult.value)
      setIncidentsError('')
    } else {
      setIncidentsError('Unable to load incidents.')
    }

    if (summaryResult.status === 'fulfilled') {
      setSummary(summaryResult.value)
      setSummaryError('')
    } else {
      setSummaryError('Unable to load dashboard summary.')
    }
  }

  // --------------------------------------------------
  // INITIAL DATA LOAD
  // --------------------------------------------------

  useEffect(() => {
    let cancelled = false

    async function loadDashboard() {
      const [incidentResult, summaryResult] = await Promise.allSettled([
        fetchIncidents(),
        fetchDashboardSummary(),
      ])

      if (cancelled) return

      if (incidentResult.status === 'fulfilled') {
        setIncidents(incidentResult.value)
        setIncidentsError('')
      } else {
        setIncidentsError('Unable to load incidents.')
      }

      if (summaryResult.status === 'fulfilled') {
        setSummary(summaryResult.value)
        setSummaryError('')
      } else {
        setSummaryError('Unable to load dashboard summary.')
      }

      setLoading(false)
    }

    void loadDashboard()

    return () => {
      cancelled = true
    }
  }, [])

  // --------------------------------------------------
  // NAVIGATION
  // --------------------------------------------------

  function navigateTo(page: string) {
    setActivePage(page)
    setSelectedIncidentId(null)
    setMobileNavOpen(false)
  }

  // --------------------------------------------------
  // RENDER
  // --------------------------------------------------

  return (
    <div className="app-shell">

      {mobileNavOpen && (
        <button
          type="button"
          className="mobile-nav-backdrop"
          aria-label="Close navigation"
          onClick={() => setMobileNavOpen(false)}
        />
      )}

      <aside
        id="opspilot-navigation"
        className={`sidebar ${mobileNavOpen ? 'sidebar-open' : ''}`}
      >

        <div className="brand">
          <div className="brand-icon">
            <Activity size={20} />
          </div>

          <div>
            <h1>OpsPilot</h1>
            <span>INCIDENT INTELLIGENCE</span>
          </div>
        </div>

        <div
          className="workspace"
          aria-label="Current workspace: Operations"
        >
          <div className="workspace-avatar">O</div>

          <div className="workspace-info">
            <strong>Operations</strong>
            <span>Incident Management</span>
          </div>
        </div>

        <div className="nav-section">
          <span className="nav-heading">WORKSPACE</span>

          {navigation.map(({ label, icon: Icon }) => (
            <button
              key={label}
              className={`nav-item ${
                activePage === label ? 'active' : ''
              }`}
              onClick={() => navigateTo(label)}
              type="button"
              aria-current={activePage === label ? 'page' : undefined}
            >
              <Icon size={17} />

              <span>{label}</span>

              {label === 'Incidents' && openIncidentCount > 0 && (
                <span
                  className="nav-count"
                  aria-label={`${openIncidentCount} open incidents`}
                  title={`${openIncidentCount} open incidents`}
                >
                  {openIncidentCount}
                </span>
              )}
            </button>
          ))}
        </div>

        <div className="sidebar-bottom">
          <div className="sidebar-footer">
            <span className="status-dot" />

            <div>
              <strong>OpsPilot</strong>
              <span>Incident Intelligence Platform</span>
            </div>
          </div>
        </div>

      </aside>

      <main className="main-content">

        <header className="topbar">
          <button
            type="button"
            className="mobile-nav-toggle"
            aria-label={mobileNavOpen ? 'Close navigation menu' : 'Open navigation menu'}
            aria-expanded={mobileNavOpen}
            aria-controls="opspilot-navigation"
            onClick={() => setMobileNavOpen((open) => !open)}
          >
            {mobileNavOpen ? <X size={20} /> : <Menu size={20} />}
          </button>

          <div className="breadcrumb">
            <span>Workspace</span>
            <span className="breadcrumb-separator">/</span>

            <strong>
              {selectedIncidentId !== null
                ? 'Incident Details'
                : activePage}
            </strong>
          </div>

          <div className="topbar-right">
            <div className="environment-tag">
              <span className="status-dot" />
              DEVELOPMENT
            </div>
          </div>
        </header>

        <div className="page-content">

          {activePage === 'Overview' && (
            <Dashboard
              incidents={incidents}
              summary={summary}
              loading={loading}
              error={summaryError || incidentsError}
              onViewAll={() => navigateTo('Incidents')}
              onSelectIncident={(id) => {
                setSelectedIncidentId(id)
                setActivePage('Incidents')
              }}
            />
          )}

          {activePage === 'Incidents' && (
            selectedIncidentId !== null ? (
              <IncidentDetails
                incidentId={selectedIncidentId}
                onBack={() => setSelectedIncidentId(null)}
                onIncidentUpdated={refreshIncidents}
              />
            ) : (
              <IncidentsPage
                incidents={incidents}
                loading={loading}
                error={incidentsError}
                onSelectIncident={setSelectedIncidentId}
              />
            )
          )}

          {activePage === 'Actions' && <ActionsPage />}

          {activePage === 'Integrations' && <IntegrationsPage />}

          {activePage === 'Help & Guide' && <HelpPage />}

          {activePage === 'System Health' && <SystemHealthPage />}

        </div>
      </main>
    </div>
  )
}

// --------------------------------------------------
// DASHBOARD
// --------------------------------------------------

function Dashboard({
  incidents,
  summary,
  loading,
  error,
  onViewAll,
  onSelectIncident,
}: {
  incidents: Incident[]
  summary: DashboardSummary | null
  loading: boolean
  error: string
  onViewAll: () => void
  onSelectIncident: (id: number) => void
}) {
  const total = summary?.total_incidents ?? 0
  const critical = summary?.critical_incidents ?? 0
  const high = summary?.high_incidents ?? 0
  const medium = summary?.medium_incidents ?? 0
  const low = summary?.low_incidents ?? 0

  const open = summary?.open_incidents ?? 0
  const acknowledged = summary?.acknowledged_incidents ?? 0
  const resolved = summary?.resolved_incidents ?? 0

  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">OPERATIONS CENTER</div>
          <h2>System Overview</h2>
          <p>
            Monitor incidents, investigate failures and manage responses.
          </p>
        </div>
      </div>

      {error && (
        <div className="api-error">
          {error}
        </div>
      )}

      {loading && (
        <div className="panel loading-panel">
          Loading dashboard data...
        </div>
      )}

      <section className="metrics-grid">

        <MetricCard
          label="Total Incidents"
          value={loading || !summary ? '—' : String(total)}
          icon={<Activity size={17} />}
        />

        <MetricCard
          label="Critical Incidents"
          value={loading || !summary ? '—' : String(critical)}
          icon={<Siren size={17} />}
          variant="critical"
        />

        <MetricCard
          label="Open Incidents"
          value={loading || !summary ? '—' : String(open)}
          icon={<AlertTriangle size={17} />}
          variant="warning"
        />

        <MetricCard
          label="Resolved Incidents"
          value={loading || !summary ? '—' : String(resolved)}
          icon={<CheckCircle2 size={17} />}
          variant="success"
        />

      </section>

      <section className="dashboard-grid">

        <div className="panel incident-trend">
          <div className="panel-header">
            <div>
              <h3>Incident status</h3>
              <p>Current operational status distribution</p>
            </div>
          </div>

          <div className="severity-total">
            <strong>{loading || !summary ? '—' : total}</strong>
            <span>Total incidents</span>
          </div>

          <div className="severity-list">
            <DistributionRow
              label="Open"
              value={open}
              total={total}
              color="#e87979"
            />

            <DistributionRow
              label="Acknowledged"
              value={acknowledged}
              total={total}
              color="#e6ad57"
            />

            <DistributionRow
              label="Resolved"
              value={resolved}
              total={total}
              color="#4fc2a1"
            />
          </div>
        </div>

        <div className="panel severity-panel">
          <div className="panel-header">
            <div>
              <h3>Severity breakdown</h3>
              <p>Current incident distribution</p>
            </div>
          </div>

          <div className="severity-total">
            <strong>{loading || !summary ? '—' : total}</strong>
            <span>Total incidents</span>
          </div>

          <div className="severity-list">
            <SeverityRow
              label="Critical"
              value={critical}
              total={total}
              color="critical"
            />

            <SeverityRow
              label="High"
              value={high}
              total={total}
              color="high"
            />

            <SeverityRow
              label="Medium"
              value={medium}
              total={total}
              color="medium"
            />

            <SeverityRow
              label="Low"
              value={low}
              total={total}
              color="low"
            />
          </div>
        </div>

      </section>

      <section className="panel incidents-panel">
        <div className="panel-header">
          <div>
            <h3>Recent incidents</h3>
            <p>Latest incidents retrieved from PostgreSQL</p>
          </div>

          <button
            className="view-all"
            onClick={onViewAll}
            type="button"
          >
            View all incidents
            <ArrowUpRight size={14} />
          </button>
        </div>

        <div className="table-wrapper">
          <table>
            <thead>
              <tr>
                <th>INCIDENT</th>
                <th>SERVICE</th>
                <th>SEVERITY</th>
                <th>STATUS</th>
                <th>DETECTED</th>
              </tr>
            </thead>

            <tbody>
              {!loading && incidents.slice(0, 5).map((incident) => (
                <tr
                  key={incident.id}
                  onClick={() => onSelectIncident(incident.id)}
                  className="clickable-row"
                >
                  <td>
                    <div className="incident-title">
                      <strong>
                        INC-{String(incident.id).padStart(4, '0')}
                      </strong>
                      <span>{incident.event_type}</span>
                    </div>
                  </td>

                  <td>
                    <span className="service-name">
                      {incident.service}
                    </span>
                  </td>

                  <td>
                    <SeverityBadge severity={incident.severity} />
                  </td>

                  <td>
                    <StatusBadge status={incident.status} />
                  </td>

                  <td className="time-cell">
                    <Clock3 size={13} />
                    {formatDate(incident.created_at)}
                  </td>
                </tr>
              ))}

              {!loading && incidents.length === 0 && (
                <tr>
                  <td colSpan={5}>No incidents found.</td>
                </tr>
              )}

              {loading && (
                <tr>
                  <td colSpan={5}>Loading incidents...</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </section>

      <footer className="dashboard-footer">
        <span>
          <Command size={12} />
          OpsPilot Incident Intelligence Platform
        </span>

        <span>PostgreSQL · API data</span>
      </footer>
    </>
  )
}

// --------------------------------------------------
// INCIDENTS PAGE
// --------------------------------------------------

function IncidentsPage({
  incidents,
  loading,
  error,
  onSelectIncident,
}: {
  incidents: Incident[]
  loading: boolean
  error: string
  onSelectIncident: (id: number) => void
}) {
  const [filter, setFilter] = useState('ALL')
  const [search, setSearch] = useState('')

  const filteredIncidents = incidents.filter((incident) => {
    const matchesStatus =
      filter === 'ALL' || incident.status === filter

    const searchTerm = search.toLowerCase()

    const matchesSearch =
      incident.service.toLowerCase().includes(searchTerm) ||
      incident.event_type.toLowerCase().includes(searchTerm) ||
      incident.event_id.toLowerCase().includes(searchTerm) ||
      String(incident.id).includes(searchTerm)

    return matchesStatus && matchesSearch
  })

  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">INCIDENT MANAGEMENT</div>
          <h2>Incidents</h2>
          <p>
            Investigate and manage detected system failures.
          </p>
        </div>
      </div>

      {error && (
        <div className="api-error">
          {error}
        </div>
      )}

      <section className="panel incidents-panel">
        <div className="panel-header incidents-page-header">
          <div>
            <h3>All incidents</h3>
            <p>
              {filteredIncidents.length} of {incidents.length} incidents
            </p>
          </div>

          <select
            value={filter}
            onChange={(event) => setFilter(event.target.value)}
            className="filter-button"
          >
            <option value="ALL">All statuses</option>
            <option value="OPEN">Open</option>
            <option value="ACKNOWLEDGED">Acknowledged</option>
            <option value="RESOLVED">Resolved</option>
          </select>
        </div>

        <div className="incident-search-wrapper">
          <div className="filter-button incident-search">
            <Search size={14} />

            <input
              type="text"
              placeholder="Search by service, event type or ID..."
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </div>
        </div>

        {loading && (
          <p className="table-message">Loading incidents...</p>
        )}

        {!loading && !error && (
          <div className="table-wrapper">
            <table>
              <thead>
                <tr>
                  <th>ID</th>
                  <th>SERVICE</th>
                  <th>EVENT TYPE</th>
                  <th>SEVERITY</th>
                  <th>STATUS</th>
                  <th>CREATED</th>
                </tr>
              </thead>

              <tbody>
                {filteredIncidents.map((incident) => (
                  <tr
                    key={incident.id}
                    onClick={() => onSelectIncident(incident.id)}
                    className="clickable-row"
                  >
                    <td>
                      <strong>
                        INC-{String(incident.id).padStart(4, '0')}
                      </strong>
                    </td>

                    <td>
                      <span className="service-name">
                        {incident.service}
                      </span>
                    </td>

                    <td>{incident.event_type}</td>

                    <td>
                      <SeverityBadge severity={incident.severity} />
                    </td>

                    <td>
                      <StatusBadge status={incident.status} />
                    </td>

                    <td>{formatDate(incident.created_at)}</td>
                  </tr>
                ))}

                {filteredIncidents.length === 0 && (
                  <tr>
                    <td colSpan={6}>No matching incidents found.</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </>
  )
}

// --------------------------------------------------
// METRIC CARD
// --------------------------------------------------

function MetricCard({
  label,
  value,
  icon,
  variant = '',
}: {
  label: string
  value: string
  icon: ReactNode
  variant?: string
}) {
  return (
    <div className="panel metric-card">
      <div className="metric-top">
        <span>{label}</span>

        <div className={`metric-icon ${variant}`}>
          {icon}
        </div>
      </div>

      <div className="metric-value">
        {value}
      </div>

      <div className="metric-bottom">
        <span>From PostgreSQL</span>
      </div>
    </div>
  )
}

// --------------------------------------------------
// DISTRIBUTION ROW
// --------------------------------------------------

function DistributionRow({
  label,
  value,
  total,
  color,
}: {
  label: string
  value: number
  total: number
  color: string
}) {
  const percentage = total > 0 ? (value / total) * 100 : 0

  return (
    <div className="severity-row">
      <div className="severity-row-label">
        <span
          className="severity-indicator"
          style={{ background: color }}
        />

        <span>{label}</span>
        <strong>{value}</strong>
      </div>

      <div className="progress-track">
        <div
          className="progress-fill"
          style={{
            width: `${percentage}%`,
            background: color,
          }}
        />
      </div>
    </div>
  )
}

// --------------------------------------------------
// SEVERITY ROW
// --------------------------------------------------

function SeverityRow({
  label,
  value,
  total,
  color,
}: {
  label: string
  value: number
  total: number
  color: string
}) {
  const percentage = total > 0 ? (value / total) * 100 : 0

  return (
    <div className="severity-row">
      <div className="severity-row-label">
        <span className={`severity-indicator ${color}`} />
        <span>{label}</span>
        <strong>{value}</strong>
      </div>

      <div className="progress-track">
        <div
          className={`progress-fill ${color}`}
          style={{ width: `${percentage}%` }}
        />
      </div>
    </div>
  )
}

// --------------------------------------------------
// SEVERITY BADGE
// --------------------------------------------------

function SeverityBadge({ severity }: { severity: string }) {
  const normalized = severity.toLowerCase()

  return (
    <span className={`severity-badge ${normalized}`}>
      <span />
      {severity}
    </span>
  )
}

// --------------------------------------------------
// STATUS BADGE
// --------------------------------------------------

function StatusBadge({ status }: { status: string }) {
  const normalized = status.toLowerCase()

  return (
    <span className={`status-badge ${normalized}`}>
      {status.replace('_', ' ')}
    </span>
  )
}

// --------------------------------------------------
// HELP & PLATFORM GUIDE
// --------------------------------------------------

function HelpPage() {
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">DOCUMENTATION</div>
          <h2>OpsPilot Help & Guide</h2>
          <p>
            Understand the platform, incident lifecycle, architecture and
            operational workflows.
          </p>
        </div>

        <span className="demo-tag">MVP GUIDE</span>
      </div>

      <section className="help-section panel">
        <h3>1. What is OpsPilot?</h3>
        <p>
          OpsPilot is an AI-assisted incident intelligence and response
          platform. It receives system events, identifies potential incidents,
          collects related evidence and can provide operational
          recommendations.
        </p>
        <p>
          Its goal is to help engineering teams understand failures faster and
          manage incident response through a controlled, auditable workflow.
        </p>
      </section>

      <section className="help-section panel">
        <h3>2. How the system works</h3>

        <div className="help-flow">
          <div>
            <strong>01. Event ingestion</strong>
            <p>Applications send events to the Go API.</p>
          </div>

          <div>
            <strong>02. Asynchronous processing</strong>
            <p>Events are published and processed through Kafka.</p>
          </div>

          <div>
            <strong>03. Incident detection</strong>
            <p>
              Related events are evaluated against the incident detection
              rules.
            </p>
          </div>

          <div>
            <strong>04. Evidence and AI analysis</strong>
            <p>Relevant context is collected and AI can assist with analysis.</p>
          </div>

          <div>
            <strong>05. Action recommendations</strong>
            <p>
              Investigation and mitigation recommendations can be recorded.
            </p>
          </div>

          <div>
            <strong>06. Human review</strong>
            <p>
              Operators review recommendations and approve or reject actions
              where required.
            </p>
          </div>

          <div>
            <strong>07. Execution reconciliation</strong>
            <p>
              Uncertain execution outcomes are surfaced for operator
              reconciliation.
            </p>
          </div>
        </div>
      </section>

      <section className="help-section panel">
        <h3>3. Understanding the dashboard</h3>
        <ul>
          <li>
            <strong>Total Incidents:</strong> Number of incidents recorded.
          </li>
          <li>
            <strong>Critical Incidents:</strong> Incidents classified as
            critical.
          </li>
          <li>
            <strong>Open Incidents:</strong> Incidents awaiting acknowledgement
            or resolution.
          </li>
          <li>
            <strong>Resolved Incidents:</strong> Incidents marked as resolved.
          </li>
          <li>
            <strong>Incident Status:</strong> Distribution across open,
            acknowledged and resolved states.
          </li>
          <li>
            <strong>Severity Breakdown:</strong> Distribution across critical,
            high, medium and low severity.
          </li>
        </ul>
      </section>

      <section className="help-section panel">
        <h3>4. Incident management</h3>
        <p>
          The Incidents page lists detected incidents. Select an incident to
          inspect its details, related events and recommended actions.
        </p>

        <ul>
          <li>
            <strong>Open:</strong> The incident needs operator attention.
          </li>
          <li>
            <strong>Acknowledged:</strong> An operator has acknowledged the
            incident.
          </li>
          <li>
            <strong>Resolved:</strong> The incident has been marked as resolved.
          </li>
        </ul>

        <p>
          Acknowledging an incident records that it has been noticed. Resolving
          it records that the incident is considered handled. These status
          changes do not independently prove that an underlying service has
          recovered.
        </p>
      </section>

      <section className="help-section panel">
        <h3>5. Action management</h3>
        <p>
          Actions are recommendations associated with incidents. They are not
          all automatically executed.
        </p>

        <ul>
          <li>
            <strong>Investigation:</strong> An action intended to gather
            information or inspect a problem.
          </li>
          <li>
            <strong>Mitigation:</strong> An action intended to reduce or
            contain the impact of an incident.
          </li>
          <li>
            <strong>Approval required:</strong> The action requires operator
            approval before execution is allowed.
          </li>
          <li>
            <strong>Approved:</strong> The action has been approved. Approval
            does not mean it has executed.
          </li>
          <li>
            <strong>Executed:</strong> The action is recorded as executed.
          </li>
          <li>
            <strong>Failed:</strong> An execution attempt failed.
          </li>
          <li>
            <strong>Reconciliation required:</strong> The execution outcome
            needs operator review.
          </li>
          <li>
            <strong>Rejected:</strong> The recommendation was rejected.
          </li>
        </ul>
      </section>

      <section className="help-section panel">
        <h3>6. Execution reconciliation</h3>
        <p>
          Reconciliation is used when an action's execution outcome cannot be
          reliably determined.
        </p>

        <ul>
          <li>
            <strong>Confirmed success:</strong> Evidence confirms the operation
            succeeded.
          </li>
          <li>
            <strong>Confirmed not performed:</strong> Evidence confirms the
            operation did not happen.
          </li>
          <li>
            <strong>Unknown:</strong> There is not enough evidence to establish
            the outcome.
          </li>
        </ul>

        <p>
          Use available execution evidence before making a reconciliation
          decision. Do not treat approval as execution.
        </p>
      </section>

      <section className="help-section panel">
        <h3>7. System architecture</h3>

        <div className="help-architecture">
          <div>
            <strong>React + Vite</strong>
            <span>Operator dashboard</span>
          </div>

          <div>
            <strong>Go API</strong>
            <span>HTTP endpoints and application logic</span>
          </div>

          <div>
            <strong>Kafka</strong>
            <span>Asynchronous event processing</span>
          </div>

          <div>
            <strong>PostgreSQL + pgvector</strong>
            <span>Persistent records and vector storage</span>
          </div>

          <div>
            <strong>Redis</strong>
            <span>Fast state and incident-window processing</span>
          </div>

          <div>
            <strong>Ollama</strong>
            <span>Local language and embedding models</span>
          </div>
        </div>
      </section>

      <section className="help-section panel">
        <h3>8. Important operational principles</h3>

        <ul>
          <li>Event ingestion and incident detection are separate stages.</li>
          <li>
            Incident acknowledgement and resolution are operator status changes.
          </li>
          <li>
            AI recommendations are decision support, not guaranteed diagnoses.
          </li>
          <li>Approval is not the same as execution.</li>
          <li>
            Uncertain execution outcomes must be reconciled using evidence.
          </li>
          <li>
            Idempotency helps prevent duplicate processing of repeated
            requests.
          </li>
        </ul>
      </section>

      <section className="help-section panel">
        <h3>9. MVP scope and limitations</h3>

        <p>
          This MVP demonstrates event ingestion, incident processing, incident
          management, AI-assisted analysis and controlled action workflows.
        </p>

        <p>
          It is not yet a complete production monitoring product. Authentication,
          role-based access control, configurable workspaces, notification
          delivery and production-grade service health monitoring should not
          be assumed to exist unless separately implemented.
        </p>
      </section>
    </>
  )
}

// --------------------------------------------------
// DATE FORMATTER
// --------------------------------------------------

function formatDate(value: string): string {
  return new Date(value).toLocaleString('en-IN', {
    day: '2-digit',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export default App
