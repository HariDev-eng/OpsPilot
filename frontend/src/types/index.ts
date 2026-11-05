
export interface Incident {
  id: number
  event_id: string
  service: string
  event_type: string
  severity: string
  status: 'OPEN' | 'ACKNOWLEDGED' | 'RESOLVED' | string
  created_at: string
  acknowledged_at?: string
  resolved_at?: string
}


export interface Incident {
  id: number
  event_id: string
  service: string
  event_type: string
  severity: string
  status: 'OPEN' | 'ACKNOWLEDGED' | 'RESOLVED' | string
  created_at: string
  acknowledged_at?: string
  resolved_at?: string
}

export interface DashboardSummary {
  total_incidents: number
  critical_incidents: number
  high_incidents: number
  medium_incidents: number
  low_incidents: number
  open_incidents: number
  acknowledged_incidents: number
  resolved_incidents: number
}

export interface IncidentEvent {
  event_id: string
  event_type: string
  service: string
  timestamp: string
  payload: Record<string, unknown>
  created_at: string
}

export type ActionStatus =
  | 'PROPOSED'
  | 'PENDING_APPROVAL'
  | 'APPROVED'
  | 'REJECTED'
  | 'EXECUTING'
  | 'EXECUTED'
  | 'FAILED'
  | 'RECONCILIATION_REQUIRED'

export interface IncidentAction {
  id: number
  incident_id: number
  description: string
  action_type: string
  status: ActionStatus
  requires_approval: boolean
  idempotency_key: string
  approved_by?: string
  rejected_by?: string
  approved_at?: string
  rejected_at?: string
  operation_key?: string
  parameters: Record<string, unknown>
  execution_started_at?: string
  executed_at?: string
  execution_error?: string
  execution_attempts: number
  created_at: string
  updated_at: string
}

export type HealthStatus = 'healthy' | 'degraded' | 'unhealthy' | 'unknown'

export interface ComponentHealth {
  status: HealthStatus
}

export interface SystemHealth {
  status: HealthStatus
  timestamp: string
  components: {
    postgres: ComponentHealth
    redis: ComponentHealth
    kafka: ComponentHealth
  }
}
