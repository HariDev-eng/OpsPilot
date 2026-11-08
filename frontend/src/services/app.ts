import type {
  Incident,
  DashboardSummary,
  IncidentEvent,
  IncidentAction,
} from '../types'
import type {
  Integration,
  CreateIntegrationResponse,
  IntegrationListResponse,
} from '../types'

import type { SystemHealth } from '../types'

const API_BASE_URL = (
  import.meta.env.VITE_API_BASE_URL || '/api/v1'
).replace(/\/$/, '')

async function request<T>(path: string): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`)

  if (!response.ok) {
    const message = await response.text()
    throw new Error(message || `API request failed: ${response.status}`)
  }

  return response.json() as Promise<T>
}

// Dashboard
export function fetchDashboardSummary(): Promise<DashboardSummary> {
  return request<DashboardSummary>('/dashboard/summary')
}

// Incidents
export function fetchIncidents(): Promise<Incident[]> {
  return request<Incident[]>('/incidents?limit=100')
}

export function fetchIncident(id: number): Promise<Incident> {
  return request<Incident>(`/incidents/${id}`)
}

// Incident events
export function fetchIncidentEvents(
  incidentId: number,
): Promise<IncidentEvent[]> {
  return request<IncidentEvent[]>(`/incidents/${incidentId}/events`)
}

// Incident actions
export function fetchIncidentActions(
  incidentId: number,
): Promise<IncidentAction[]> {
  return request<IncidentAction[]>(`/incidents/${incidentId}/actions`)
}


async function postWithoutBody(path: string): Promise<void> {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    method: 'POST',
  })

  if (!response.ok) {
    const message = await response.text()
    throw new Error(message || `Request failed: ${response.status}`)
  }
}

export function acknowledgeIncident(id: number): Promise<void> {
  return postWithoutBody(`/incidents/${id}/acknowledge`)
}

export function resolveIncident(id: number): Promise<void> {
  return postWithoutBody(`/incidents/${id}/resolve`)
}


async function postRequest<T>(
  path: string,
  body?: Record<string, unknown>,
): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    ...(body ? { body: JSON.stringify(body) } : {}),
  })

  if (!response.ok) {
    const message = await response.text()
    throw new Error(message || `Request failed: ${response.status}`)
  }

  if (response.status === 204) {
    return undefined as T
  }

  return response.json() as Promise<T>
}

export function approveAction(id: number): Promise<void> {
  return postRequest<void>(`/actions/${id}/approve`)
}

export function rejectAction(
  id: number,
  reason: string,
): Promise<void> {
  return postRequest<void>(`/actions/${id}/reject`, { reason })
}

export function reconcileAction(
  id: number,
  decision: 'CONFIRMED_SUCCESS' | 'CONFIRMED_NOT_PERFORMED' | 'UNKNOWN',
  reason: string,
): Promise<void> {
  return postRequest<void>(`/actions/${id}/reconcile`, {
    decision,
    reason,
  })
}


export async function fetchSystemHealth(): Promise<SystemHealth> {
  const response = await fetch(`${API_BASE_URL}/health`)

  if (!response.ok) {
    throw new Error(`Health check failed: HTTP ${response.status}`)
  }

  return response.json() as Promise<SystemHealth>
}


const INTEGRATIONS_API = `${API_BASE_URL}/integrations`

async function integrationRequest<T>(
  path: string,
  operatorSecret: string,
  options: RequestInit = {},
): Promise<T> {
  const response = await fetch(`${INTEGRATIONS_API}${path}`, {
    ...options,
    headers: {
      Authorization: `Bearer ${operatorSecret}`,
      ...(options.body ? { 'Content-Type': 'application/json' } : {}),
      ...options.headers,
    },
  })

  if (!response.ok) {
    const message = await response.text()
    throw new Error(message || `Request failed: ${response.status}`)
  }

  if (response.status === 204) {
    return undefined as T
  }

  return response.json() as Promise<T>
}

export async function fetchIntegrations(
  operatorSecret: string,
): Promise<Integration[]> {
  const result = await integrationRequest<IntegrationListResponse>(
    '',
    operatorSecret,
  )

  return result.integrations
}

export function createIntegration(
  operatorSecret: string,
  name: string,
  serviceName: string,
): Promise<CreateIntegrationResponse> {
  return integrationRequest<CreateIntegrationResponse>(
    '',
    operatorSecret,
    {
      method: 'POST',
      body: JSON.stringify({
        name,
        service_name: serviceName,
      }),
    },
  )
}

export function revokeIntegration(
  operatorSecret: string,
  id: string,
): Promise<{ id: string; status: string }> {
  return integrationRequest<{ id: string; status: string }>(
    `/${encodeURIComponent(id)}`,
    operatorSecret,
    { method: 'DELETE' },
  )
}
