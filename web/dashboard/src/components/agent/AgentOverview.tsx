import { useQuery } from '@tanstack/react-query'
import { api, Agent, Detection } from '../../lib/api'
import { AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer } from 'recharts'
import { RefreshCw } from 'lucide-react'
import SeverityBadge from '../SeverityBadge'

function timeAgo(date: Date): string {
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
  if (seconds < 60) return seconds + 's ago'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return minutes + 'm ago'
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return hours + 'h ago'
  return Math.floor(hours / 24) + 'd ago'
}

function Skeleton({ className = '' }: { className?: string }) {
  return <div className={`animate-pulse bg-gray-200 dark:bg-slate-700 rounded ${className}`} />
}

interface HeartbeatPoint {
  cpu_pct: number
  mem_mb: number
  events_per_sec: number
  active_rules: number
  timestamp: string
}

export default function AgentOverview({ agent }: { agent: Agent }) {
  const { data: hbRes, isLoading: hbLoading, refetch: refetchHb } = useQuery({
    queryKey: ['agent-heartbeat-history', agent.id],
    queryFn: () => api.getAgentHeartbeatHistory(agent.id, 60),
    refetchInterval: 30000,
  })

  const { data: detRes, isLoading: detLoading, refetch: refetchDet } = useQuery({
    queryKey: ['agent-detections-recent', agent.id],
    queryFn: () => api.getDetections({ agent_id: agent.id, per_page: '5' }),
    refetchInterval: 30000,
  })

  const heartbeats = (hbRes?.data || []) as HeartbeatPoint[]
  const detections = (detRes?.data || []) as Detection[]

  // Derive stats from heartbeat data
  const latestHb = heartbeats.length > 0 ? heartbeats[heartbeats.length - 1] : null
  const activeRules = latestHb?.active_rules ?? 0
  const eventsPerSec = latestHb?.events_per_sec ?? 0

  // Chart data — reverse so oldest is first (left) for time series
  const chartData = [...heartbeats].reverse().map(h => ({
    time: new Date(h.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
    cpu: h.cpu_pct,
    mem: h.mem_mb,
    eps: h.events_per_sec,
  }))

  const chartTooltipStyle = {
    contentStyle: {
      backgroundColor: '#1e293b',
      border: '1px solid #334155',
      borderRadius: '8px',
      fontSize: '12px',
      color: '#e2e8f0',
    },
    labelStyle: { color: '#94a3b8' },
  }

  return (
    <div className="space-y-6">
      {/* Status card */}
      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50">
        <div className="flex items-start justify-between">
          <div>
            <h2 className="text-2xl font-bold text-gray-900 dark:text-slate-100 tracking-tight">{agent.hostname}</h2>
            <div className="flex items-center gap-3 mt-2">
              <span className="inline-flex items-center gap-2 rounded-full px-3 py-1 text-xs font-medium bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400">
                {agent.status === 'online' && (
                  <span className="relative flex h-2 w-2">
                    <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" />
                    <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500" />
                  </span>
                )}
                {agent.status !== 'online' && (
                  <span className={`h-2 w-2 rounded-full ${agent.status === 'offline' ? 'bg-gray-400' : 'bg-amber-500'}`} />
                )}
                {agent.status}
              </span>
              <span className="text-xs text-gray-400 dark:text-slate-500 font-mono">{agent.id}</span>
            </div>
          </div>
          <button
            onClick={() => { refetchHb(); refetchDet() }}
            className="p-2 rounded-lg text-gray-400 dark:text-slate-500 hover:text-gray-600 dark:hover:text-slate-300 hover:bg-gray-100 dark:hover:bg-slate-700 transition-colors"
          >
            <RefreshCw className="w-4 h-4" />
          </button>
        </div>

        <div className="grid grid-cols-2 md:grid-cols-3 gap-4 mt-5">
          <div className="rounded-lg bg-gray-50 dark:bg-slate-900 px-4 py-3">
            <span className="text-xs text-gray-500 dark:text-slate-400">OS Version</span>
            <p className="text-sm font-medium text-gray-900 dark:text-slate-100 mt-0.5 font-mono">{agent.os_version}</p>
          </div>
          <div className="rounded-lg bg-gray-50 dark:bg-slate-900 px-4 py-3">
            <span className="text-xs text-gray-500 dark:text-slate-400">Engine Version</span>
            <p className="text-sm font-medium text-gray-900 dark:text-slate-100 mt-0.5 font-mono">{agent.engine_version}</p>
          </div>
          <div className="rounded-lg bg-gray-50 dark:bg-slate-900 px-4 py-3">
            <span className="text-xs text-gray-500 dark:text-slate-400">Group</span>
            <p className="text-sm font-medium text-gray-900 dark:text-slate-100 mt-0.5">{agent.group_name || agent.group_id || 'Default'}</p>
          </div>
          <div className="rounded-lg bg-gray-50 dark:bg-slate-900 px-4 py-3">
            <span className="text-xs text-gray-500 dark:text-slate-400">Registered</span>
            <p className="text-sm font-medium text-gray-900 dark:text-slate-100 mt-0.5">{new Date(agent.registered_at).toLocaleDateString()}</p>
          </div>
          <div className="rounded-lg bg-gray-50 dark:bg-slate-900 px-4 py-3">
            <span className="text-xs text-gray-500 dark:text-slate-400">Last Heartbeat</span>
            <p className="text-sm font-medium text-gray-900 dark:text-slate-100 mt-0.5">
              {agent.last_heartbeat ? timeAgo(new Date(agent.last_heartbeat)) : 'Never'}
            </p>
          </div>
        </div>
      </div>

      {/* Quick stats row */}
      <div className="grid grid-cols-3 gap-4">
        <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-5 shadow-sm dark:shadow-slate-900/50">
          <p className="text-xs font-medium text-gray-500 dark:text-slate-400 uppercase tracking-wide">Active Rules</p>
          {hbLoading ? <Skeleton className="h-8 w-16 mt-2" /> : (
            <p className="mt-2 text-2xl font-bold text-gray-900 dark:text-slate-100 tabular-nums">{activeRules}</p>
          )}
        </div>
        <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-5 shadow-sm dark:shadow-slate-900/50">
          <p className="text-xs font-medium text-gray-500 dark:text-slate-400 uppercase tracking-wide">Detections (24h)</p>
          {detLoading ? <Skeleton className="h-8 w-16 mt-2" /> : (
            <p className="mt-2 text-2xl font-bold text-gray-900 dark:text-slate-100 tabular-nums">{detRes?.meta?.total ?? detections.length}</p>
          )}
        </div>
        <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-5 shadow-sm dark:shadow-slate-900/50">
          <p className="text-xs font-medium text-gray-500 dark:text-slate-400 uppercase tracking-wide">Events/sec</p>
          {hbLoading ? <Skeleton className="h-8 w-16 mt-2" /> : (
            <p className="mt-2 text-2xl font-bold text-gray-900 dark:text-slate-100 tabular-nums">{eventsPerSec.toFixed(1)}</p>
          )}
        </div>
      </div>

      {/* Sparkline charts */}
      <div className="grid grid-cols-3 gap-4">
        {[
          { key: 'cpu' as const, label: 'CPU %', color: '#34d399', unit: '%' },
          { key: 'mem' as const, label: 'Memory MB', color: '#60a5fa', unit: ' MB' },
          { key: 'eps' as const, label: 'Events/sec', color: '#fbbf24', unit: '' },
        ].map(chart => (
          <div key={chart.key} className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-4 shadow-sm dark:shadow-slate-900/50">
            <p className="text-xs font-medium text-gray-500 dark:text-slate-400 mb-2">{chart.label}</p>
            {hbLoading ? <Skeleton className="h-[168px] w-full" /> : (
              <div style={{ height: 168 }}>
                <ResponsiveContainer width="100%" height="100%">
                  <AreaChart data={chartData} margin={{ top: 4, right: 4, bottom: 0, left: 0 }}>
                    <defs>
                      <linearGradient id={`grad-${chart.key}`} x1="0" y1="0" x2="0" y2="1">
                        <stop offset="5%" stopColor={chart.color} stopOpacity={0.3} />
                        <stop offset="95%" stopColor={chart.color} stopOpacity={0} />
                      </linearGradient>
                    </defs>
                    <XAxis dataKey="time" tick={{ fontSize: 10, fill: '#64748b' }} axisLine={false} tickLine={false} interval="preserveStartEnd" />
                    <YAxis hide domain={['auto', 'auto']} />
                    <Tooltip
                      {...chartTooltipStyle}
                      formatter={(value: number) => [`${value.toFixed(1)}${chart.unit}`, chart.label]}
                    />
                    <Area
                      type="monotone"
                      dataKey={chart.key}
                      stroke={chart.color}
                      strokeWidth={2}
                      fill={`url(#grad-${chart.key})`}
                      dot={false}
                      activeDot={{ r: 3, fill: chart.color }}
                    />
                  </AreaChart>
                </ResponsiveContainer>
              </div>
            )}
          </div>
        ))}
      </div>

      {/* Recent detections */}
      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
        <div className="flex items-center justify-between px-6 py-4 border-b border-gray-100 dark:border-slate-700">
          <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Recent Detections</h3>
          <button
            onClick={() => refetchDet()}
            className="p-1.5 rounded text-gray-400 dark:text-slate-500 hover:text-gray-600 dark:hover:text-slate-300 hover:bg-gray-100 dark:hover:bg-slate-700 transition-colors"
          >
            <RefreshCw className="w-3.5 h-3.5" />
          </button>
        </div>
        {detLoading ? (
          <div className="p-6 space-y-3">
            {[...Array(3)].map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}
          </div>
        ) : detections.length === 0 ? (
          <p className="text-sm text-gray-400 dark:text-slate-500 text-center py-8">No recent detections</p>
        ) : (
          <table className="w-full text-left text-sm">
            <thead className="bg-gray-50/50 dark:bg-slate-800/50">
              <tr>
                <th className="px-6 py-2.5 text-xs font-medium text-gray-500 dark:text-slate-400">Rule</th>
                <th className="px-6 py-2.5 text-xs font-medium text-gray-500 dark:text-slate-400">Severity</th>
                <th className="px-6 py-2.5 text-xs font-medium text-gray-500 dark:text-slate-400">Technique</th>
                <th className="px-6 py-2.5 text-xs font-medium text-gray-500 dark:text-slate-400 text-right">Time</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {detections.map(d => (
                <tr key={d.id} className="hover:bg-gray-50/50 dark:hover:bg-slate-700/30">
                  <td className="px-6 py-2.5 text-gray-900 dark:text-slate-100 font-medium">{d.rule_name}</td>
                  <td className="px-6 py-2.5"><SeverityBadge severity={d.severity} /></td>
                  <td className="px-6 py-2.5 text-gray-500 dark:text-slate-400 font-mono text-xs">
                    {d.labels?.['technique.id'] || '-'}
                  </td>
                  <td className="px-6 py-2.5 text-gray-500 dark:text-slate-400 text-right text-xs tabular-nums">
                    {timeAgo(new Date(d.timestamp))}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
