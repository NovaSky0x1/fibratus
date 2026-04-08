import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, Agent, Detection, Command } from '../../lib/api'
import {
  RefreshCw, Shield, ShieldOff, ShieldAlert, Wifi, WifiOff, Info, Trash2,
  Loader2, CheckCircle2, XCircle
} from 'lucide-react'
import SeverityBadge from '../SeverityBadge'
import { useState, useEffect } from 'react'

interface ActionResult {
  success: boolean
  message: string
}

function timeAgo(date: Date): string {
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
  if (seconds < 60) return seconds + 's ago'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return minutes + 'm ago'
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return hours + 'h ago'
  return Math.floor(hours / 24) + 'd ago'
}

function ResultBanner({ result }: { result: ActionResult | null }) {
  if (!result) return null
  return (
    <div className={`mt-3 flex items-center gap-2 px-3 py-2 rounded-lg text-xs font-medium ${
      result.success
        ? 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400 border border-emerald-200 dark:border-emerald-800'
        : 'bg-red-50 dark:bg-red-900/30 text-red-700 dark:text-red-400 border border-red-200 dark:border-red-800'
    }`}>
      {result.success ? <CheckCircle2 className="w-3.5 h-3.5 shrink-0" /> : <XCircle className="w-3.5 h-3.5 shrink-0" />}
      {result.message}
    </div>
  )
}

export default function AgentOverview({ agent }: { agent: Agent }) {
  const queryClient = useQueryClient()

  const { data: detRes, isLoading: detLoading, refetch: refetchDet } = useQuery({
    queryKey: ['agent-detections-recent', agent.id],
    queryFn: () => api.getDetections({ agent_id: agent.id, per_page: '5' }),
    refetchInterval: 30000,
  })
  const detections = (detRes?.data || []) as Detection[]

  // System info — auto-collect on first load, then cache
  const [sysInfo, setSysInfo] = useState<Record<string, string> | null>(null)
  const [sysInfoLoading, setSysInfoLoading] = useState(false)

  // Tamper protection state
  const [tamperResult, setTamperResult] = useState<ActionResult | null>(null)
  const [tamperToggling, setTamperToggling] = useState(false)

  // Network isolation state
  const [isolationResult, setIsolationResult] = useState<ActionResult | null>(null)
  const [whitelistIps, setWhitelistIps] = useState('')

  // Uninstall state
  const [uninstallConfirm, setUninstallConfirm] = useState(false)
  const [uninstallResult, setUninstallResult] = useState<ActionResult | null>(null)

  const cmdMutation = useMutation({
    mutationFn: async ({ type, payload }: { type: string; payload?: Record<string, unknown> }) => {
      const res = await api.createCommand(agent.id, type, payload)
      if (res.error) throw new Error(res.error.message)
      return res.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agent-commands', agent.id] })
    },
  })

  const isPending = cmdMutation.isPending

  const handleTamperToggle = async () => {
    setTamperResult(null)
    setTamperToggling(true)
    const newState = !agent.tamper_protection
    try {
      await api.setTamperProtection(agent.id, newState)
      await cmdMutation.mutateAsync({ type: 'set_tamper_protection', payload: { enabled: newState } })
      setTamperResult({ success: true, message: newState ? 'Tamper protection enabled.' : 'Tamper protection disabled.' })
      queryClient.invalidateQueries({ queryKey: ['agent', agent.id] })
    } catch (e) {
      setTamperResult({ success: false, message: e instanceof Error ? e.message : 'Failed to toggle tamper protection' })
    } finally {
      setTamperToggling(false)
    }
  }

  const handleIsolate = async () => {
    setIsolationResult(null)
    const ips = whitelistIps.split(',').map(s => s.trim()).filter(Boolean)
    try {
      await cmdMutation.mutateAsync({ type: 'isolate', payload: ips.length > 0 ? { whitelist_ips: ips } : undefined })
      setIsolationResult({ success: true, message: 'Network isolation command sent. Agent will be isolated shortly.' })
    } catch (e) {
      setIsolationResult({ success: false, message: e instanceof Error ? e.message : 'Failed to send isolation command' })
    }
  }

  const handleUnisolate = async () => {
    setIsolationResult(null)
    try {
      await cmdMutation.mutateAsync({ type: 'unisolate' })
      setIsolationResult({ success: true, message: 'Unisolation command sent. Network access will be restored shortly.' })
    } catch (e) {
      setIsolationResult({ success: false, message: e instanceof Error ? e.message : 'Failed to send unisolation command' })
    }
  }

  const handleUninstall = async () => {
    setUninstallResult(null)
    try {
      await cmdMutation.mutateAsync({ type: 'uninstall' })
      await api.deleteAgent(agent.id)
      setUninstallResult({ success: true, message: 'Agent uninstalled and removed. Redirecting...' })
      setUninstallConfirm(false)
      setTimeout(() => { window.location.href = '/agents' }, 2000)
    } catch (e) {
      setUninstallResult({ success: false, message: e instanceof Error ? e.message : 'Failed to uninstall agent' })
    }
  }

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

      {/* Agent Controls — Tamper, Isolation, Uninstall */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {/* Tamper Protection */}
        <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-5 shadow-sm dark:shadow-slate-900/50">
          <div className="flex items-start gap-3 mb-4">
            <div className="p-2 rounded-lg bg-gray-100 dark:bg-slate-700 text-gray-600 dark:text-slate-400">
              <Shield className="w-4 h-4" />
            </div>
            <div className="flex-1 min-w-0">
              <h4 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Tamper Protection</h4>
              <p className="text-xs text-gray-500 dark:text-slate-400 mt-0.5">Prevent unauthorized modification or termination</p>
            </div>
          </div>
          <div className="flex items-center justify-between">
            <span className={'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ' +
              (agent.tamper_protection
                ? 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400'
                : 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400')
            }>
              {agent.tamper_protection ? 'Active' : 'Inactive'}
            </span>
            <button
              role="switch"
              aria-checked={agent.tamper_protection}
              onClick={handleTamperToggle}
              disabled={tamperToggling || isPending}
              className={'relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-fibratus-500 focus:ring-offset-2 dark:focus:ring-offset-slate-800 disabled:opacity-50 disabled:cursor-not-allowed ' +
                (agent.tamper_protection ? 'bg-emerald-500' : 'bg-gray-300 dark:bg-slate-600')
              }
            >
              <span
                className={'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ' +
                  (agent.tamper_protection ? 'translate-x-5' : 'translate-x-0')
                }
              />
            </button>
          </div>
          <ResultBanner result={tamperResult} />
        </div>

        {/* Network Isolation */}
        <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-5 shadow-sm dark:shadow-slate-900/50">
          <div className="flex items-start gap-3 mb-4">
            <div className="p-2 rounded-lg bg-gray-100 dark:bg-slate-700 text-gray-600 dark:text-slate-400">
              <ShieldAlert className="w-4 h-4" />
            </div>
            <div className="flex-1 min-w-0">
              <h4 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Network Isolation</h4>
              <p className="text-xs text-gray-500 dark:text-slate-400 mt-0.5">Isolate or restore network connectivity via WFP</p>
            </div>
          </div>
          <div className="flex items-center gap-2 mb-3">
            <span className={'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ' +
              (agent.isolated
                ? 'bg-red-50 dark:bg-red-900/30 text-red-700 dark:text-red-400'
                : 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400')
            }>
              {agent.isolated ? 'Isolated' : 'Connected'}
            </span>
          </div>
          <div className="flex gap-2">
            <button
              onClick={handleIsolate}
              disabled={isPending || agent.isolated}
              className="flex-1 flex items-center justify-center gap-2 px-3 py-2 text-xs font-medium rounded-lg border border-red-300 dark:border-red-800 bg-red-50 dark:bg-red-900/30 text-red-700 dark:text-red-400 hover:bg-red-100 dark:hover:bg-red-900/50 disabled:opacity-50 transition-colors"
            >
              {isPending ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <ShieldAlert className="w-3.5 h-3.5" />}
              Isolate
            </button>
            <button
              onClick={handleUnisolate}
              disabled={isPending || !agent.isolated}
              className="flex-1 flex items-center justify-center gap-2 px-3 py-2 text-xs font-medium rounded-lg border border-emerald-300 dark:border-emerald-800 bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400 hover:bg-emerald-100 dark:hover:bg-emerald-900/50 disabled:opacity-50 transition-colors"
            >
              {isPending ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <ShieldOff className="w-3.5 h-3.5" />}
              Unisolate
            </button>
          </div>
          <div className="mt-3">
            <label className="block text-xs text-gray-500 dark:text-slate-400 mb-1">Whitelist IPs (comma-separated)</label>
            <input
              type="text"
              value={whitelistIps}
              onChange={e => setWhitelistIps(e.target.value)}
              placeholder="10.0.0.5, 192.168.1.100"
              className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-xs font-mono text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
            />
          </div>
          <ResultBanner result={isolationResult} />
        </div>

        {/* Uninstall Agent */}
        <div className="rounded-xl border border-red-200 dark:border-red-900/50 bg-red-50/50 dark:bg-red-950/20 p-5 shadow-sm dark:shadow-slate-900/50">
          <div className="flex items-start gap-3 mb-4">
            <div className="p-2 rounded-lg bg-red-100 dark:bg-red-900/40 text-red-600 dark:text-red-400">
              <Trash2 className="w-4 h-4" />
            </div>
            <div className="flex-1 min-w-0">
              <h4 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Uninstall Agent</h4>
              <p className="text-xs text-gray-500 dark:text-slate-400 mt-0.5">Permanently remove the agent from this endpoint</p>
            </div>
          </div>
          {agent.tamper_protection ? (
            <div className="flex items-center gap-2 px-3 py-2 rounded-lg border border-amber-300 dark:border-amber-800 bg-amber-50 dark:bg-amber-900/20 text-amber-700 dark:text-amber-400">
              <Shield className="w-3.5 h-3.5 shrink-0" />
              <p className="text-xs font-medium">Tamper protection must be disabled before uninstalling.</p>
            </div>
          ) : !uninstallConfirm ? (
            <button
              onClick={() => setUninstallConfirm(true)}
              disabled={isPending}
              className="w-full flex items-center justify-center gap-2 px-3 py-2 text-xs font-medium rounded-lg bg-red-600 text-white hover:bg-red-700 disabled:opacity-50 transition-colors"
            >
              <Trash2 className="w-3.5 h-3.5" />
              Uninstall Agent
            </button>
          ) : (
            <div className="space-y-2">
              <p className="text-xs text-red-600 dark:text-red-400 font-medium">
                Are you sure? This will permanently remove the agent from {agent.hostname}. This action cannot be undone.
              </p>
              <div className="flex gap-2">
                <button
                  onClick={handleUninstall}
                  disabled={isPending}
                  className="flex-1 flex items-center justify-center gap-2 px-3 py-2 text-xs font-medium rounded-lg bg-red-600 text-white hover:bg-red-700 disabled:opacity-50 transition-colors"
                >
                  {isPending ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : 'Confirm Uninstall'}
                </button>
                <button
                  onClick={() => setUninstallConfirm(false)}
                  className="flex-1 px-3 py-2 text-xs font-medium rounded-lg border border-gray-300 dark:border-slate-600 text-gray-700 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-700 transition-colors"
                >
                  Cancel
                </button>
              </div>
            </div>
          )}
          <ResultBanner result={uninstallResult} />
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
                <pre className="mt-1 text-[11px] font-mono text-gray-700 dark:text-slate-300 bg-gray-50 dark:bg-slate-900 rounded-lg p-3 max-h-64 overflow-auto whitespace-pre-wrap">{sysInfo.ip_config}</pre>
              </div>
            )}
            {sysInfo.logged_users && (
              <div>
                <span className="text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">Logged In Users</span>
                <div className="mt-1 rounded-lg bg-gray-50 dark:bg-slate-900 p-3">
                  {sysInfo.logged_users.trim().split('\n').map((line, i) => {
                    const fields = line.trim().split(/\s+/)
                    const isHeader = i === 0
                    const sessionId = !isHeader && fields.length > 2 ? fields[2] : null
                    return (
                      <div key={i} className={'flex items-center justify-between py-0.5 ' + (isHeader ? 'border-b border-gray-200 dark:border-slate-700 mb-1 pb-1' : '')}>
                        <pre className="text-[11px] font-mono text-gray-700 dark:text-slate-300 whitespace-pre">{line}</pre>
                        {!isHeader && sessionId && /^\d+$/.test(sessionId) && (
                          <button
                            onClick={async () => {
                              await cmdMutation.mutateAsync({ type: 'logoff_user', payload: { session_id: sessionId } })
                              setTimeout(collectInfo, 3000)
                            }}
                            disabled={isPending}
                            className="ml-2 shrink-0 rounded px-2 py-0.5 text-[10px] font-medium text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-900/20 hover:bg-red-100 dark:hover:bg-red-900/40 border border-red-200 dark:border-red-800 disabled:opacity-50"
                          >
                            Logoff
                          </button>
                        )}
                      </div>
                    )
                  })}
                </div>
              </div>
            )}
            {sysInfo.security_software && (
              <div>
                <span className="text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">Security Software</span>
                <pre className="mt-1 text-[11px] font-mono text-gray-700 dark:text-slate-300 bg-gray-50 dark:bg-slate-900 rounded-lg p-3 max-h-48 overflow-auto whitespace-pre-wrap">{sysInfo.security_software}</pre>
              </div>
            )}
            {sysInfo.installed_rmms && (
              <div>
                <span className="text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">Installed RMMs</span>
                <pre className="mt-1 text-[11px] font-mono text-gray-700 dark:text-slate-300 bg-gray-50 dark:bg-slate-900 rounded-lg p-3 max-h-48 overflow-auto whitespace-pre-wrap">{sysInfo.installed_rmms}</pre>
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
