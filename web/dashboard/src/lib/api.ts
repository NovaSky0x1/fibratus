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

export interface Macro {
  id: string
  org_id: string
  name: string
  expr: string
  list: string[]
  description: string
  raw_yaml: string
  created_at: string
  updated_at: string
}

export interface AuditEntry {
  id: string
  org_id: string
  user_id: string
  user_email: string
  action: string
  resource_type: string
  resource_id: string
  resource_name: string
  details: unknown
  ip_address: string
  timestamp: string
}

export interface Account {
  id: string
  name: string
  plan: string
  require_2fa: boolean
  org_count: number
  user_count: number
  created_at: string
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
  totp_enabled: boolean
  locked: boolean
  created_at: string
}

export interface AuthResponse {
  token: string
  totp_required?: boolean
  user?: User
  account_id?: string
  org_id?: string
}

export interface UserGroup {
  id: string
  account_id: string
  name: string
  description: string
  permissions: string[]
  org_restrictions: string[]
  created_at: string
  updated_at: string
}

export interface PermissionDef {
  id: string
  name: string
  category: string
}

// ═════════════════════════════════════════════════
// API Client
// ═════════════════════════════════════════════════

export const api = {
  // Auth
  signup: (data: { account_name: string; org_name: string; email: string; name: string; password: string }) =>
    fetchApi<AuthResponse>('/auth/signup', { method: 'POST', body: JSON.stringify(data) }),

  login: (data: { email: string; password: string; totp_code?: string }) =>
    fetchApi<AuthResponse>('/auth/login', { method: 'POST', body: JSON.stringify(data) }),

  // TOTP 2FA
  getTOTPStatus: () => fetchApi<{ enabled: boolean }>('/auth/totp/status'),
  setupTOTP: () => fetchApi<{ secret: string; uri: string }>('/auth/totp/setup', { method: 'POST' }),
  verifyTOTP: (code: string) => fetchApi<{ enabled: boolean; recovery_codes: string[] }>('/auth/totp/verify', { method: 'POST', body: JSON.stringify({ code }) }),
  disableTOTP: (password: string) => fetchApi<{ status: string }>('/auth/totp/disable', { method: 'POST', body: JSON.stringify({ password }) }),

  // User profile
  getCurrentUser: () => fetchApi<User>('/auth/me'),

  // Organizations
  getOrganizations: () => fetchApi<Organization[]>('/account/organizations'),
  createOrganization: (data: { name: string; slug: string }) =>
    fetchApi<Organization>('/account/organizations', { method: 'POST', body: JSON.stringify(data) }),
  deleteOrganization: (id: string) =>
    fetchApi<void>(`/account/organizations/${id}`, { method: 'DELETE' }),

  // Dashboard
  getDashboardOverview: () => fetchApi<FleetOverview>(orgPath('/dashboard/overview')),

  // Agents
  getAgents: (params?: Record<string, string>) => {
    const q = new URLSearchParams(params || {}).toString()
    return fetchApi<Agent[]>(orgPath(`/agents?${q}`))
  },
  getAgent: (id: string) => fetchApi<Agent>(orgPath(`/agents/${id}`)),
  deleteAgent: (id: string) => fetchApi<void>(orgPath(`/agents/${id}`), { method: 'DELETE' }),

  // Users
  getUsers: () => fetchApi<User[]>(orgPath('/users')),
  createUser: (data: { email: string; name: string; password: string; role: string; org_restrictions?: string[]; group_ids?: string[] }) =>
    fetchApi<User>(orgPath('/users'), { method: 'POST', body: JSON.stringify(data) }),
  updateUserRole: (id: string, role: string) =>
    fetchApi<{ status: string; role: string }>(orgPath(`/users/${id}/role`), { method: 'PUT', body: JSON.stringify({ role }) }),
  updateUser: (id: string, data: { name: string; email: string }) =>
    fetchApi<{ status: string }>(orgPath(`/users/${id}`), { method: 'PUT', body: JSON.stringify(data) }),
  resetUserPassword: (id: string, password: string) =>
    fetchApi<{ status: string }>(orgPath(`/users/${id}/password`), { method: 'PUT', body: JSON.stringify({ password }) }),
  disableUserTOTP: (id: string) =>
    fetchApi<{ status: string }>(orgPath(`/users/${id}/totp`), { method: 'DELETE' }),
  deleteUser: (id: string) =>
    fetchApi<{ status: string }>(orgPath(`/users/${id}`), { method: 'DELETE' }),

  // User Groups
  getGroups: () => fetchApi<unknown[]>(orgPath('/groups')),
  createGroup: (data: { name: string; description: string; permissions: string[]; org_restrictions: string[] }) =>
    fetchApi<unknown>(orgPath('/groups'), { method: 'POST', body: JSON.stringify(data) }),
  updateGroup: (id: string, data: { name: string; description: string; permissions: string[]; org_restrictions: string[] }) =>
    fetchApi<unknown>(orgPath(`/groups/${id}`), { method: 'PUT', body: JSON.stringify(data) }),
  deleteGroup: (id: string) =>
    fetchApi<void>(orgPath(`/groups/${id}`), { method: 'DELETE' }),
  addGroupMember: (groupId: string, userId: string) =>
    fetchApi<unknown>(orgPath(`/groups/${groupId}/members`), { method: 'POST', body: JSON.stringify({ user_id: userId }) }),
  removeGroupMember: (groupId: string, userId: string) =>
    fetchApi<void>(orgPath(`/groups/${groupId}/members/${userId}`), { method: 'DELETE' }),
  getPermissions: () => fetchApi<{ id: string; name: string; category: string }[]>(orgPath('/permissions')),

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
  getDetectionProcessTree: (id: string) =>
    fetchApi<{ detection: Detection; events: unknown[] }>(orgPath(`/detections/${id}/process-tree`)),
  getDetectionProcessContext: (id: string, pid: number) =>
    fetchApi<{ events: unknown[]; target_pid: number; parent_pid: number; child_pids: Record<number, boolean> }>(
      orgPath(`/detections/${id}/process-context?pid=${pid}`)
    ),

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

  // Macros
  getMacros: () => fetchApi<Macro[]>(orgPath('/macros')),
  createMacro: (data: Partial<Macro>) =>
    fetchApi<Macro>(orgPath('/macros'), { method: 'POST', body: JSON.stringify(data) }),
  updateMacro: (id: string, data: Partial<Macro>) =>
    fetchApi<Macro>(orgPath(`/macros/${id}`), { method: 'PUT', body: JSON.stringify(data) }),
  deleteMacro: (id: string) => fetchApi<void>(orgPath(`/macros/${id}`), { method: 'DELETE' }),

  // Audit Log
  getAuditLog: (params?: Record<string, string>) => {
    const q = new URLSearchParams(params || {}).toString()
    return fetchApi<AuditEntry[]>(orgPath(`/audit-log?${q}`))
  },

  // GitHub Sync
  getGitHubSyncConfig: () => fetchApi<unknown>(orgPath('/github-sync')),
  saveGitHubSyncConfig: (data: { repo_url: string; branch: string; path: string; token: string; interval: number; enabled: boolean }) =>
    fetchApi<unknown>(orgPath('/github-sync'), { method: 'PUT', body: JSON.stringify(data) }),
  triggerGitHubSync: () =>
    fetchApi<{ created: number; updated: number; skipped: number; errors: string[]; duration: string }>(orgPath('/github-sync/trigger'), { method: 'POST' }),

  // Account Settings
  getAccountSettings: () => fetchApi<{ require_2fa: boolean; account_name: string; plan: string }>('/account/settings'),
  updateAccountSettings: (data: { require_2fa: boolean }) =>
    fetchApi<{ require_2fa: boolean }>('/account/settings', { method: 'PUT', body: JSON.stringify(data) }),

  // Admin (root only)
  adminGetAccounts: () => fetchApi<unknown[]>('/admin/accounts'),
  adminCreateAccount: (data: { name: string; plan: string }) =>
    fetchApi<unknown>('/admin/accounts', { method: 'POST', body: JSON.stringify(data) }),
  adminDeleteAccount: (id: string) =>
    fetchApi<void>(`/admin/accounts/${id}`, { method: 'DELETE' }),
  adminGetAccountOrgs: (accountId: string) =>
    fetchApi<unknown[]>(`/admin/accounts/${accountId}/orgs`),
  adminGetAllUsers: () => fetchApi<unknown[]>('/admin/users'),
  adminUpdateAccount: (accountId: string, data: { name?: string; plan?: string; require_2fa?: boolean }) =>
    fetchApi<Account>(`/admin/accounts/${accountId}`, { method: 'PUT', body: JSON.stringify(data) }),
  adminSwitchAccount: (accountId: string) =>
    fetchApi<{ account_id: string; account_name: string }>('/admin/switch-account', { method: 'POST', body: JSON.stringify({ account_id: accountId }) }),
  adminUpdateUser: (id: string, data: { name?: string; email?: string; role?: string }) =>
    fetchApi<{ status: string }>(`/admin/users/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  adminDeleteUser: (id: string) =>
    fetchApi<void>(`/admin/users/${id}`, { method: 'DELETE' }),
  adminUnlockUser: (id: string) =>
    fetchApi<{ status: string }>(`/admin/users/${id}/unlock`, { method: 'POST' }),
  adminResetPassword: (id: string, password: string) =>
    fetchApi<{ status: string }>(`/admin/users/${id}/reset-password`, { method: 'POST', body: JSON.stringify({ password }) }),
  adminDisableTOTP: (id: string) =>
    fetchApi<{ status: string }>(`/admin/users/${id}/disable-totp`, { method: 'POST' }),

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
