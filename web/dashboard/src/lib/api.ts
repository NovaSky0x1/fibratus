const API_BASE = '/api/v1'

interface ApiResponse<T> {
  data?: T
  error?: { code: number; message: string }
  meta?: { total: number; page: number; per_page: number }
}

async function fetchApi<T>(path: string, options?: RequestInit): Promise<ApiResponse<T>> {
  const res = await fetch(`${API_BASE}${path}`, {
    headers: {
      'Content-Type': 'application/json',
    },
    ...options,
  })
  return res.json()
}

export interface FleetOverview {
  total_agents: number
  online_agents: number
  offline_agents: number
  total_detections_24h: number
  severity_breakdown: Record<string, number>
}

export interface Agent {
  id: string
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

export const api = {
  getDashboardOverview: () =>
    fetchApi<FleetOverview>('/dashboard/overview'),

  getAgents: (params?: { page?: number; status?: string; search?: string }) => {
    const query = new URLSearchParams()
    if (params?.page) query.set('page', String(params.page))
    if (params?.status) query.set('status', params.status)
    if (params?.search) query.set('search', params.search)
    return fetchApi<Agent[]>(`/agents?${query}`)
  },

  getAgent: (id: string) =>
    fetchApi<Agent>(`/agents/${id}`),

  getDetections: (params?: { page?: number; severity?: string; agent_id?: string }) => {
    const query = new URLSearchParams()
    if (params?.page) query.set('page', String(params.page))
    if (params?.severity) query.set('severity', params.severity)
    if (params?.agent_id) query.set('agent_id', params.agent_id)
    return fetchApi<Detection[]>(`/detections?${query}`)
  },

  getDetection: (id: string) =>
    fetchApi<Detection>(`/detections/${id}`),

  getDetectionTimeline: (from?: string, to?: string) => {
    const query = new URLSearchParams()
    if (from) query.set('from', from)
    if (to) query.set('to', to)
    return fetchApi<TimelineBucket[]>(`/detections/timeline?${query}`)
  },
}
