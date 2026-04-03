const API_BASE = '/api/v1'

function getToken(): string | null {
  return localStorage.getItem('fleet_token')
}

function getOrgId(): string {
  return localStorage.getItem('fleet_org_id') || ''
}

export interface ApiResponse<T> {
  data?: T
  error?: { code: number; message: string }
  meta?: { total: number; page: number; per_page: number }
}

async function fetchApi<T>(path: string, options?: RequestInit): Promise<ApiResponse<T>> {
  const token = getToken()
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (token) headers['Authorization'] = `Bearer ${token}`

  const res = await fetch(`${API_BASE}${path}`, { headers, ...options })

  if (res.status === 401) {
    localStorage.removeItem('fleet_token')
    localStorage.removeItem('fleet_org_id')
    window.location.href = '/login'
    throw new Error('Unauthorized')
  }

  if (res.status === 204) return {} as ApiResponse<T>
  return res.json()
}

function orgPath(path: string): string {
  return `/orgs/${getOrgId()}${path}`
}

// ═════════════════════════════════════════════════
// Types
// ═════════════════════════════════════════════════

export interface FleetOverview {
  total_agents: number
  online_agents: number
  offline_agents: number
  total_detections_24h: number
  severity_breakdown: Record<string, number>
}

export interface Agent {
  id: string
  org_id: string
  hostname: string
  os_version: string
  engine_version: string
  group_id: string
  group_name: string
  tags: Record<string, string>
  status: 'online' | 'offline' | 'stale'
  last_heartbeat: string
  registered_at: string
}

export interface Detection {
  id: string
  org_id: string
  agent_id: string
  agent_hostname: string
  rule_id: string
  rule_name: string
  title: string
  text: string
  description: string
  severity: string
  labels: Record<string, string>
  tags: string[]
  events: unknown
  timestamp: string
}

export interface TimelineBucket {
  timestamp: string
  count: number
  by_severity: Record<string, number>
}

export interface Organization {
  id: string
  account_id: string
  name: string
  slug: string
  agent_count: number
}

export interface Rule {
  id: string
  org_id: string
  name: string
  version: string
  description: string
  condition: string
  output: string
  severity: string
  labels: Record<string, string>
  tags: string[]
  raw_yaml: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface Command {
  id: string
  org_id: string
  agent_id: string
  type: string
  payload: unknown
  status: 'pending' | 'running' | 'completed' | 'failed'
  result: unknown
  error_message: string
  created_by: string
  created_at: string
  started_at: string | null
  completed_at: string | null
}

export interface EnrollmentToken {
  id: string
  name: string
  org_id: string
  org_name: string
  max_uses: number
  uses_count: number
  expires_at: string
  created_by: string
  created_at: string
}

export interface User {
  id: string
  email: string
  name: string
  account_id: string
  role: string
  created_at: string
}

export interface AuthResponse {
  token: string
  user?: User
  account_id?: string
  org_id?: string
}

// ═════════════════════════════════════════════════
// API Client
// ═════════════════════════════════════════════════

export const api = {
  // Auth
  signup: (data: { account_name: string; org_name: string; email: string; name: string; password: string }) =>
    fetchApi<AuthResponse>('/auth/signup', { method: 'POST', body: JSON.stringify(data) }),

  login: (data: { email: string; password: string }) =>
    fetchApi<AuthResponse>('/auth/login', { method: 'POST', body: JSON.stringify(data) }),

  // Organizations
  getOrganizations: () => fetchApi<Organization[]>('/account/organizations'),
  createOrganization: (data: { name: string; slug: string }) =>
    fetchApi<Organization>('/account/organizations', { method: 'POST', body: JSON.stringify(data) }),

  // Dashboard
  getDashboardOverview: () => fetchApi<FleetOverview>(orgPath('/dashboard/overview')),

  // Agents
  getAgents: (params?: Record<string, string>) => {
    const q = new URLSearchParams(params || {}).toString()
    return fetchApi<Agent[]>(orgPath(`/agents?${q}`))
  },
  getAgent: (id: string) => fetchApi<Agent>(orgPath(`/agents/${id}`)),
  deleteAgent: (id: string) => fetchApi<void>(orgPath(`/agents/${id}`), { method: 'DELETE' }),

  // Detections
  getDetections: (params?: Record<string, string>) => {
    const q = new URLSearchParams(params || {}).toString()
    return fetchApi<Detection[]>(orgPath(`/detections?${q}`))
  },
  getDetection: (id: string) => fetchApi<Detection>(orgPath(`/detections/${id}`)),
  getDetectionTimeline: (params?: Record<string, string>) => {
    const q = new URLSearchParams(params || {}).toString()
    return fetchApi<TimelineBucket[]>(orgPath(`/detections/timeline?${q}`))
  },

  // Rules (org-scoped)
  getRules: (params?: Record<string, string>) => {
    const q = new URLSearchParams(params || {}).toString()
    return fetchApi<Rule[]>(orgPath(`/rules?${q}`))
  },
  createRule: (yaml: string) => {
    const token = getToken()
    const orgId = getOrgId()
    return fetch(`${API_BASE}/orgs/${orgId}/rules`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-yaml', Authorization: `Bearer ${token}` },
      body: yaml,
    }).then(r => r.json()) as Promise<ApiResponse<Rule>>
  },
  updateRule: (id: string, data: Partial<Rule>) =>
    fetchApi<Rule>(orgPath(`/rules/${id}`), { method: 'PUT', body: JSON.stringify(data) }),
  updateRuleYaml: (id: string, yaml: string) => {
    const token = getToken()
    const orgId = getOrgId()
    return fetch(`${API_BASE}/orgs/${orgId}/rules/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/x-yaml', Authorization: `Bearer ${token}` },
      body: yaml,
    }).then(r => r.json()) as Promise<ApiResponse<Rule>>
  },
  deleteRule: (id: string) => fetchApi<void>(orgPath(`/rules/${id}`), { method: 'DELETE' }),

  // Telemetry (live events)
  getOrgTelemetry: (params?: Record<string, string>) => {
    const q = new URLSearchParams(params || {}).toString()
    return fetchApi<unknown[]>(orgPath(`/telemetry?${q}`))
  },
  getAgentEvents: (agentId: string, limit?: number) =>
    fetchApi<unknown[]>(orgPath(`/agents/${agentId}/events?limit=${limit || 100}`)),

  // Commands (active response)
  getAgentCommands: (agentId: string) =>
    fetchApi<Command[]>(orgPath(`/agents/${agentId}/commands`)),
  createCommand: (agentId: string, type: string, payload?: Record<string, unknown>) =>
    fetchApi<Command>(orgPath(`/agents/${agentId}/commands`), {
      method: 'POST',
      body: JSON.stringify({ type, payload: payload || {} }),
    }),

  // Enrollment Tokens
  getEnrollmentTokens: () => fetchApi<EnrollmentToken[]>(orgPath('/enrollment-tokens')),
  createEnrollmentToken: (data: { name: string; max_uses: number; expires_in: number }) =>
    fetchApi<EnrollmentToken>(orgPath('/enrollment-tokens'), { method: 'POST', body: JSON.stringify(data) }),
  deleteEnrollmentToken: (id: string) => fetchApi<void>(orgPath(`/enrollment-tokens/${id}`), { method: 'DELETE' }),
}

// ═════════════════════════════════════════════════
// Session
// ═════════════════════════════════════════════════

export function setSession(token: string, orgId: string) {
  localStorage.setItem('fleet_token', token)
  localStorage.setItem('fleet_org_id', orgId)
}

export function clearSession() {
  localStorage.removeItem('fleet_token')
  localStorage.removeItem('fleet_org_id')
}

export function isAuthenticated(): boolean {
  return !!getToken() && !!getOrgId()
}

export function getCurrentOrgId(): string {
  return getOrgId()
}

export function setCurrentOrgId(orgId: string) {
  localStorage.setItem('fleet_org_id', orgId)
}
