const API_BASE = '/api/v1'

function getToken(): string | null {
  return localStorage.getItem('fleet_token')
}

function getOrgId(): string {
  return localStorage.getItem('fleet_org_id') || ''
}

interface ApiResponse<T> {
  data?: T
  error?: { code: number; message: string }
  meta?: { total: number; page: number; per_page: number }
}

async function fetchApi<T>(path: string, options?: RequestInit): Promise<ApiResponse<T>> {
  const token = getToken()
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  const res = await fetch(`${API_BASE}${path}`, {
    headers,
    ...options,
  })

  if (res.status === 401) {
    localStorage.removeItem('fleet_token')
    localStorage.removeItem('fleet_org_id')
    window.location.href = '/login'
    throw new Error('Unauthorized')
  }

  return res.json()
}

// Org-scoped API helper
function orgPath(path: string): string {
  const orgId = getOrgId()
  return `/orgs/${orgId}${path}`
}

// ═══════════════════════════════════════════════════════════════
// Auth
// ═══════════════════════════════════════════════════════════════

export interface SignupData {
  account_name: string
  org_name: string
  email: string
  name: string
  password: string
}

export interface LoginData {
  email: string
  password: string
}

export interface AuthResponse {
  token: string
  user?: {
    id: string
    email: string
    name: string
    account_id: string
    role: string
  }
  account_id?: string
  org_id?: string
  user_id?: string
}

// ═══════════════════════════════════════════════════════════════
// Types
// ═══════════════════════════════════════════════════════════════

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
  events: unknown[]
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

// ═══════════════════════════════════════════════════════════════
// API client
// ═══════════════════════════════════════════════════════════════

export type { ApiResponse }

export const api = {
  // Generic org-scoped resource fetch
  getOrgResource: <T>(path: string, options?: RequestInit) =>
    fetchApi<T>(orgPath(path), options),

  // Auth
  signup: (data: SignupData) =>
    fetchApi<AuthResponse>('/auth/signup', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  login: (data: LoginData) =>
    fetchApi<AuthResponse>('/auth/login', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Organizations
  getOrganizations: () =>
    fetchApi<Organization[]>('/account/organizations'),

  // Dashboard (org-scoped)
  getDashboardOverview: () =>
    fetchApi<FleetOverview>(orgPath('/dashboard/overview')),

  // Agents (org-scoped)
  getAgents: (params?: { page?: number; status?: string; search?: string }) => {
    const query = new URLSearchParams()
    if (params?.page) query.set('page', String(params.page))
    if (params?.status) query.set('status', params.status)
    if (params?.search) query.set('search', params.search)
    return fetchApi<Agent[]>(orgPath(`/agents?${query}`))
  },

  getAgent: (id: string) =>
    fetchApi<Agent>(orgPath(`/agents/${id}`)),

  // Detections (org-scoped)
  getDetections: (params?: { page?: number; severity?: string; agent_id?: string }) => {
    const query = new URLSearchParams()
    if (params?.page) query.set('page', String(params.page))
    if (params?.severity) query.set('severity', params.severity)
    if (params?.agent_id) query.set('agent_id', params.agent_id)
    return fetchApi<Detection[]>(orgPath(`/detections?${query}`))
  },

  getDetection: (id: string) =>
    fetchApi<Detection>(orgPath(`/detections/${id}`)),

  getDetectionTimeline: (from?: string, to?: string) => {
    const query = new URLSearchParams()
    if (from) query.set('from', from)
    if (to) query.set('to', to)
    return fetchApi<TimelineBucket[]>(orgPath(`/detections/timeline?${query}`))
  },
}

// ═══════════════════════════════════════════════════════════════
// Session helpers
// ═══════════════════════════════════════════════════════════════

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
