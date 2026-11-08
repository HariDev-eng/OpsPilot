
import { useCallback, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import {
  Activity,
  Check,
  Copy,
  KeyRound,
  Plus,
  RefreshCw,
  Search,
  ShieldCheck,
  ShieldX,
  X,
} from 'lucide-react'

import type {
  Integration,
  CreateIntegrationResponse,
} from '../types'

import {
  fetchIntegrations,
  createIntegration,
  revokeIntegration,
} from '../services/app'

import './IntegrationsPage.css'

export default function IntegrationsPage() {
  const [operatorSecret, setOperatorSecret] = useState('')
  const [secretInput, setSecretInput] = useState('')

  const [integrations, setIntegrations] = useState<Integration[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const [search, setSearch] = useState('')
  const [filter, setFilter] = useState('ALL')

  const [showCreate, setShowCreate] = useState(false)
  const [name, setName] = useState('')
  const [serviceName, setServiceName] = useState('')
  const [creating, setCreating] = useState(false)

  const [createdResult, setCreatedResult] =
    useState<CreateIntegrationResponse | null>(null)

  const [copied, setCopied] = useState(false)
  const [revokingId, setRevokingId] = useState<string | null>(null)

  const loadIntegrations = useCallback(async (secret: string) => {
    setLoading(true)
    setError('')

    try {
      const data = await fetchIntegrations(secret)
      setIntegrations(data)
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : 'Unable to load integrations.',
      )
    } finally {
      setLoading(false)
    }
  }, [])

  const activeCount = integrations.filter(
    (item) => !item.revoked_at,
  ).length

  const revokedCount = integrations.length - activeCount

  const filteredIntegrations = useMemo(() => {
    const term = search.toLowerCase()

    return integrations.filter((item) => {
      const matchesSearch =
        item.name.toLowerCase().includes(term) ||
        item.service_name.toLowerCase().includes(term) ||
        item.key_prefix.toLowerCase().includes(term)

      const matchesStatus =
        filter === 'ALL' ||
        (filter === 'ACTIVE' && !item.revoked_at) ||
        (filter === 'REVOKED' && Boolean(item.revoked_at))

      return matchesSearch && matchesStatus
    })
  }, [integrations, search, filter])

  async function connectOperator(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()

    const secret = secretInput.trim()

    if (!secret) {
      setError('Enter the operator secret.')
      return
    }

    setOperatorSecret(secret)
    await loadIntegrations(secret)
  }

  async function handleCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()

    if (!name.trim() || !serviceName.trim()) {
      setError('Integration name and service name are required.')
      return
    }

    setCreating(true)
    setError('')

    try {
      const result = await createIntegration(
        operatorSecret,
        name.trim(),
        serviceName.trim(),
      )

      setCreatedResult(result)
      setCopied(false)
      setName('')
      setServiceName('')
      setShowCreate(false)

      await loadIntegrations(operatorSecret)
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : 'Unable to create integration.',
      )
    } finally {
      setCreating(false)
    }
  }

  async function handleCopyKey() {
    if (!createdResult?.api_key) return

    try {
      await navigator.clipboard.writeText(createdResult.api_key)
      setCopied(true)
    } catch {
      setError('Clipboard access failed. Select and copy the key manually.')
    }
  }

  async function handleRevoke(integration: Integration) {
    if (integration.revoked_at) return

    const confirmed = window.confirm(
      `Revoke integration "${integration.name}"? Its API key will stop working.`,
    )

    if (!confirmed) return

    setRevokingId(integration.id)
    setError('')

    try {
      await revokeIntegration(operatorSecret, integration.id)
      await loadIntegrations(operatorSecret)
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : 'Unable to revoke integration.',
      )
    } finally {
      setRevokingId(null)
    }
  }

  function disconnectOperator() {
    setOperatorSecret('')
    setSecretInput('')
    setIntegrations([])
    setError('')
    setCreatedResult(null)
    setSearch('')
    setFilter('ALL')
  }

  function formatDate(value: string | null) {
    if (!value) return 'Never'

    return new Date(value).toLocaleString('en-IN', {
      day: '2-digit',
      month: 'short',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    })
  }

  return (
    <div className="integrations-page">
      <div className="page-heading">
        <div>
          <div className="eyebrow">DEVELOPER PLATFORM</div>
          <h2>Integrations</h2>
          <p>
            Register external services and manage their event-ingestion
            credentials.
          </p>
        </div>

        {operatorSecret && (
          <div className="integration-heading-actions">
            <button
              className="integration-secondary-button"
              onClick={() => void loadIntegrations(operatorSecret)}
              disabled={loading}
              type="button"
            >
              <RefreshCw size={15} />
              Refresh
            </button>

            <button
              className="integration-primary-button"
              onClick={() => setShowCreate(true)}
              type="button"
            >
              <Plus size={16} />
              New integration
            </button>
          </div>
        )}
      </div>

      {error && (
        <div className="integration-error" role="alert">
          <span>{error}</span>
          <button
            onClick={() => setError('')}
            type="button"
            aria-label="Dismiss error"
          >
            <X size={15} />
          </button>
        </div>
      )}

      {!operatorSecret ? (
        <section className="panel integration-auth-panel">
          <div className="integration-auth-icon">
            <ShieldCheck size={22} />
          </div>

          <h3>Operator authentication</h3>
          <p>
            Enter the operator secret configured on the OpsPilot API
            server to manage integrations.
          </p>

          <form onSubmit={(event) => void connectOperator(event)}>
            <label htmlFor="operator-secret">Operator secret</label>

            <input
              id="operator-secret"
              type="password"
              value={secretInput}
              onChange={(event) => setSecretInput(event.target.value)}
              placeholder="Enter operator secret"
              autoComplete="off"
            />

            <button
              className="integration-primary-button"
              type="submit"
              disabled={loading}
            >
              {loading ? 'Connecting...' : 'Connect'}
            </button>
          </form>

          <div className="integration-security-note">
            This secret is held in page memory only. It is not saved
            to browser storage.
          </div>
        </section>
      ) : (
        <>
          <div className="integration-toolbar">
            <div className="integration-connected">
              <span />
              Operator authenticated
            </div>

            <button
              className="integration-text-button"
              onClick={disconnectOperator}
              type="button"
            >
              Disconnect
            </button>
          </div>

          <section className="integration-metrics">
            <div className="panel integration-metric">
              <div className="integration-metric-top">
                <span>Total integrations</span>
                <Activity size={17} />
              </div>
              <strong>{integrations.length}</strong>
              <small>Registered integrations</small>
            </div>

            <div className="panel integration-metric">
              <div className="integration-metric-top">
                <span>Active</span>
                <ShieldCheck size={17} />
              </div>
              <strong>{activeCount}</strong>
              <small>Credentials not revoked</small>
            </div>

            <div className="panel integration-metric">
              <div className="integration-metric-top">
                <span>Revoked</span>
                <ShieldX size={17} />
              </div>
              <strong>{revokedCount}</strong>
              <small>Disabled integrations</small>
            </div>
          </section>

          <section className="panel integration-table-panel">
            <div className="integration-table-heading">
              <div>
                <h3>Registered integrations</h3>
                <p>
                  {filteredIntegrations.length} of {integrations.length} records
                </p>
              </div>
            </div>

            <div className="integration-filters">
              <div className="integration-search">
                <Search size={16} />
                <input
                  value={search}
                  onChange={(event) => setSearch(event.target.value)}
                  placeholder="Search name, service or key prefix..."
                />
              </div>

              <select
                value={filter}
                onChange={(event) => setFilter(event.target.value)}
                aria-label="Filter integrations by status"
              >
                <option value="ALL">All statuses</option>
                <option value="ACTIVE">Active</option>
                <option value="REVOKED">Revoked</option>
              </select>
            </div>

            {loading ? (
              <div className="integration-empty">
                Loading integrations...
              </div>
            ) : integrations.length === 0 ? (
              <div className="integration-empty">
                <KeyRound size={25} />
                <strong>No integrations yet</strong>
                <span>
                  Create an integration to connect an external service.
                </span>
                <button
                  className="integration-primary-button"
                  onClick={() => setShowCreate(true)}
                  type="button"
                >
                  <Plus size={15} />
                  Create integration
                </button>
              </div>
            ) : filteredIntegrations.length === 0 ? (
              <div className="integration-empty">
                No integrations match your search.
              </div>
            ) : (
              <div className="table-wrapper">
                <table className="integration-table">
                  <thead>
                    <tr>
                      <th>INTEGRATION</th>
                      <th>KEY PREFIX</th>
                      <th>CREATED</th>
                      <th>LAST USED</th>
                      <th>STATUS</th>
                      <th>ACTION</th>
                    </tr>
                  </thead>

                  <tbody>
                    {filteredIntegrations.map((integration) => (
                      <tr key={integration.id}>
                        <td>
                          <div className="integration-name-cell">
                            <strong>{integration.name}</strong>
                            <span>{integration.service_name}</span>
                          </div>
                        </td>

                        <td>
                          <code>{integration.key_prefix}</code>
                        </td>

                        <td>{formatDate(integration.created_at)}</td>
                        <td>{formatDate(integration.last_used_at)}</td>

                        <td>
                          <span
                            className={`integration-status ${
                              integration.revoked_at
                                ? 'revoked'
                                : 'active'
                            }`}
                          >
                            <span />
                            {integration.revoked_at ? 'Revoked' : 'Active'}
                          </span>
                        </td>

                        <td>
                          {integration.revoked_at ? (
                            <span className="integration-disabled-action">
                              Revoked {formatDate(integration.revoked_at)}
                            </span>
                          ) : (
                            <button
                              className="integration-revoke-button"
                              onClick={() => void handleRevoke(integration)}
                              disabled={revokingId === integration.id}
                              type="button"
                            >
                              {revokingId === integration.id
                                ? 'Revoking...'
                                : 'Revoke'}
                            </button>
                          )}
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

      {showCreate && (
        <div className="integration-modal-backdrop">
          <section
            className="integration-modal panel"
            role="dialog"
            aria-modal="true"
            aria-labelledby="create-integration-title"
          >
            <div className="integration-modal-heading">
              <div>
                <div className="eyebrow">NEW CONNECTION</div>
                <h3 id="create-integration-title">Create integration</h3>
                <p>Register an external service with OpsPilot.</p>
              </div>

              <button
                className="integration-icon-button"
                onClick={() => setShowCreate(false)}
                type="button"
                aria-label="Close"
              >
                <X size={18} />
              </button>
            </div>

            <form onSubmit={(event) => void handleCreate(event)}>
              <label htmlFor="integration-name">Integration name</label>
              <input
                id="integration-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="e.g. Payment Gateway"
                maxLength={120}
                required
              />

              <label htmlFor="integration-service">Service name</label>
              <input
                id="integration-service"
                value={serviceName}
                onChange={(event) => setServiceName(event.target.value)}
                placeholder="e.g. payment-service"
                maxLength={120}
                required
              />

              <div className="integration-modal-note">
                A new API key will be generated. The full key is shown
                only once after creation.
              </div>

              <div className="integration-modal-actions">
                <button
                  className="integration-secondary-button"
                  onClick={() => setShowCreate(false)}
                  type="button"
                >
                  Cancel
                </button>

                <button
                  className="integration-primary-button"
                  disabled={creating}
                  type="submit"
                >
                  {creating ? 'Creating...' : 'Create integration'}
                </button>
              </div>
            </form>
          </section>
        </div>
      )}

      {createdResult && (
        <div className="integration-modal-backdrop">
          <section
            className="integration-modal panel"
            role="dialog"
            aria-modal="true"
            aria-labelledby="integration-key-title"
          >
            <div className="integration-key-success">
              <div className="integration-key-icon">
                <Check size={22} />
              </div>

              <h3 id="integration-key-title">Integration created</h3>
              <p>
                {createdResult.integration.name} is registered successfully.
              </p>
            </div>

            <div className="integration-warning">
              <ShieldX size={17} />
              Copy this API key now. It cannot be retrieved again.
            </div>

            <label htmlFor="generated-api-key">API key</label>

            <div className="integration-generated-key">
              <input
                id="generated-api-key"
                value={createdResult.api_key}
                readOnly
                onFocus={(event) => event.target.select()}
              />

              <button
                onClick={() => void handleCopyKey()}
                type="button"
                aria-label="Copy API key"
              >
                {copied ? <Check size={17} /> : <Copy size={17} />}
                {copied ? 'Copied' : 'Copy'}
              </button>
            </div>

            <div className="integration-modal-note">
              Store this key in your external service's secret manager.
              Never commit it to source control.
            </div>

            <button
              className="integration-primary-button integration-done-button"
              onClick={() => {
                setCreatedResult(null)
                setCopied(false)
              }}
              type="button"
            >
              Done
            </button>
          </section>
        </div>
      )}
    </div>
  )
}
