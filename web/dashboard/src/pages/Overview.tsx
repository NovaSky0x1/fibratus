import { useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
  Legend,
} from 'recharts'
import {
  api,
  type FleetOverview,
  type Detection,
  type TimelineBucket,
} from '../lib/api'
import SeverityBadge from '../components/SeverityBadge'

/* ═══════════════════════════════════════════════════
   Color & gradient constants
   ═══════════════════════════════════════════════════ */

const SEVERITY_COLORS: Record<string, string> = {
  critical: '#dc2626',
  high: '#ea580c',
  medium: '#d97706',
  low: '#2563eb',
}

const SEVERITY_ORDER = ['critical', 'high', 'medium', 'low'] as const

const AGENT_RING_COLORS: Record<string, string> = {
  online: '#10b981',
  offline: '#f59e0b',
  stale: '#6b7280',
}

/* ═══════════════════════════════════════════════════
   Inline SVG icons
   ═══════════════════════════════════════════════════ */

function IconServer() {
  return (
    <svg className="h-7 w-7 text-blue-500 dark:text-cyan-400" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="M21.75 17.25v-.228a4.5 4.5 0 0 0-.12-1.03l-2.268-9.64a3.375 3.375 0 0 0-3.285-2.602H7.923a3.375 3.375 0 0 0-3.285 2.602l-2.268 9.64a4.5 4.5 0 0 0-.12 1.03v.228m19.5 0a3 3 0 0 1-3 3H5.25a3 3 0 0 1-3-3m19.5 0a3 3 0 0 0-3-3H5.25a3 3 0 0 0-3 3m16.5 0h.008v.008h-.008v-.008Zm-3 0h.008v.008h-.008v-.008Z" />
    </svg>
  )
}

function IconOnline() {
  return (
    <svg className="h-7 w-7 text-emerald-500 dark:text-teal-400" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" />
    </svg>
  )
}

function IconOffline() {
  return (
    <svg className="h-7 w-7 text-amber-500 dark:text-orange-400" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z" />
    </svg>
  )
}

function IconShield() {
  return (
    <svg className="h-7 w-7 text-red-500 dark:text-pink-400" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m0-10.036A11.959 11.959 0 0 1 3.598 6 11.99 11.99 0 0 0 3 9.75c0 5.592 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.75h-.152c-3.196 0-6.1-1.25-8.25-3.286Z" />
    </svg>
  )
}

/* ═══════════════════════════════════════════════════
   Overview page
   ═══════════════════════════════════════════════════ */

export default function Overview() {
  const navigate = useNavigate()

  const { data: overview } = useQuery({
    queryKey: ['overview'],
    queryFn: () => api.getDashboardOverview(),
  })

  const { data: detections } = useQuery({
    queryKey: ['recent-detections'],
    queryFn: () => api.getDetections({ page: '1' }),
  })

  const { data: timeline } = useQuery({
    queryKey: ['detection-timeline'],
    queryFn: () => api.getDetectionTimeline(),
  })

  const stats = overview?.data as FleetOverview | undefined
  const recentDets = (detections?.data || []) as Detection[]
  const timelineData = (timeline?.data || []) as TimelineBucket[]

  /* Severity breakdown with totals for percentage bars */
  const severityData = useMemo(() => {
    if (!stats?.severity_breakdown) return []
    const total = Object.values(stats.severity_breakdown).reduce((a, b) => a + b, 0)
    return SEVERITY_ORDER
      .filter((sev) => stats.severity_breakdown[sev] !== undefined)
      .map((sev) => ({
        name: sev,
        count: stats.severity_breakdown[sev],
        pct: total > 0 ? Math.round((stats.severity_breakdown[sev] / total) * 100) : 0,
      }))
  }, [stats?.severity_breakdown])

  /* Agent ring chart data */
  const agentRingData = useMemo(() => {
    if (!stats) return []
    const stale = Math.max(0, stats.total_agents - stats.online_agents - stats.offline_agents)
    const segments = [
      { name: 'Online', value: stats.online_agents },
      { name: 'Offline', value: stats.offline_agents },
    ]
    if (stale > 0) segments.push({ name: 'Stale', value: stale })
    return segments
  }, [stats])

  /* Top triggered rules — derived from recent detections */
  const topRules = useMemo(() => {
    const counts: Record<string, { name: string; count: number; severity: string }> = {}
    recentDets.forEach((d) => {
      const key = d.rule_name || d.title
      if (!counts[key]) counts[key] = { name: key, count: 0, severity: d.severity }
      counts[key].count++
    })
    return Object.values(counts)
      .sort((a, b) => b.count - a.count)
      .slice(0, 5)
  }, [recentDets])

  /* Severity bar gradient classes */
  const severityBarGradient: Record<string, string> = {
    critical: 'from-red-500 to-red-400',
    high: 'from-orange-500 to-orange-400',
    medium: 'from-amber-500 to-amber-400',
    low: 'from-blue-500 to-blue-400',
  }

  const severityBarBg: Record<string, string> = {
    critical: 'bg-red-100 dark:bg-red-900/30',
    high: 'bg-orange-100 dark:bg-orange-900/30',
    medium: 'bg-amber-100 dark:bg-amber-900/30',
    low: 'bg-blue-100 dark:bg-blue-900/30',
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold text-gray-900 dark:text-slate-100">Fleet Overview</h1>
        <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
          Real-time visibility into your endpoint fleet
        </p>
      </div>

      {/* ───── Stat Cards ───── */}
      <div className="grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-4">
        {/* Total Agents */}
        <div className="relative overflow-hidden rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
          <div className="absolute inset-y-0 left-0 w-1 bg-gradient-to-b from-blue-500 to-cyan-400" />
          <div className="flex items-center justify-between p-5 pl-4 ml-1">
            <div>
              <p className="text-3xl font-bold tracking-tight text-gray-900 dark:text-slate-100">
                {stats?.total_agents ?? 0}
              </p>
              <p className="mt-1 text-sm font-medium text-gray-500 dark:text-slate-400">Total Agents</p>
            </div>
            <div className="rounded-lg bg-blue-50 dark:bg-blue-900/30 p-2.5">
              <IconServer />
            </div>
          </div>
        </div>

        {/* Online Agents */}
        <div className="relative overflow-hidden rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
          <div className="absolute inset-y-0 left-0 w-1 bg-gradient-to-b from-emerald-500 to-teal-400" />
          <div className="flex items-center justify-between p-5 pl-4 ml-1">
            <div>
              <p className="text-3xl font-bold tracking-tight text-emerald-600 dark:text-emerald-400">
                {stats?.online_agents ?? 0}
              </p>
              <p className="mt-1 text-sm font-medium text-gray-500 dark:text-slate-400">Online Agents</p>
            </div>
            <div className="rounded-lg bg-emerald-50 dark:bg-emerald-900/30 p-2.5">
              <IconOnline />
            </div>
          </div>
        </div>

        {/* Offline Agents */}
        <div className="relative overflow-hidden rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
          <div className="absolute inset-y-0 left-0 w-1 bg-gradient-to-b from-amber-500 to-orange-400" />
          <div className="flex items-center justify-between p-5 pl-4 ml-1">
            <div>
              <p className="text-3xl font-bold tracking-tight text-amber-600 dark:text-amber-400">
                {stats?.offline_agents ?? 0}
              </p>
              <p className="mt-1 text-sm font-medium text-gray-500 dark:text-slate-400">Offline Agents</p>
            </div>
            <div className="rounded-lg bg-amber-50 dark:bg-amber-900/30 p-2.5">
              <IconOffline />
            </div>
          </div>
        </div>

        {/* Detections 24h */}
        <div className="relative overflow-hidden rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
          <div className="absolute inset-y-0 left-0 w-1 bg-gradient-to-b from-red-500 to-pink-400" />
          <div className="flex items-center justify-between p-5 pl-4 ml-1">
            <div>
              <p className="text-3xl font-bold tracking-tight text-red-600 dark:text-red-400">
                {stats?.total_detections_24h ?? 0}
              </p>
              <p className="mt-1 text-sm font-medium text-gray-500 dark:text-slate-400">Detections (24h)</p>
            </div>
            <div className="rounded-lg bg-red-50 dark:bg-red-900/30 p-2.5">
              <IconShield />
            </div>
          </div>
        </div>
      </div>

      {/* ───── Charts row: Timeline + Agent Ring ───── */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        {/* Detection Timeline */}
        <div className="lg:col-span-2 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50">
          <h3 className="text-sm font-semibold text-gray-700 dark:text-slate-300">
            Detection Timeline (24h)
          </h3>
          {timelineData.length > 0 ? (
            <div className="mt-4 h-72">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={timelineData} barCategoryGap="15%">
                  <defs>
                    <linearGradient id="barGradCritical" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor="#dc2626" stopOpacity={0.9} />
                      <stop offset="100%" stopColor="#f87171" stopOpacity={0.6} />
                    </linearGradient>
                    <linearGradient id="barGradHigh" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor="#ea580c" stopOpacity={0.9} />
                      <stop offset="100%" stopColor="#fb923c" stopOpacity={0.6} />
                    </linearGradient>
                    <linearGradient id="barGradMedium" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor="#d97706" stopOpacity={0.9} />
                      <stop offset="100%" stopColor="#fbbf24" stopOpacity={0.6} />
                    </linearGradient>
                    <linearGradient id="barGradLow" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor="#2563eb" stopOpacity={0.9} />
                      <stop offset="100%" stopColor="#60a5fa" stopOpacity={0.6} />
                    </linearGradient>
                    <linearGradient id="barGradDefault" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor="#4c6ef5" stopOpacity={0.9} />
                      <stop offset="100%" stopColor="#818cf8" stopOpacity={0.6} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid
                    strokeDasharray="3 3"
                    stroke="currentColor"
                    className="text-gray-200 dark:text-slate-700"
                  />
                  <XAxis
                    dataKey="timestamp"
                    tickFormatter={(t) =>
                      new Date(t).toLocaleTimeString([], {
                        hour: '2-digit',
                        minute: '2-digit',
                      })
                    }
                    fontSize={11}
                    stroke="currentColor"
                    className="text-gray-400 dark:text-slate-500"
                    tickLine={false}
                  />
                  <YAxis
                    fontSize={11}
                    stroke="currentColor"
                    className="text-gray-400 dark:text-slate-500"
                    tickLine={false}
                    axisLine={false}
                  />
                  <Tooltip
                    labelFormatter={(t) => new Date(t as string).toLocaleString()}
                    cursor={{ fill: 'currentColor', className: 'text-gray-100 dark:text-slate-700/50' }}
                    contentStyle={{
                      backgroundColor: 'var(--tooltip-bg, #fff)',
                      borderColor: 'var(--tooltip-border, #e5e7eb)',
                      borderRadius: '0.5rem',
                      fontSize: '0.75rem',
                    }}
                  />
                  <Bar dataKey="count" fill="url(#barGradDefault)" radius={[4, 4, 0, 0]} />
                </BarChart>
              </ResponsiveContainer>
            </div>
          ) : (
            <div className="mt-4 flex h-72 items-center justify-center">
              <p className="text-sm text-gray-400 dark:text-slate-500">
                No detection data in the last 24 hours.
              </p>
            </div>
          )}
        </div>

        {/* Agent Status Ring */}
        <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50">
          <h3 className="text-sm font-semibold text-gray-700 dark:text-slate-300">
            Agent Status
          </h3>
          {agentRingData.length > 0 && agentRingData.some((d) => d.value > 0) ? (
            <>
              <div className="mt-2 h-56">
                <ResponsiveContainer width="100%" height="100%">
                  <PieChart>
                    <Pie
                      data={agentRingData}
                      cx="50%"
                      cy="50%"
                      innerRadius={55}
                      outerRadius={85}
                      paddingAngle={3}
                      dataKey="value"
                      strokeWidth={0}
                    >
                      {agentRingData.map((entry) => (
                        <Cell
                          key={entry.name}
                          fill={AGENT_RING_COLORS[entry.name.toLowerCase()] || '#6b7280'}
                        />
                      ))}
                    </Pie>
                    <Legend
                      verticalAlign="bottom"
                      iconType="circle"
                      iconSize={8}
                      formatter={(value: string) => (
                        <span className="text-xs text-gray-600 dark:text-slate-400">{value}</span>
                      )}
                    />
                    <Tooltip
                      contentStyle={{
                        backgroundColor: 'var(--tooltip-bg, #fff)',
                        borderColor: 'var(--tooltip-border, #e5e7eb)',
                        borderRadius: '0.5rem',
                        fontSize: '0.75rem',
                      }}
                    />
                  </PieChart>
                </ResponsiveContainer>
              </div>
              <div className="mt-3 flex justify-center gap-5">
                {agentRingData.map((seg) => (
                  <div key={seg.name} className="text-center">
                    <p className="text-lg font-bold text-gray-900 dark:text-slate-100">{seg.value}</p>
                    <p className="text-xs text-gray-500 dark:text-slate-400">{seg.name}</p>
                  </div>
                ))}
              </div>
            </>
          ) : (
            <div className="mt-2 flex h-56 items-center justify-center">
              <p className="text-sm text-gray-400 dark:text-slate-500">No agents enrolled.</p>
            </div>
          )}
        </div>
      </div>

      {/* ───── Severity Bars + Top Rules ───── */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* Severity Breakdown */}
        <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50">
          <h3 className="text-sm font-semibold text-gray-700 dark:text-slate-300">
            Severity Breakdown
          </h3>
          {severityData.length > 0 ? (
            <div className="mt-4 space-y-4">
              {severityData.map((sev) => (
                <div key={sev.name}>
                  <div className="flex items-center justify-between mb-1.5">
                    <span className="text-sm font-medium capitalize text-gray-700 dark:text-slate-300">
                      {sev.name}
                    </span>
                    <span className="text-xs tabular-nums text-gray-500 dark:text-slate-400">
                      {sev.count} ({sev.pct}%)
                    </span>
                  </div>
                  <div className={`h-2.5 w-full rounded-full ${severityBarBg[sev.name] || 'bg-gray-100 dark:bg-slate-700'}`}>
                    <div
                      className={`h-2.5 rounded-full bg-gradient-to-r ${severityBarGradient[sev.name] || 'from-gray-400 to-gray-300'}`}
                      style={{ width: `${Math.max(sev.pct, 2)}%`, transition: 'width 0.6s ease' }}
                    />
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="mt-4 flex h-40 items-center justify-center">
              <p className="text-sm text-gray-400 dark:text-slate-500">No severity data available.</p>
            </div>
          )}
        </div>

        {/* Top Detection Rules */}
        <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50">
          <h3 className="text-sm font-semibold text-gray-700 dark:text-slate-300">
            Top Detection Rules
          </h3>
          {topRules.length > 0 ? (
            <div className="mt-4 space-y-3">
              {topRules.map((rule, idx) => (
                <div
                  key={rule.name}
                  className="flex items-center gap-3 rounded-lg border border-gray-100 dark:border-slate-700 bg-gray-50/50 dark:bg-slate-700/30 px-4 py-3"
                >
                  <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-gray-200 dark:bg-slate-600 text-xs font-bold text-gray-600 dark:text-slate-300">
                    {idx + 1}
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-gray-900 dark:text-slate-100">
                      {rule.name}
                    </p>
                    <SeverityBadge severity={rule.severity} />
                  </div>
                  <span className="shrink-0 rounded-md bg-gray-200 dark:bg-slate-600 px-2.5 py-1 text-xs font-bold tabular-nums text-gray-700 dark:text-slate-300">
                    {rule.count}
                  </span>
                </div>
              ))}
            </div>
          ) : (
            <div className="mt-4 flex h-40 items-center justify-center">
              <p className="text-sm text-gray-400 dark:text-slate-500">No rule data yet.</p>
            </div>
          )}
        </div>
      </div>

      {/* ───── Recent Detections ───── */}
      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
        <div className="flex items-center justify-between border-b border-gray-200 dark:border-slate-700 px-6 py-4">
          <h3 className="text-sm font-semibold text-gray-700 dark:text-slate-300">
            Recent Detections
          </h3>
          {recentDets.length > 0 && (
            <button
              onClick={() => navigate('/detections')}
              className="text-xs font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300 transition-colors"
            >
              View all detections &rarr;
            </button>
          )}
        </div>
        <div className="divide-y divide-gray-100 dark:divide-slate-700">
          {recentDets.slice(0, 10).map((det) => (
            <button
              key={det.id}
              onClick={() => navigate('/detections')}
              className="flex w-full items-center gap-4 px-6 py-4 text-left transition-colors hover:bg-gray-50 dark:hover:bg-slate-700/40"
            >
              {/* Severity indicator dot */}
              <span
                className="h-2.5 w-2.5 shrink-0 rounded-full"
                style={{ backgroundColor: SEVERITY_COLORS[det.severity] || '#6b7280' }}
              />
              {/* Main info */}
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium text-gray-900 dark:text-slate-100">
                  {det.title || det.rule_name}
                </p>
                <div className="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-1">
                  <span className="text-xs text-gray-500 dark:text-slate-400">
                    {det.agent_hostname || det.agent_id}
                  </span>
                  {det.labels?.['technique.id'] && (
                    <span className="text-xs font-mono text-gray-400 dark:text-slate-500">
                      {det.labels['technique.id']}
                    </span>
                  )}
                </div>
              </div>
              {/* Severity badge */}
              <SeverityBadge severity={det.severity} />
              {/* Timestamp */}
              <span className="shrink-0 text-xs tabular-nums text-gray-400 dark:text-slate-500">
                {formatRelativeTime(det.timestamp)}
              </span>
            </button>
          ))}
          {recentDets.length === 0 && (
            <div className="px-6 py-16 text-center">
              <svg className="mx-auto h-10 w-10 text-gray-300 dark:text-slate-600" fill="none" viewBox="0 0 24 24" strokeWidth={1} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m9-.75a9 9 0 1 1-18 0 9 9 0 0 1 18 0Zm-9 3.75h.008v.008H12v-.008Z" />
              </svg>
              <p className="mt-2 text-sm text-gray-400 dark:text-slate-500">
                No detections yet. Connect agents to start monitoring.
              </p>
            </div>
          )}
        </div>
      </div>

      {/* ───── Quick Actions ───── */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <QuickAction
          title="Deploy Agent"
          description="Enroll a new endpoint"
          icon={
            <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
            </svg>
          }
          onClick={() => navigate('/settings')}
          accent="from-blue-500 to-cyan-400"
        />
        <QuickAction
          title="Create Rule"
          description="Add a detection rule"
          icon={
            <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.325.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 0 1 1.37.49l1.296 2.247a1.125 1.125 0 0 1-.26 1.431l-1.003.827c-.293.241-.438.613-.43.992a7.723 7.723 0 0 1 0 .255c-.008.378.137.75.43.991l1.004.827c.424.35.534.955.26 1.43l-1.298 2.247a1.125 1.125 0 0 1-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.47 6.47 0 0 1-.22.128c-.331.183-.581.495-.644.869l-.213 1.281c-.09.543-.56.94-1.11.94h-2.594c-.55 0-1.019-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 0 1-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 0 1-1.369-.49l-1.297-2.247a1.125 1.125 0 0 1 .26-1.431l1.004-.827c.292-.24.437-.613.43-.991a6.932 6.932 0 0 1 0-.255c.007-.38-.138-.751-.43-.992l-1.004-.827a1.125 1.125 0 0 1-.26-1.43l1.297-2.247a1.125 1.125 0 0 1 1.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.086.22-.128.332-.183.582-.495.644-.869l.214-1.28Z" />
              <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z" />
            </svg>
          }
          onClick={() => navigate('/rules')}
          accent="from-emerald-500 to-teal-400"
        />
        <QuickAction
          title="View Events"
          description="Live telemetry stream"
          icon={
            <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 3v11.25A2.25 2.25 0 0 0 6 16.5h2.25M3.75 3h-1.5m1.5 0h16.5m0 0h1.5m-1.5 0v11.25A2.25 2.25 0 0 1 18 16.5h-2.25m-7.5 0h7.5m-7.5 0-1 3m8.5-3 1 3m0 0 .5 1.5m-.5-1.5h-9.5m0 0-.5 1.5M9 11.25v-5.5a.75.75 0 0 1 1.5 0v5.5a.75.75 0 0 1-1.5 0Zm3.75-3v3a.75.75 0 0 1-1.5 0v-3a.75.75 0 0 1 1.5 0Zm2.25-1.5v4.5a.75.75 0 0 1-1.5 0v-4.5a.75.75 0 0 1 1.5 0Z" />
            </svg>
          }
          onClick={() => navigate('/events')}
          accent="from-violet-500 to-purple-400"
        />
        <QuickAction
          title="Manage Users"
          description="User & access control"
          icon={
            <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M15 19.128a9.38 9.38 0 0 0 2.625.372 9.337 9.337 0 0 0 4.121-.952 4.125 4.125 0 0 0-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 0 1 8.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0 1 11.964-3.07M12 6.375a3.375 3.375 0 1 1-6.75 0 3.375 3.375 0 0 1 6.75 0Zm8.25 2.25a2.625 2.625 0 1 1-5.25 0 2.625 2.625 0 0 1 5.25 0Z" />
            </svg>
          }
          onClick={() => navigate('/management')}
          accent="from-amber-500 to-orange-400"
        />
      </div>
    </div>
  )
}

/* ═══════════════════════════════════════════════════
   Sub-components
   ═══════════════════════════════════════════════════ */

function QuickAction({
  title,
  description,
  icon,
  onClick,
  accent,
}: {
  title: string
  description: string
  icon: React.ReactNode
  onClick: () => void
  accent: string
}) {
  return (
    <button
      onClick={onClick}
      className="group relative overflow-hidden rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-5 text-left shadow-sm transition-all hover:shadow-md hover:border-gray-300 dark:hover:border-slate-600 dark:shadow-slate-900/50"
    >
      <div className={`absolute inset-x-0 top-0 h-0.5 bg-gradient-to-r ${accent} opacity-0 transition-opacity group-hover:opacity-100`} />
      <div className={`inline-flex rounded-lg bg-gradient-to-br ${accent} p-2 text-white`}>
        {icon}
      </div>
      <p className="mt-3 text-sm font-semibold text-gray-900 dark:text-slate-100">{title}</p>
      <p className="mt-0.5 text-xs text-gray-500 dark:text-slate-400">{description}</p>
    </button>
  )
}

/* ═══════════════════════════════════════════════════
   Helpers
   ═══════════════════════════════════════════════════ */

function formatRelativeTime(ts: string): string {
  const now = Date.now()
  const then = new Date(ts).getTime()
  const diffSec = Math.floor((now - then) / 1000)
  if (diffSec < 60) return 'just now'
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`
  return `${Math.floor(diffSec / 86400)}d ago`
}
