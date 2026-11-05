
import { useEffect, useState } from 'react'
import {
  ArrowLeft,
  AlertTriangle,
  CheckCircle2,
  Clock3,
  RefreshCw,
  ShieldCheck,
} from 'lucide-react'

import type {
  Incident,
  IncidentEvent,
  IncidentAction,
} from '../types'

import {
  fetchIncident,
  fetchIncidentEvents,
  fetchIncidentActions,
  acknowledgeIncident,
  resolveIncident,
} from '../services/app'

type Props = {
  incidentId: number
  onBack: () => void
  onIncidentUpdated: () => void | Promise<void>
}

function IncidentDetails({
  incidentId,
  onBack,
  onIncidentUpdated,
}: Props) {
  const [incident, setIncident] = useState<Incident | null>(null)
  const [events, setEvents] = useState<IncidentEvent[]>([])
  const [actions, setActions] = useState<IncidentAction[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')



  async function loadIncident(showLoading = true) {
    if (showLoading) setLoading(true)

    try {
      const [incidentData, eventData, actionData] = await Promise.all([
        fetchIncident(incidentId),
        fetchIncidentEvents(incidentId),
        fetchIncidentActions(incidentId),
      ])

      setIncident(incidentData)
      setEvents(eventData ?? [])
      setActions(actionData ?? [])
      setError('')
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to load incident',
      )
    } finally {
      setLoading(false)
    }
  }

  // Initial load and reload when incidentId changes.
  useEffect(() => {
    let cancelled = false

    Promise.all([
      fetchIncident(incidentId),
      fetchIncidentEvents(incidentId),
      fetchIncidentActions(incidentId),
    ])
      .then(([incidentData, eventData, actionData]) => {
        if (cancelled) return

        setIncident(incidentData)
        setEvents(eventData ?? [])
        setActions(actionData ?? [])
        setError('')
      })
      .catch((err: unknown) => {
        if (cancelled) return

        setError(
          err instanceof Error ? err.message : 'Failed to load incident',
        )
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [incidentId])


  async function updateStatus(
    operation: (id: number) => Promise<void>,
  ) {
    if (!incident) return

    setSaving(true)
    setError('')

    try {
      await operation(incident.id)
      await loadIncident()
      await onIncidentUpdated()
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Unable to update incident',
      )
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="incident-detail-page">
      <button
        type="button"
        className="filter-button"
        onClick={onBack}
        style={{ marginBottom: 20 }}
      >
        <ArrowLeft size={16} />
        Back to incidents
      </button>

      {error && <div className="api-error">{error}</div>}

      {loading && (
        <div className="panel" style={{ padding: 24 }}>
          Loading incident details...
        </div>
      )}

      {!loading && incident && (
        <>
          <div className="page-heading">
            <div>
              <div className="eyebrow">INCIDENT INVESTIGATION</div>
              <h2>
                INC-{String(incident.id).padStart(4, '0')}
              </h2>
              <p>
                {incident.service} · {incident.event_type}
              </p>
            </div>

            <button
              type="button"
              className="filter-button"
              onClick={() => void loadIncident()}
            >
              <RefreshCw size={15} />
              Refresh
            </button>
          </div>

          <section className="panel detail-section">
            <div className="panel-header">
              <div>
                <h3>Incident overview</h3>
                <p>Detection and lifecycle information</p>
              </div>
            </div>

            <div className="detail-grid">
              <DetailField label="Service" value={incident.service} />
              <DetailField label="Event type" value={incident.event_type} />
              <DetailField label="Severity" value={incident.severity} />
              <DetailField label="Status" value={incident.status} />
              <DetailField
                label="Detected"
                value={formatDate(incident.created_at)}
              />
              <DetailField label="Event ID" value={incident.event_id} />
            </div>

            <div className="detail-actions">
              {incident.status === 'OPEN' && (
                <button
                  type="button"
                  disabled={saving}
                  onClick={() => void updateStatus(acknowledgeIncident)}
                >
                  <ShieldCheck size={16} />
                  Acknowledge incident
                </button>
              )}

              {incident.status === 'ACKNOWLEDGED' && (
                <button
                  type="button"
                  disabled={saving}
                  onClick={() => void updateStatus(resolveIncident)}
                >
                  <CheckCircle2 size={16} />
                  Resolve incident
                </button>
              )}

              {incident.status === 'RESOLVED' && (
                <span className="detail-complete">
                  <CheckCircle2 size={16} />
                  Incident resolved
                </span>
              )}
            </div>
          </section>

          <section className="panel detail-section">
            <div className="panel-header">
              <div>
                <h3>Event history</h3>
                <p>{events.length} associated events</p>
              </div>
              <Clock3 size={17} />
            </div>

            {events.length === 0 ? (
              <p className="detail-empty">No associated events found.</p>
            ) : (
              <div className="detail-event-list">
                {events.map((event) => (
                  <div className="detail-event" key={event.event_id}>
                    <div className="detail-event-header">
                      <strong>{event.event_type}</strong>
                      <span>{formatDate(event.timestamp)}</span>
                    </div>
                    <p>{event.event_id}</p>
                    <pre>{JSON.stringify(event.payload, null, 2)}</pre>
                  </div>
                ))}
              </div>
            )}
          </section>

          <section className="panel detail-section">
            <div className="panel-header">
              <div>
                <h3>Recommended actions</h3>
                <p>{actions.length} associated actions</p>
              </div>
              <AlertTriangle size={17} />
            </div>

            {actions.length === 0 ? (
              <p className="detail-empty">
                No recommended actions are available.
              </p>
            ) : (
              <div className="table-wrapper">
                <table>
                  <thead>
                    <tr>
                      <th>ACTION</th>
                      <th>TYPE</th>
                      <th>STATUS</th>
                      <th>APPROVAL</th>
                    </tr>
                  </thead>
                  <tbody>
                    {actions.map((action) => (
                      <tr key={action.id}>
                        <td>{action.description}</td>
                        <td>{action.action_type}</td>
                        <td>{action.status.replaceAll('_', ' ')}</td>
                        <td>
                          {action.requires_approval
                            ? 'Required'
                            : 'Not required'}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>
        </>
      )}
    </div>
  )
}

function DetailField({
  label,
  value,
}: {
  label: string
  value: string
}) {
  return (
    <div className="detail-field">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  )
}

function formatDate(value: string) {
  return new Date(value).toLocaleString('en-IN', {
    dateStyle: 'medium',
    timeStyle: 'short',
  })
}

export default IncidentDetails
