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

  // Account/org-level tamper protection enforcement
  const { data: settingsData } = useQuery({
    queryKey: ['account-settings'],
    queryFn: () => api.getAccountSettings(),
    staleTime: 60000,
  })
  const accountSettings = settingsData?.data as { tamper_protection_enabled: boolean; eventlog_enabled: boolean; latest_agent_version?: string; latest_agent_msi_url?: string; org_protection?: Array<{ id: string; name: string; tamper_protection_enabled: boolean }> } | undefined
  const tamperLockedByPolicy = !!(accountSettings?.tamper_protection_enabled || accountSettings?.org_protection?.some(o => o.id === agent.org_id && o.tamper_protection_enabled))
  const latestVersion = accountSettings?.latest_agent_version || ''
  const updateAvailable = latestVersion && agent.engine_version !== latestVersion

  // Tamper protection state
  const [tamperResult, setTamperResult] = useState<ActionResult | null>(null)
  const [tamperToggling, setTamperToggling] = useState(false)

  // Event log collection state
  const [eventLogResult, setEventLogResult] = useState<ActionResult | null>(null)
  const [eventLogToggling, setEventLogToggling] = useState(false)
  const eventLogLockedByPolicy = !!accountSettings?.eventlog_enabled
  const effectiveEventLog = agent.eventlog_collection || eventLogLockedByPolicy

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

  const handleEventLogToggle = async () => {
    setEventLogResult(null)
    setEventLogToggling(true)
    const newState = !agent.eventlog_collection
    try {
      await api.setEventLogCollection(agent.id, newState)
      setEventLogResult({ success: true, message: newState ? 'Event log collection enabled.' : 'Event log collection disabled.' })
      queryClient.invalidateQueries({ queryKey: ['agent', agent.id] })
    } catch (e) {
      setEventLogResult({ success: false, message: e instanceof Error ? e.message : 'Failed to toggle event log collection' })
    } finally {
      setEventLogToggling(false)
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
      // Poll for result by command ID (doesn't go through command history)
      const poll = setInterval(async () => {
        const cmdRes = await api.getCommand(cmdId)
        const cmd = cmdRes.data as Command | undefined
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

              {/* Event Log Collection */}
              <span className={'inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium ' +
                (effectiveEventLog
                  ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400'
                  : 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400')
              }>
                <Info className="w-3 h-3" />
                WEL {effectiveEventLog ? 'ON' : 'OFF'}
              </span>

              {/* Network / Status */}
              <span className={'inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium ' +
                (agent.isolated
                  ? 'bg-red-50 dark:bg-red-900/30 text-red-700 dark:text-red-400'
                  : agent.status === 'online'
                    ? 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400'
                    : 'bg-gray-100 dark:bg-slate-700/50 text-gray-600 dark:text-slate-400')
              }>
                {agent.isolated ? <WifiOff className="w-3 h-3" /> : <Wifi className="w-3 h-3" />}
                {agent.isolated ? 'Isolated' : agent.status === 'online' ? 'Connected' : 'Offline'}
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
            <p className="text-sm font-medium text-gray-900 dark:text-slate-100 mt-0.5 font-mono">
              {agent.engine_version}
              {updateAvailable && (
                <span className="ml-2 inline-flex items-center rounded-full bg-amber-100 dark:bg-amber-900/30 px-2 py-0.5 text-[10px] font-medium text-amber-800 dark:text-amber-400">
                  Update Available
                </span>
              )}
            </p>
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

      {/* Agent Controls — Tamper, Event Log, Isolation, Uninstall */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
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
            <div className="flex items-center gap-2">
              <span className={'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ' +
                (agent.tamper_protection
                  ? 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400'
                  : 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400')
              }>
                {agent.tamper_protection ? 'Active' : 'Inactive'}
              </span>
              {tamperLockedByPolicy && (
                <span className="text-[10px] px-1.5 py-0.5 rounded bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400 font-medium">
                  Enforced by policy
                </span>
              )}
            </div>
            <button
              role="switch"
              aria-checked={agent.tamper_protection}
              onClick={handleTamperToggle}
              disabled={tamperToggling || isPending || tamperLockedByPolicy}
              title={tamperLockedByPolicy ? 'Tamper protection is enforced by account or organization policy' : undefined}
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

        {/* Event Log Collection */}
        <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-5 shadow-sm dark:shadow-slate-900/50">
          <div className="flex items-start gap-3 mb-4">
            <div className="p-2 rounded-lg bg-gray-100 dark:bg-slate-700 text-gray-600 dark:text-slate-400">
              <Info className="w-4 h-4" />
            </div>
            <div className="flex-1 min-w-0">
              <h4 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Event Log Collection</h4>
              <p className="text-xs text-gray-500 dark:text-slate-400 mt-0.5">Collect Windows Event Logs (Security, Sysmon, etc.)</p>
            </div>
          </div>
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <span className={'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ' +
                (effectiveEventLog
                  ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400'
                  : 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400')
              }>
                {effectiveEventLog ? 'Active' : 'Inactive'}
              </span>
              {eventLogLockedByPolicy && (
                <span className="text-[10px] px-1.5 py-0.5 rounded bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400 font-medium">
                  Enforced by policy
                </span>
              )}
            </div>
            <button
              role="switch"
              aria-checked={effectiveEventLog}
              onClick={handleEventLogToggle}
              disabled={eventLogToggling || isPending || eventLogLockedByPolicy}
              title={eventLogLockedByPolicy ? 'Event log collection is enforced by account policy' : undefined}
              className={'relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 dark:focus:ring-offset-slate-800 disabled:opacity-50 disabled:cursor-not-allowed ' +
                (effectiveEventLog ? 'bg-blue-500' : 'bg-gray-300 dark:bg-slate-600')
              }
            >
              <span
                className={'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ' +
                  (effectiveEventLog ? 'translate-x-5' : 'translate-x-0')
                }
              />
            </button>
          </div>
          <ResultBanner result={eventLogResult} />
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
                : agent.status === 'online'
                  ? 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400'
                  : 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400')
            }>
              {agent.isolated ? 'Isolated' : agent.status === 'online' ? 'Connected' : 'Offline'}
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
            {sysInfo.ip_config && (() => {
              let adapters: Array<{name: string; description: string; status: string; mac: string; speed_mbps: number; ipv4: string[]; ipv6: string[]; dns: string[]; dhcp: string}> = []
              try { const p = JSON.parse(sysInfo.ip_config); adapters = Array.isArray(p) ? p : p ? [p] : [] } catch { /* raw text fallback below */ }
              return adapters.length > 0 ? (
                <div>
                  <span className="text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">Network Adapters</span>
                  <div className="mt-2 space-y-3">
                    {adapters.map((a, i) => (
                      <div key={i} className="rounded-lg border border-gray-200 dark:border-slate-700 bg-gray-50 dark:bg-slate-900 p-3">
                        <div className="flex items-center justify-between mb-2">
                          <div>
                            <span className="text-sm font-medium text-gray-900 dark:text-slate-100">{a.name}</span>
                            <span className="ml-2 text-xs text-gray-500 dark:text-slate-400">{a.description}</span>
                          </div>
                          <span className={'inline-flex rounded-full px-2 py-0.5 text-[10px] font-medium ' +
                            (a.status === 'Up' ? 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400'
                              : 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400')
                          }>{a.status}</span>
                        </div>
                        <div className="grid grid-cols-2 md:grid-cols-3 gap-x-4 gap-y-1 text-xs">
                          {a.mac && <div><span className="text-gray-400 dark:text-slate-500">MAC</span> <span className="font-mono text-gray-700 dark:text-slate-300">{a.mac}</span></div>}
                          {a.ipv4?.length > 0 && <div><span className="text-gray-400 dark:text-slate-500">IPv4</span> <span className="font-mono text-gray-700 dark:text-slate-300">{a.ipv4.join(', ')}</span></div>}
                          {a.ipv6?.length > 0 && <div className="col-span-2"><span className="text-gray-400 dark:text-slate-500">IPv6</span> <span className="font-mono text-gray-700 dark:text-slate-300 break-all">{a.ipv6.join(', ')}</span></div>}
                          {a.dns?.length > 0 && <div><span className="text-gray-400 dark:text-slate-500">DNS</span> <span className="font-mono text-gray-700 dark:text-slate-300">{a.dns.join(', ')}</span></div>}
                          {a.dhcp && <div><span className="text-gray-400 dark:text-slate-500">DHCP</span> <span className="font-mono text-gray-700 dark:text-slate-300">{a.dhcp}</span></div>}
                          {a.speed_mbps > 0 && <div><span className="text-gray-400 dark:text-slate-500">Speed</span> <span className="font-mono text-gray-700 dark:text-slate-300">{a.speed_mbps >= 1000 ? (a.speed_mbps/1000)+'Gbps' : a.speed_mbps+'Mbps'}</span></div>}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              ) : (
                <div>
                  <span className="text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">IP Configuration</span>
                  <pre className="mt-1 text-[11px] font-mono text-gray-700 dark:text-slate-300 bg-gray-50 dark:bg-slate-900 rounded-lg p-3 max-h-64 overflow-auto whitespace-pre-wrap">{sysInfo.ip_config}</pre>
                </div>
              )
            })()}
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
            {sysInfo.security_software && sysInfo.security_software !== '[]' && sysInfo.security_software !== 'null' && (() => {
              let avs: Array<{displayName: string; enabled: number; up_to_date: number}> = []
              try { const p = JSON.parse(sysInfo.security_software); avs = Array.isArray(p) ? p : p ? [p] : [] } catch {}
              return avs.length > 0 ? (
                <div>
                  <span className="text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">Security Software</span>
                  <div className="mt-2 space-y-2">
                    {avs.map((av, i) => (
                      <div key={i} className="flex items-center justify-between rounded-lg bg-gray-50 dark:bg-slate-900 px-4 py-3">
                        <div className="flex items-center gap-3">
                          <Shield className="w-4 h-4 text-emerald-500" />
                          <span className="text-sm font-medium text-gray-900 dark:text-slate-100">{av.displayName}</span>
                        </div>
                        <div className="flex items-center gap-2">
                          <span className={'inline-flex rounded-full px-2 py-0.5 text-[10px] font-medium ' +
                            (av.enabled ? 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400' : 'bg-red-50 dark:bg-red-900/30 text-red-700 dark:text-red-400')
                          }>{av.enabled ? 'Enabled' : 'Disabled'}</span>
                          <span className={'inline-flex rounded-full px-2 py-0.5 text-[10px] font-medium ' +
                            (av.up_to_date ? 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400' : 'bg-amber-50 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400')
                          }>{av.up_to_date ? 'Up to date' : 'Out of date'}</span>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              ) : null
            })()}
            {sysInfo.installed_rmms && sysInfo.installed_rmms !== '[]' && sysInfo.installed_rmms !== 'null' && (() => {
              let rmms: Array<{name: string; service: string; status: string; running: boolean}> = []
              try { const p = JSON.parse(sysInfo.installed_rmms); rmms = Array.isArray(p) ? p : p ? [p] : [] } catch {}
              return rmms.length > 0 ? (
                <div>
                  <span className="text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">Installed RMM Tools</span>
                  <div className="mt-2 rounded-lg border border-gray-200 dark:border-slate-700 overflow-hidden">
                    <table className="w-full text-sm">
                      <thead className="bg-gray-50 dark:bg-slate-900">
                        <tr>
                          <th className="px-4 py-2 text-left text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">Name</th>
                          <th className="px-4 py-2 text-left text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">Service</th>
                          <th className="px-4 py-2 text-left text-[10px] uppercase tracking-wide text-gray-400 dark:text-slate-500">Status</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
                        {rmms.map((rmm, i) => (
                          <tr key={i}>
                            <td className="px-4 py-2 font-medium text-gray-900 dark:text-slate-100">{rmm.name}</td>
                            <td className="px-4 py-2 text-gray-600 dark:text-slate-400 font-mono text-xs">{rmm.service || '-'}</td>
                            <td className="px-4 py-2">
                              <span className={'inline-flex rounded-full px-2 py-0.5 text-[10px] font-medium ' +
                                (rmm.running ? 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400' : 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400')
                              }>{rmm.running ? 'Running' : rmm.status || 'Installed'}</span>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              ) : null
            })()}
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
