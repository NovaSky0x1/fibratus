import { useQuery } from '@tanstack/react-query'
import { api, Agent, Detection, Command } from '../../lib/api'
import { RefreshCw, Shield, Wifi, WifiOff, Info } from 'lucide-react'
import SeverityBadge from '../SeverityBadge'
import { useState, useEffect } from 'react'

function timeAgo(date: Date): string {
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
  if (seconds < 60) return seconds + 's ago'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return minutes + 'm ago'
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return hours + 'h ago'
  return Math.floor(hours / 24) + 'd ago'
}

export default function AgentOverview({ agent }: { agent: Agent }) {

  const { data: detRes, isLoading: detLoading, refetch: refetchDet } = useQuery({
    queryKey: ['agent-detections-recent', agent.id],
    queryFn: () => api.getDetections({ agent_id: agent.id, per_page: '5' }),
    refetchInterval: 30000,
  })
  const detections = (detRes?.data || []) as Detection[]

  // System info — auto-collect on first load, then cache
  const [sysInfo, setSysInfo] = useState<Record<string, string> | null>(null)
  const [sysInfoLoading, setSysInfoLoading] = useState(false)

  const collectInfo = async () => {
    setSysInfoLoading(true)
    try {
      const resp = await api.createCommand(agent.id, 'collect_info')
      if (!resp.data?.id) return
      const cmdId = resp.data.id
      // Poll for result
      const poll = setInterval(async () => {
        const cmds = await api.getAgentCommands(agent.id)
        const cmd = (cmds.data || []).find((c: Command) => c.id === cmdId)
        if (cmd?.status === 'completed' && cmd.result) {
          clearInterval(poll)
          const result = typeof cmd.result === 'string' ? JSON.parse(cmd.result) : cmd.result
          setSysInfo(result as Record<string, string>)
          setSysInfoLoading(false)
        } else if (cmd?.status === 'failed') {
          clearInterval(poll)
          setSysInfoLoading(false)
        }
      }, 2000)
      setTimeout(() => { clearInterval(poll); setSysInfoLoading(false) }, 30000)
    } catch { setSysInfoLoading(false) }
  }

  // Auto-collect system info on first mount
  useEffect(() => { if (agent.status === 'online' && !sysInfo) collectInfo() }, []) // eslint-disable-line

  return (
    <div className="space-y-6">
      {/* Agent identity + status */}
      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50">
        <div className="flex items-start justify-between">
          <div>
            <h2 className="text-2xl font-bold text-gray-900 dark:text-slate-100 tracking-tight">{agent.hostname}</h2>
            <div className="flex items-center gap-3 mt-2 flex-wrap">
              <span className={'inline-flex items-center gap-2 rounded-full px-3 py-1 text-xs font-medium ' +
                (agent.status === 'online'
                  ? 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400'
                  : 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400')
              }>
                {agent.status === 'online' && (
                  <span className="relative flex h-2 w-2">
                    <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" />
                    <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500" />
                  </span>
                )}
                {agent.status}
              </span>

              {/* Tamper Protection */}
              <span className={'inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium ' +
                (agent.tamper_protection
                  ? 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400'
                  : 'bg-amber-50 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400')
              }>
                <Shield className="w-3 h-3" />
                Tamper {agent.tamper_protection ? 'ON' : 'OFF'}
              </span>

              {/* Network */}
              <span className={'inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium ' +
                (agent.isolated
                  ? 'bg-red-50 dark:bg-red-900/30 text-red-700 dark:text-red-400'
                  : 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400')
              }>
                {agent.isolated ? <WifiOff className="w-3 h-3" /> : <Wifi className="w-3 h-3" />}
                {agent.isolated ? 'Isolated' : 'Connected'}
              </span>
            </div>
          </div>
          <button
            onClick={() => { refetchDet(); collectInfo() }}
            className="p-2 rounded-lg text-gray-400 dark:text-slate-500 hover:text-gray-600 dark:hover:text-slate-300 hover:bg-gray-100 dark:hover:bg-slate-700 transition-colors"
          >
            <RefreshCw className="w-4 h-4" />
          </button>
        </div>

        <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mt-5">
          <div className="rounded-lg bg-gray-50 dark:bg-slate-900 px-4 py-3">
            <span className="text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">OS</span>
            <p className="text-sm font-medium text-gray-900 dark:text-slate-100 mt-0.5 font-mono">{agent.os_version}</p>
          </div>
          <div className="rounded-lg bg-gray-50 dark:bg-slate-900 px-4 py-3">
            <span className="text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">Engine</span>
            <p className="text-sm font-medium text-gray-900 dark:text-slate-100 mt-0.5 font-mono">{agent.engine_version}</p>
          </div>
          <div className="rounded-lg bg-gray-50 dark:bg-slate-900 px-4 py-3">
            <span className="text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">Registered</span>
            <p className="text-sm font-medium text-gray-900 dark:text-slate-100 mt-0.5">{new Date(agent.registered_at).toLocaleDateString()}</p>
          </div>
          <div className="rounded-lg bg-gray-50 dark:bg-slate-900 px-4 py-3">
            <span className="text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">Last Heartbeat</span>
            <p className="text-sm font-medium text-gray-900 dark:text-slate-100 mt-0.5">
              {agent.last_heartbeat ? timeAgo(new Date(agent.last_heartbeat)) : 'Never'}
            </p>
          </div>
        </div>
      </div>

      {/* System Information (from collect_info) */}
      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
        <div className="flex items-center justify-between px-6 py-4 border-b border-gray-100 dark:border-slate-700">
          <div className="flex items-center gap-2">
            <Info className="w-4 h-4 text-gray-400 dark:text-slate-500" />
            <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">System Information</h3>
          </div>
          <button
            onClick={collectInfo}
            disabled={sysInfoLoading}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700 disabled:opacity-50"
          >
            <RefreshCw className={'w-3 h-3 ' + (sysInfoLoading ? 'animate-spin' : '')} />
            {sysInfoLoading ? 'Collecting...' : 'Refresh'}
          </button>
        </div>
        {sysInfoLoading && !sysInfo && (
          <div className="p-6 space-y-2">
            {[...Array(6)].map((_, i) => <div key={i} className="h-4 rounded bg-gray-200 dark:bg-slate-700 animate-pulse" style={{ width: `${60 + Math.random() * 40}%` }} />)}
          </div>
        )}
        {sysInfo && (
          <div className="p-6 space-y-4">
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div>
                <span className="text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">Hostname</span>
                <p className="text-sm font-mono text-gray-900 dark:text-slate-100">{sysInfo.hostname}</p>
              </div>
              <div>
                <span className="text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">Architecture</span>
                <p className="text-sm font-mono text-gray-900 dark:text-slate-100">{sysInfo.arch}</p>
              </div>
              <div>
                <span className="text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">CPUs</span>
                <p className="text-sm font-mono text-gray-900 dark:text-slate-100">{sysInfo.cpus}</p>
              </div>
              <div>
                <span className="text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">OS</span>
                <p className="text-sm font-mono text-gray-900 dark:text-slate-100">{sysInfo.os}</p>
              </div>
            </div>
            {sysInfo.ip_config && (
              <div>
                <span className="text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">IP Configuration</span>
                <pre className="mt-1 text-[11px] font-mono text-gray-700 dark:text-slate-300 bg-gray-50 dark:bg-slate-900 rounded-lg p-3 max-h-40 overflow-auto whitespace-pre-wrap">{sysInfo.ip_config}</pre>
              </div>
            )}
            {sysInfo.logged_users && (
              <div>
                <span className="text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">Logged In Users</span>
                <pre className="mt-1 text-[11px] font-mono text-gray-700 dark:text-slate-300 bg-gray-50 dark:bg-slate-900 rounded-lg p-3 max-h-32 overflow-auto whitespace-pre-wrap">{sysInfo.logged_users}</pre>
              </div>
            )}
          </div>
        )}
        {!sysInfo && !sysInfoLoading && (
          <p className="text-sm text-gray-400 dark:text-slate-500 text-center py-8">Agent offline — system info unavailable</p>
        )}
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
            {[...Array(3)].map((_, i) => <div key={i} className="h-10 rounded bg-gray-200 dark:bg-slate-700 animate-pulse" />)}
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
                <tr key={d.id} className="cursor-pointer hover:bg-gray-50/50 dark:hover:bg-slate-700/30" onClick={() => window.location.href = `/detections?id=${d.id}`}>
                  <td className="px-6 py-2.5 text-gray-900 dark:text-slate-100 font-medium">{d.rule_name}</td>
                  <td className="px-6 py-2.5"><SeverityBadge severity={d.severity} /></td>
                  <td className="px-6 py-2.5 text-gray-500 dark:text-slate-400 font-mono text-xs">{d.labels?.['technique.id'] || '-'}</td>
                  <td className="px-6 py-2.5 text-gray-500 dark:text-slate-400 text-right text-xs tabular-nums">{timeAgo(new Date(d.timestamp))}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
