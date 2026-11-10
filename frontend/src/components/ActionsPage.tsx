
import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import {
  Activity,
  CheckCircle2,
  Clock3,
  RefreshCw,
  Search,
  ShieldCheck,
  XCircle,
  AlertTriangle,
} from 'lucide-react'

import type { IncidentAction } from '../types'

import {
  fetchIncidents,
  fetchIncidentActions,
  approveAction,
  rejectAction,
  reconcileAction,
} from '../services/app'

type ActionWithIncident = IncidentAction & {
  incident_service: string
  incident_severity: string
}

type ReconciliationDecision =
  | 'CONFIRMED_SUCCESS'
  | 'CONFIRMED_NOT_PERFORMED'
  | 'UNKNOWN'

function ActionsPage() {
  const [actions, setActions] = useState<ActionWithIncident[]>([])
  const [loading, setLoading] = useState(true)
  const [savingId, setSavingId] = useState<number | null>(null)
  const [error, setError] = useState('')

  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('ALL')
  const [typeFilter, setTypeFilter] = useState('ALL')

  const fetchAllActions = useCallback(async (): Promise<ActionWithIncident[]> => {
    const incidents = await fetchIncidents()

    const results = await Promise.all(
      incidents.map(async (incident) => {
        const incidentActions = await fetchIncidentActions(incident.id)

        return (incidentActions ?? []).map((action) => ({
          ...action,
          incident_service: incident.service,
          incident_severity: incident.severity,
        }))
      }),
    )

    return results.flat().sort(
      (a, b) =>
        new Date(b.created_at).getTime() -
        new Date(a.created_at).getTime(),
    )
  }, [])

  // Initial data loading
  useEffect(() => {
    let cancelled = false

    fetchAllActions()
      .then((data) => {
        if (!cancelled) {
          setActions(data)
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(
            err instanceof Error
              ? err.message
              : 'Unable to load actions.',
          )
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false)
        }
      })

    return () => {
      cancelled = true
    }
  }, [fetchAllActions])

  // Manual refresh and post-mutation refresh
  async function loadActions() {
    setLoading(true)
    setError('')

    try {
      const data = await fetchAllActions()
      setActions(data)
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : 'Unable to load actions.',
      )
    } finally {
      setLoading(false)
    }
  }


  // --------------------------------------------------
  // ACTION OPERATIONS
  // --------------------------------------------------

  async function executeOperation(
    actionId: number,
    operation: () => Promise<void>,
  ) {
    setSavingId(actionId)
    setError('')

    try {
      await operation()
      await loadActions()
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Action operation failed.',
      )
    } finally {
      setSavingId(null)
    }
  }

  function handleApprove(action: ActionWithIncident) {
    const confirmed = window.confirm(
      `Approve action #${action.id}?\n\n${action.description}`,
    )

    if (!confirmed) return

    void executeOperation(action.id, () => approveAction(action.id))
  }

  function handleReject(action: ActionWithIncident) {
    const reason = window.prompt(
      `Enter the reason for rejecting action #${action.id}:`,
    )

    if (!reason?.trim()) return

    void executeOperation(action.id, () =>
      rejectAction(action.id, reason.trim()),
    )
  }

  function handleReconcile(
    action: ActionWithIncident,
    decision: ReconciliationDecision,
  ) {
    const reason = window.prompt(
      `Explain the reconciliation decision for action #${action.id}:`,
    )

    if (!reason?.trim()) return

    const labels: Record<ReconciliationDecision, string> = {
      CONFIRMED_SUCCESS: 'Confirm successful execution',
      CONFIRMED_NOT_PERFORMED: 'Confirm operation was not performed',
      UNKNOWN: 'Mark outcome as unknown',
    }

    const confirmed = window.confirm(
      `${labels[decision]}?\n\n${action.description}`,
    )

    if (!confirmed) return

    void executeOperation(action.id, () =>
      reconcileAction(action.id, decision, reason.trim()),
    )
  }

  // --------------------------------------------------
  // FILTERING
  // --------------------------------------------------

  const filteredActions = useMemo(() => {
    const term = search.toLowerCase()

    return actions.filter((action) => {
      const matchesStatus =
        statusFilter === 'ALL' || action.status === statusFilter

      const matchesType =
        typeFilter === 'ALL' || action.action_type === typeFilter

      const matchesSearch =
        action.description.toLowerCase().includes(term) ||
        action.incident_service.toLowerCase().includes(term) ||
        String(action.id).includes(term) ||
        String(action.incident_id).includes(term)

      return matchesStatus && matchesType && matchesSearch
    })
  }, [actions, search, statusFilter, typeFilter])

  // --------------------------------------------------
  // METRICS
  // --------------------------------------------------

  const total = actions.length

  const pendingApproval = actions.filter(
    (action) =>
      action.status === 'PENDING_APPROVAL' ||
      action.status === 'PROPOSED' && action.requires_approval,
  ).length

  const approved = actions.filter(
    (action) => action.status === 'APPROVED',
  ).length

  const executed = actions.filter(
    (action) => action.status === 'EXECUTED',
  ).length

  const attentionRequired = actions.filter(
    (action) =>
      action.status === 'FAILED' ||
      action.status === 'RECONCILIATION_REQUIRED',
  ).length

  return (
    <>
      {/* PAGE HEADING */}

      <div className="page-heading">
        <div>
          <div className="eyebrow">OPERATIONAL CONTROL</div>
          <h2>Action Management</h2>
          <p>
            Review recommendations, manage approvals and track execution.
          </p>
        </div>

        <button
          type="button"
          className="filter-button"
          onClick={() => void loadActions()}
          disabled={loading}
        >
          <RefreshCw size={15} />
          Refresh
        </button>
      </div>

      {error && (
        <div className="api-error">
          {error}
        </div>
      )}

      {/* METRICS */}

      <section className="metrics-grid">

        <ActionMetric
          label="Total Actions"
          value={loading ? '—' : total}
          icon={<Activity size={18} />}
        />

        <ActionMetric
          label="Pending Approval"
          value={loading ? '—' : pendingApproval}
          icon={<Clock3 size={18} />}
          variant="warning"
        />

        <ActionMetric
          label="Approved"
          value={loading ? '—' : approved}
          icon={<ShieldCheck size={18} />}
        />

        <ActionMetric
          label="Executed"
          value={loading ? '—' : executed}
          icon={<CheckCircle2 size={18} />}
          variant="success"
        />

      </section>

      {/* ATTENTION NOTICE */}

      {attentionRequired > 0 && (
        <div className="action-warning">
          <AlertTriangle size={17} />
          <span>
            {attentionRequired} actions require attention due to failure or
            unresolved execution outcomes.
          </span>
        </div>
      )}

      {/* ACTION TABLE */}

      <section className="panel incidents-panel action-table-panel">

        <div className="panel-header action-table-header">
          <div>
            <h3>All recommended actions</h3>
            <p>
              {filteredActions.length} of {actions.length} actions
            </p>
          </div>
        </div>

        {/* FILTERS */}

        <div className="action-filters">

          <div className="filter-button action-search">
            <Search size={15} />

            <input
              type="text"
              placeholder="Search actions, services or IDs..."
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </div>

          <select
            className="filter-button"
            value={statusFilter}
            onChange={(event) => setStatusFilter(event.target.value)}
          >
            <option value="ALL">All statuses</option>
            <option value="PROPOSED">Proposed</option>
            <option value="PENDING_APPROVAL">Pending approval</option>
            <option value="APPROVED">Approved</option>
            <option value="REJECTED">Rejected</option>
            <option value="EXECUTING">Executing</option>
            <option value="EXECUTED">Executed</option>
            <option value="FAILED">Failed</option>
            <option value="RECONCILIATION_REQUIRED">
              Reconciliation required
            </option>
          </select>

          <select
            className="filter-button"
            value={typeFilter}
            onChange={(event) => setTypeFilter(event.target.value)}
          >
            <option value="ALL">All types</option>
            <option value="INVESTIGATION">Investigation</option>
            <option value="MITIGATION">Mitigation</option>
          </select>

        </div>

        {/* TABLE */}

        {loading ? (
          <div className="action-empty">
            Loading actions...
          </div>
        ) : filteredActions.length === 0 ? (
          <div className="action-empty">
            No actions match your filters.
          </div>
        ) : (
          <div className="table-wrapper">

            <table className="actions-table">

              <thead>
                <tr>
                  <th>ACTION</th>
                  <th>INCIDENT</th>
                  <th>TYPE</th>
                  <th>STATUS</th>
                  <th>APPROVAL</th>
                  <th>CREATED</th>
                  <th>OPERATIONS</th>
                </tr>
              </thead>

              <tbody>
                {filteredActions.map((action) => {
                  const busy = savingId === action.id

                  const canApprove =
                    action.status === 'PENDING_APPROVAL' ||
                    (
                      action.status === 'PROPOSED' &&
                      action.requires_approval
                    )

                  const canReject =
                    action.status === 'PENDING_APPROVAL' ||
                    action.status === 'PROPOSED'

                  const canReconcile =
                    action.status === 'RECONCILIATION_REQUIRED'

                  return (
                    <tr key={action.id}>

                      <td data-label="Action">
                        <div className="action-description">
                          <strong>ACT-{action.id}</strong>
                          <span>{action.description}</span>
                        </div>
                      </td>

                      <td data-label="Incident">
                        <div className="action-incident">
                          <strong>
                            INC-{String(action.incident_id).padStart(4, '0')}
                          </strong>
                          <span>{action.incident_service}</span>
                        </div>
                      </td>

                      <td data-label="Type">{action.action_type}</td>

                      <td data-label="Status">
                        <ActionStatusBadge status={action.status} />
                      </td>

                      <td data-label="Approval">
                        {action.requires_approval ? 'Required' : 'Not required'}
                      </td>

                      <td data-label="Created">
                        {formatDate(action.created_at)}
                      </td>

                      <td data-label="Operations">
                        <div className="action-controls">

                          {canApprove && (
                            <button
                              type="button"
                              disabled={busy}
                              className="action-control approve"
                              onClick={() => handleApprove(action)}
                              title="Approve action"
                            >
                              <CheckCircle2 size={15} />
                            </button>
                          )}

                          {canReject && (
                            <button
                              type="button"
                              disabled={busy}
                              className="action-control reject"
                              onClick={() => handleReject(action)}
                              title="Reject action"
                            >
                              <XCircle size={15} />
                            </button>
                          )}

                          {canReconcile && (
                            <>
                              <button
                                type="button"
                                disabled={busy}
                                className="action-control approve"
                                onClick={() =>
                                  handleReconcile(action, 'CONFIRMED_SUCCESS')
                                }
                                title="Confirm successful execution"
                              >
                                <CheckCircle2 size={15} />
                              </button>

                              <button
                                type="button"
                                disabled={busy}
                                className="action-control reject"
                                onClick={() =>
                                  handleReconcile(
                                    action,
                                    'CONFIRMED_NOT_PERFORMED',
                                  )
                                }
                                title="Confirm operation was not performed"
                              >
                                <XCircle size={15} />
                              </button>

                              <button
                                type="button"
                                disabled={busy}
                                className="action-control"
                                onClick={() =>
                                  handleReconcile(action, 'UNKNOWN')
                                }
                                title="Outcome unknown"
                              >
                                ?
                              </button>
                            </>
                          )}

                          {busy && <span>Saving...</span>}

                          {!canApprove && !canReject && !canReconcile && (
                            <span className="action-no-operation">—</span>
                          )}

                        </div>
                      </td>

                    </tr>
                  )
                })}
              </tbody>

            </table>

          </div>
        )}

      </section>
    </>
  )
}

// --------------------------------------------------
// ACTION METRIC
// --------------------------------------------------

function ActionMetric({
  label,
  value,
  icon,
  variant = '',
}: {
  label: string
  value: number | string
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
// ACTION STATUS BADGE
// --------------------------------------------------

function ActionStatusBadge({
  status,
}: {
  status: string
}) {
  return (
    <span className={`status-badge ${status.toLowerCase()}`}>
      {status.replaceAll('_', ' ')}
    </span>
  )
}

// --------------------------------------------------
// DATE FORMATTER
// --------------------------------------------------

function formatDate(value: string) {
  return new Date(value).toLocaleString('en-IN', {
    day: '2-digit',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export default ActionsPage
