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
  const orgId = getOrgId()
  if (!orgId) {
    // No org selected ("All Organizations") — use account-scoped path
    return `/account${path}`
  }
  return `/orgs/${orgId}${path}`
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
  tamper_protection: boolean
  isolated: boolean
  eventlog_collection: boolean
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
  tamper_protection_enabled: boolean
  telemetry_retention_days: number
}

export interface ValidationError {
  type: string
  message: string
  suggestion?: string
  position?: number
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
  validation_status: 'valid' | 'invalid' | 'pending'
  validation_errors?: ValidationError[]
  created_at: string
  updated_at: string
}

export interface SigmaConversionResult {
  success: boolean
  fibratus_yaml?: string
  rule_name?: string
  condition?: string
  severity?: string
  labels?: Record<string, string>
  errors?: string[]
  warnings?: string[]
  unconvertible?: boolean
  reason?: string
  sigma_id?: string
  sigma_title?: string
}

export interface SigmaBatchResult {
  total: number
  converted: number
  failed: number
  skipped: number
  results: SigmaConversionResult[]
}

export interface SigmaLogsourceMapping {
  category: string
  product: string
  service: string
  event_type: string
  macro: string
  description: string
  fields: Record<string, string>
}

export interface SigmaFieldMapping {
  sigma_field: string
  fibratus_field: string
  category: string
  notes: string
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
  telemetry_retention_days: number
  org_count: number
  user_count: number
  created_at: string
}

export interface Capture {
  id: string
  org_id: string
  agent_id: string
  agent_hostname: string
  filter: string
  status: 'active' | 'completed' | 'failed' | 'cancelled'
  event_count: number
  duration_sec: number
  kcap_path: string
  created_by: string
  started_at: string
  completed_at: string | null
}

export interface CaptureEvent {
  id: number
  capture_id: string
  org_id: string
  seq: number
  timestamp: string
  event_name: string
  event_category: string
  pid: number
  process_name: string
  process_exe: string
  process_cmdline: string
  parent_pid: number
  parent_name: string
  params: Record<string, unknown>
  raw_event: unknown
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
  is_locked: boolean
  login_attempts: number
  org_restrictions: string | string[] | null
  created_at: string
}

export interface AuthResponse {
  token: string
  totp_required?: boolean
  mfa_setup_required?: boolean
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
  members?: Array<{ id: string; name: string; email: string }>
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

  // API Keys
  createAPIKey: (data: { name: string }) =>
    fetchApi<{ id: string; name: string; key_prefix: string; key: string; created_at: string }>('/auth/api-keys', { method: 'POST', body: JSON.stringify(data) }),
  listAPIKeys: () =>
    fetchApi<Array<{ id: string; name: string; key_prefix: string; created_at: string; last_used_at?: string; expires_at?: string }>>('/auth/api-keys'),
  deleteAPIKey: (id: string) =>
    fetchApi<void>(`/auth/api-keys/${id}`, { method: 'DELETE' }),

  // User profile
  getCurrentUser: () => fetchApi<User>('/auth/me'),
  getMyPermissions: () => fetchApi<string[]>('/auth/me/permissions'),
  updateMyProfile: (data: { name: string }) =>
    fetchApi<{ status: string }>('/auth/me/profile', { method: 'PUT', body: JSON.stringify(data) }),
  changeMyPassword: (data: { current_password: string; new_password: string }) =>
    fetchApi<{ status: string }>('/auth/me/password', { method: 'PUT', body: JSON.stringify(data) }),

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
  setTamperProtection: (agentId: string, enabled: boolean) =>
    fetchApi<void>(orgPath(`/agents/${agentId}/tamper-protection`), {
      method: 'PUT',
      body: JSON.stringify({ enabled }),
    }),
  setEventLogCollection: (agentId: string, enabled: boolean) =>
    fetchApi<void>(orgPath(`/agents/${agentId}/eventlog-collection`), {
      method: 'PUT',
      body: JSON.stringify({ enabled }),
    }),

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
  getUserGroups: (userId: string) =>
    fetchApi<Array<{ group_id: string; group_name: string }>>(orgPath(`/users/${userId}/groups`)),
  updateUserGroups: (userId: string, groupIds: string[]) =>
    fetchApi<void>(orgPath(`/users/${userId}/groups`), { method: 'PUT', body: JSON.stringify({ group_ids: groupIds }) }),

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
  getDetectionProcessContext: (id: string, pid: number, ancestorsOnly = false) =>
    fetchApi<{ events: unknown[]; target_pid: number; parent_pid: number; child_pids: Record<number, boolean> }>(
      orgPath(`/detections/${id}/process-context?pid=${pid}${ancestorsOnly ? '&ancestors=true' : ''}`)
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
  validateRuleCondition: (condition: string) =>
    fetchApi<{ valid: boolean; errors?: ValidationError[]; warnings?: string[] }>(
      orgPath('/rules/validate-condition'),
      { method: 'POST', body: JSON.stringify({ condition }) },
    ),
  validateAllRules: () =>
    fetchApi<{ validated: number; invalid: number; valid: number }>(
      orgPath('/rules/validate-all'),
      { method: 'POST' },
    ),

  // Telemetry (live events)
  getOrgTelemetry: (params?: Record<string, string>) => {
    const q = new URLSearchParams(params || {}).toString()
    return fetchApi<unknown[]>(orgPath(`/telemetry?${q}`))
  },
  getTelemetryFields: () =>
    fetchApi<{ event_types: string[]; event_categories: string[]; process_names: string[]; agents: string[] }>(orgPath('/telemetry/fields')),
  getTelemetryProcessTree: (agentId: string, pid: number, timestamp: string) =>
    fetchApi<{ events: unknown[]; focus_pid: string }>(orgPath(`/telemetry/process-tree?agent_id=${agentId}&pid=${pid}&timestamp=${timestamp}`)),
  getAgentEvents: (agentId: string, limit?: number) =>
    fetchApi<unknown[]>(orgPath(`/agents/${agentId}/events?limit=${limit || 100}`)),

  // Commands (active response)
  getAgentCommands: (agentId: string) =>
    fetchApi<Command[]>(orgPath(`/agents/${agentId}/commands`)),
  getAgentHeartbeatHistory: (agentId: string, limit = 60) =>
    fetchApi<Array<{ cpu_pct: number; mem_mb: number; events_per_sec: number; active_rules: number; timestamp: string }>>(orgPath(`/agents/${agentId}/heartbeat-history?limit=${limit}`)),
  createCommand: (agentId: string, type: string, payload?: Record<string, unknown>) =>
    fetchApi<Command>(orgPath(`/agents/${agentId}/commands`), {
      method: 'POST',
      body: JSON.stringify({ type, payload: payload || {} }),
    }),

  // Captures
  listCaptures: (agentId: string) =>
    fetchApi<Capture[]>(orgPath(`/agents/${agentId}/captures`)),
  createCapture: (agentId: string, data: { filter?: string; duration_sec?: number }) =>
    fetchApi<Capture>(orgPath(`/agents/${agentId}/captures`), { method: 'POST', body: JSON.stringify(data) }),
  getCapture: (captureId: string) =>
    fetchApi<Capture>(orgPath(`/captures/${captureId}`)),
  getCaptureEvents: (captureId: string, params?: Record<string, string>) => {
    const q = new URLSearchParams(params || {}).toString()
    return fetchApi<CaptureEvent[]>(orgPath(`/captures/${captureId}/events?${q}`))
  },
  stopCapture: (captureId: string) =>
    fetchApi<Capture>(orgPath(`/captures/${captureId}/stop`), { method: 'POST' }),
  deleteCapture: (captureId: string) =>
    fetchApi<void>(orgPath(`/captures/${captureId}`), { method: 'DELETE' }),

  // Macros
  getMacros: () => fetchApi<Macro[]>(orgPath('/macros')),
  createMacro: (data: Partial<Macro>) =>
    fetchApi<Macro>(orgPath('/macros'), { method: 'POST', body: JSON.stringify(data) }),
  updateMacro: (id: string, data: Partial<Macro>) =>
    fetchApi<Macro>(orgPath(`/macros/${id}`), { method: 'PUT', body: JSON.stringify(data) }),
  deleteMacro: (id: string) => fetchApi<void>(orgPath(`/macros/${id}`), { method: 'DELETE' }),
  uploadMacros: (yamlContent: string) =>
    fetchApi<{ imported: number }>(orgPath('/macros/upload'), { method: 'POST', body: yamlContent }),

  // Audit Log
  getAuditLog: (params?: Record<string, string>) => {
    const q = new URLSearchParams(params || {}).toString()
    return fetchApi<AuditEntry[]>(orgPath(`/audit-log?${q}`))
  },

  // GitHub Sync
  listGitHubSyncConfigs: () => fetchApi<unknown[]>(orgPath('/github-sync')),
  saveGitHubSyncConfig: (data: Record<string, unknown>) =>
    fetchApi<unknown>(orgPath('/github-sync'), { method: 'POST', body: JSON.stringify(data) }),
  deleteGitHubSyncConfig: (id: string) =>
    fetchApi<void>(orgPath(`/github-sync/${id}`), { method: 'DELETE' }),
  triggerGitHubSync: () =>
    fetchApi<unknown>(orgPath('/github-sync/trigger'), { method: 'POST' }),
  triggerGitHubSyncOne: (id: string) =>
    fetchApi<unknown>(orgPath(`/github-sync/${id}/trigger`), { method: 'POST' }),

  // SIGMA Converter
  convertSigmaRule: (sigmaYaml: string) =>
    fetchApi<SigmaConversionResult>(orgPath('/sigma/convert'), { method: 'POST', body: JSON.stringify({ sigma_yaml: sigmaYaml }) }),
  convertSigmaBatch: (rules: string[]) =>
    fetchApi<SigmaBatchResult>(orgPath('/sigma/convert/batch'), { method: 'POST', body: JSON.stringify({ rules }) }),
  importSigmaRule: (sigmaYaml: string) =>
    fetchApi<SigmaConversionResult>(orgPath('/sigma/import'), { method: 'POST', body: JSON.stringify({ sigma_yaml: sigmaYaml }) }),
  importSigmaBatch: (rules: string[]) =>
    fetchApi<SigmaBatchResult>(orgPath('/sigma/import/batch'), { method: 'POST', body: JSON.stringify({ rules }) }),
  validateSigmaRule: (sigmaYaml: string) =>
    fetchApi<{ convertible: boolean; unconvertible: boolean; reason: string; errors: string[]; warnings: string[]; condition: string }>(orgPath('/sigma/validate'), { method: 'POST', body: JSON.stringify({ sigma_yaml: sigmaYaml }) }),
  getSigmaLogsources: () =>
    fetchApi<SigmaLogsourceMapping[]>(orgPath('/sigma/logsources')),
  getSigmaFieldMappings: () =>
    fetchApi<SigmaFieldMapping[]>(orgPath('/sigma/field-mappings')),

  // SigmaHQ Integration
  getSigmaHQStatus: () =>
    fetchApi<{ enabled: boolean; available: boolean; rule_count: number; last_commit: string; last_updated: string; total_files: number }>('/account/sigmahq/status'),
  enableSigmaHQ: () =>
    fetchApi<{ converted: number; skipped: number; failed: number; invalid: number; errors?: string[]; duration: string }>('/account/sigmahq/enable', { method: 'POST' }),
  disableSigmaHQ: () =>
    fetchApi<{ deleted: number }>('/account/sigmahq/disable', { method: 'POST' }),
  refreshSigmaHQ: () =>
    fetchApi<{ converted: number; skipped: number; failed: number; invalid: number; errors?: string[]; duration: string }>('/account/sigmahq/refresh', { method: 'POST' }),

  // Account Settings
  getAccountSettings: () => fetchApi<{ require_2fa: boolean; account_name: string; plan: string; tamper_protection_enabled: boolean; isolation_whitelist: string[]; org_protection: Array<{ id: string; name: string; tamper_protection_enabled: boolean }> }>('/account/settings'),
  updateAccountSettings: (data: { require_2fa?: boolean; tamper_protection_enabled?: boolean; eventlog_enabled?: boolean; isolation_whitelist?: string[]; allowed_file_extensions?: string[] }) =>
    fetchApi<{ require_2fa: boolean; tamper_protection_enabled: boolean; isolation_whitelist: string[] }>('/account/settings', { method: 'PUT', body: JSON.stringify(data) }),
  updateOrgTamperProtection: (orgId: string, enabled: boolean) =>
    fetchApi<void>(`/account/orgs/${orgId}/tamper-protection`, { method: 'PUT', body: JSON.stringify({ enabled }) }),
  updateTelemetryRetention: (days: number) =>
    fetchApi<{ telemetry_retention_days: number }>('/account/telemetry-retention', { method: 'PUT', body: JSON.stringify({ days }) }),
  updateOrgRetention: (orgId: string, days: number) =>
    fetchApi<{ telemetry_retention_days: number }>(`/account/orgs/${orgId}/telemetry-retention`, { method: 'PUT', body: JSON.stringify({ days }) }),

  // Event Log Policy
  getEventLogPolicy: () =>
    fetchApi<{ id: string; org_id: string; enabled: boolean; channels: Array<{ name: string; collect_all: boolean; event_ids?: number[] }>; version: number }>(orgPath('/eventlog-policy')),
  updateEventLogPolicy: (data: { enabled: boolean; channels: Array<{ name: string; collect_all: boolean; event_ids?: number[] }> }) =>
    fetchApi<{ id: string; org_id: string; enabled: boolean; channels: Array<{ name: string; collect_all: boolean; event_ids?: number[] }>; version: number }>(orgPath('/eventlog-policy'), { method: 'PUT', body: JSON.stringify(data) }),

  // Admin (root only)
  adminGetAccounts: () => fetchApi<unknown[]>('/admin/accounts'),
  adminCreateAccount: (data: { name: string; plan: string }) =>
    fetchApi<unknown>('/admin/accounts', { method: 'POST', body: JSON.stringify(data) }),
  adminDeleteAccount: (id: string) =>
    fetchApi<void>(`/admin/accounts/${id}`, { method: 'DELETE' }),
  adminGetAccountOrgs: (accountId: string) =>
    fetchApi<unknown[]>(`/admin/accounts/${accountId}/orgs`),
  adminGetAllUsers: () => fetchApi<unknown[]>('/admin/users'),
  adminUpdateAccount: (accountId: string, data: { name?: string; plan?: string; require_2fa?: boolean; telemetry_retention_days?: number }) =>
    fetchApi<Account>(`/admin/accounts/${accountId}`, { method: 'PUT', body: JSON.stringify(data) }),
  adminSwitchAccount: (accountId: string) =>
    fetchApi<{ account_id: string; account_name: string }>('/admin/switch-account', { method: 'POST', body: JSON.stringify({ account_id: accountId }) }),

  // DB Admin (root only)
  dbQueryPostgres: (query: string) =>
    fetchApi<{ columns: string[]; rows: unknown[][]; affected_rows: number; error?: string }>('/admin/db/postgres/query', { method: 'POST', body: JSON.stringify({ query }) }),
  dbQueryClickhouse: (query: string) =>
    fetchApi<{ columns: string[]; rows: unknown[][]; affected_rows: number; error?: string }>('/admin/db/clickhouse/query', { method: 'POST', body: JSON.stringify({ query }) }),
  dbTablesPostgres: () =>
    fetchApi<{ columns: string[]; rows: unknown[][] }>('/admin/db/postgres/tables'),
  dbTablesClickhouse: () =>
    fetchApi<{ columns: string[]; rows: unknown[][] }>('/admin/db/clickhouse/tables'),
  adminGetAccountUsers: (accountId: string) => fetchApi<User[]>(`/admin/accounts/${accountId}/users`),
  adminUpdateUser: (id: string, data: { name?: string; email?: string; role?: string; account_id?: string; org_restrictions?: string[]; set_org_restrictions?: boolean }) =>
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
  return !!getToken()
}

export function getCurrentOrgId(): string {
  return getOrgId()
}

export function setCurrentOrgId(orgId: string) {
  localStorage.setItem('fleet_org_id', orgId)
}
