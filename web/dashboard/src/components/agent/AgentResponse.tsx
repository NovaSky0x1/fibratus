import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api, Agent } from '../../lib/api'
import {
  ShieldAlert, Skull, HardDrive, ScanSearch,
  Loader2, CheckCircle2, XCircle
} from 'lucide-react'

interface ActionResult {
  success: boolean
  message: string
}

function ActionCard({
  icon,
  title,
  description,
  children,
}: {
  icon: React.ReactNode
  title: string
  description: string
  children: React.ReactNode
}) {
  return (
    <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50 p-5">
      <div className="flex items-start gap-3 mb-4">
        <div className="p-2 rounded-lg bg-gray-100 dark:bg-slate-700 text-gray-600 dark:text-slate-400">
          {icon}
        </div>
        <div className="flex-1 min-w-0">
          <h4 className="text-sm font-semibold text-gray-900 dark:text-slate-100">{title}</h4>
          <p className="text-xs text-gray-500 dark:text-slate-400 mt-0.5">{description}</p>
        </div>
      </div>
      {children}
    </div>
  )
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

export default function AgentResponse({ agentId, agent }: { agentId: string; agent: Agent }) {
  const queryClient = useQueryClient()

  // Per-action state
  const [killPid, setKillPid] = useState('')
  const [killResult, setKillResult] = useState<ActionResult | null>(null)
  const [captureActive, setCaptureActive] = useState(false)
  const [captureResult, setCaptureResult] = useState<ActionResult | null>(null)
  const [yaraPid, setYaraPid] = useState('')
  const [yaraResult, setYaraResult] = useState<ActionResult | null>(null)

  const cmdMutation = useMutation({
    mutationFn: async ({ type, payload }: { type: string; payload?: Record<string, unknown> }) => {
      const res = await api.createCommand(agentId, type, payload)
      if (res.error) throw new Error(res.error.message)
      return res.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agent-commands', agentId] })
    },
  })

  const handleKillProcess = async () => {
    const pid = parseInt(killPid, 10)
    if (isNaN(pid) || pid <= 0) {
      setKillResult({ success: false, message: 'Enter a valid PID' })
      return
    }
    setKillResult(null)
    try {
      await cmdMutation.mutateAsync({ type: 'kill_process', payload: { pid } })
      setKillResult({ success: true, message: `Kill command sent for PID ${pid}` })
      setKillPid('')
    } catch (e) {
      setKillResult({ success: false, message: e instanceof Error ? e.message : 'Failed to kill process' })
    }
  }

  const handleCapture = async () => {
    setCaptureResult(null)
    const type = captureActive ? 'stop_capture' : 'start_capture'
    try {
      await cmdMutation.mutateAsync({ type })
      setCaptureActive(!captureActive)
      setCaptureResult({ success: true, message: captureActive ? 'Capture stopped' : 'Kernel capture started' })
    } catch (e) {
      setCaptureResult({ success: false, message: e instanceof Error ? e.message : 'Failed to toggle capture' })
    }
  }

  const handleYaraScan = async () => {
    const pid = parseInt(yaraPid, 10)
    if (isNaN(pid) || pid <= 0) {
      setYaraResult({ success: false, message: 'Enter a valid PID' })
      return
    }
    setYaraResult(null)
    try {
      await cmdMutation.mutateAsync({ type: 'yara_scan', payload: { pid } })
      setYaraResult({ success: true, message: `YARA scan initiated for PID ${pid}` })
      setYaraPid('')
    } catch (e) {
      setYaraResult({ success: false, message: e instanceof Error ? e.message : 'Failed to start YARA scan' })
    }
  }

  const isPending = cmdMutation.isPending

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center gap-2">
        <ShieldAlert className="w-4 h-4 text-gray-500 dark:text-slate-400" />
        <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Incident Response</h3>
        <span className="text-xs text-gray-400 dark:text-slate-500 ml-auto font-mono">{agent.hostname}</span>
      </div>

      {/* Action grid */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {/* Kill Process */}
        <ActionCard
          icon={<Skull className="w-4 h-4" />}
          title="Kill Process"
          description="Terminate a process by PID on the endpoint"
        >
          <div className="flex gap-2">
            <input
              type="number"
              value={killPid}
              onChange={e => setKillPid(e.target.value)}
              placeholder="PID"
              min={1}
              className="flex-1 rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-xs font-mono text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
            />
            <button
              onClick={handleKillProcess}
              disabled={isPending || !killPid}
              className="px-4 py-2 text-xs font-medium rounded-lg bg-red-600 text-white hover:bg-red-700 disabled:opacity-50 transition-colors"
            >
              {isPending ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : 'Kill'}
            </button>
          </div>
          <ResultBanner result={killResult} />
        </ActionCard>

        {/* Kernel Capture */}
        <ActionCard
          icon={<HardDrive className="w-4 h-4" />}
          title="Kernel Capture"
          description="Start or stop a kernel event capture (.kcap) on the endpoint"
        >
          <button
            onClick={handleCapture}
            disabled={isPending}
            className={`w-full flex items-center justify-center gap-2 px-3 py-2 text-xs font-medium rounded-lg border transition-colors disabled:opacity-50 ${
              captureActive
                ? 'border-red-300 dark:border-red-800 bg-red-50 dark:bg-red-900/30 text-red-700 dark:text-red-400 hover:bg-red-100 dark:hover:bg-red-900/50'
                : 'border-emerald-300 dark:border-emerald-800 bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400 hover:bg-emerald-100 dark:hover:bg-emerald-900/50'
            }`}
          >
            {isPending ? (
              <Loader2 className="w-3.5 h-3.5 animate-spin" />
            ) : captureActive ? (
              <>
                <span className="relative flex h-2 w-2">
                  <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75" />
                  <span className="relative inline-flex rounded-full h-2 w-2 bg-red-500" />
                </span>
                Stop Capture
              </>
            ) : (
              'Start Capture'
            )}
          </button>
          <ResultBanner result={captureResult} />
        </ActionCard>

        {/* YARA Scan */}
        <ActionCard
          icon={<ScanSearch className="w-4 h-4" />}
          title="YARA Scan"
          description="Scan a process memory with YARA signatures"
        >
          <div className="flex gap-2">
            <input
              type="number"
              value={yaraPid}
              onChange={e => setYaraPid(e.target.value)}
              placeholder="PID"
              min={1}
              className="flex-1 rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-xs font-mono text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
            />
            <button
              onClick={handleYaraScan}
              disabled={isPending || !yaraPid}
              className="px-4 py-2 text-xs font-medium rounded-lg bg-fibratus-600 text-white hover:bg-fibratus-700 disabled:opacity-50 transition-colors"
            >
              {isPending ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : 'Scan'}
            </button>
          </div>
          <ResultBanner result={yaraResult} />
        </ActionCard>
      </div>
    </div>
  )
}
