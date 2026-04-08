import { useState, useEffect } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, Command } from '../../lib/api'
import { HardDrive, Play, Square, Download, RefreshCw, Clock, Filter, Loader2 } from 'lucide-react'

interface CaptureState {
  active: boolean
  pid: number | null
  path: string | null
  filter: string
  startedAt: Date | null
}

export default function AgentCaptures({ agentId }: { agentId: string }) {
  const queryClient = useQueryClient()
  const [capture, setCapture] = useState<CaptureState>({ active: false, pid: null, path: null, filter: '', startedAt: null })
  const [filterInput, setFilterInput] = useState('')
  const [durationMin, setDurationMin] = useState(5)
  const [error, setError] = useState<string | null>(null)
  const [completedCaptures, setCompletedCaptures] = useState<Array<{ path: string; startedAt: string; stoppedAt: string; filter: string }>>([])

  // Poll commands to detect capture status
  const { data: cmdRes } = useQuery({
    queryKey: ['agent-commands', agentId],
    queryFn: () => api.getAgentCommands(agentId),
    refetchInterval: 3000,
  })

  // Detect active/completed captures from command history
  useEffect(() => {
    const cmds = (cmdRes?.data || []) as Command[]
    const startCmds = cmds.filter(c => c.type === 'start_capture')
    const stopCmds = cmds.filter(c => c.type === 'stop_capture')

    // Check for active capture
    const lastStart = startCmds[0]
    if (lastStart?.status === 'completed' && lastStart.result) {
      const result = typeof lastStart.result === 'string' ? JSON.parse(lastStart.result) : lastStart.result
      const r = result as Record<string, unknown>
      if (r.started) {
        // Check if there's a matching stop after this start
        const hasStop = stopCmds.some(s => new Date(s.created_at) > new Date(lastStart.created_at) && s.status === 'completed')
        if (!hasStop) {
          setCapture({
            active: true,
            pid: r.pid as number,
            path: r.capture_path as string,
            filter: (r.filter as string) || '',
            startedAt: new Date(lastStart.created_at),
          })
          return
        }
      }
    }
    setCapture(prev => prev.active ? { ...prev, active: false } : prev)

    // Build completed captures list from command pairs
    const completed: Array<{ path: string; startedAt: string; stoppedAt: string; filter: string }> = []
    for (const start of startCmds) {
      if (start.status !== 'completed' || !start.result) continue
      const startResult = typeof start.result === 'string' ? JSON.parse(start.result) : start.result
      const sr = startResult as Record<string, unknown>
      if (!sr.started) continue
      const matchingStop = stopCmds.find(s => new Date(s.created_at) > new Date(start.created_at) && s.status === 'completed')
      if (matchingStop) {
        completed.push({
          path: sr.capture_path as string || '',
          startedAt: start.created_at,
          stoppedAt: matchingStop.created_at,
          filter: (sr.filter as string) || 'none',
        })
      }
    }
    setCompletedCaptures(completed)
  }, [cmdRes])

  const cmdMutation = useMutation({
    mutationFn: async ({ type, payload }: { type: string; payload?: Record<string, unknown> }) => {
      const res = await api.createCommand(agentId, type, payload)
      if (res.error) throw new Error(res.error.message)
      return res.data
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['agent-commands', agentId] }),
  })

  const startCapture = async () => {
    setError(null)
    try {
      await cmdMutation.mutateAsync({
        type: 'start_capture',
        payload: {
          filter: filterInput || undefined,
          duration: durationMin * 60,
        },
      })
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to start capture')
    }
  }

  const stopCapture = async () => {
    setError(null)
    if (!capture.pid) return
    try {
      await cmdMutation.mutateAsync({
        type: 'stop_capture',
        payload: { pid: capture.pid },
      })
      setCapture({ active: false, pid: null, path: null, filter: '', startedAt: null })
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to stop capture')
    }
  }

  const elapsed = capture.startedAt ? Math.floor((Date.now() - capture.startedAt.getTime()) / 1000) : 0
  const elapsedStr = `${Math.floor(elapsed / 60)}m ${elapsed % 60}s`

  return (
    <div className="space-y-6">
      {/* Capture Control */}
      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50 overflow-hidden">
        <div className="flex items-center justify-between px-6 py-4 border-b border-gray-100 dark:border-slate-700">
          <div className="flex items-center gap-2">
            <HardDrive className="w-4 h-4 text-gray-500 dark:text-slate-400" />
            <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Kernel Event Capture</h3>
          </div>
          {capture.active && (
            <div className="flex items-center gap-2">
              <span className="relative flex h-2.5 w-2.5">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75" />
                <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-red-500" />
              </span>
              <span className="text-xs font-medium text-red-600 dark:text-red-400">Recording — {elapsedStr}</span>
            </div>
          )}
        </div>

        <div className="p-6 space-y-4">
          {!capture.active ? (
            <>
              {/* Filter expression */}
              <div>
                <label className="block text-xs font-medium text-gray-700 dark:text-slate-300 mb-1.5">
                  <Filter className="w-3 h-3 inline mr-1" />
                  Filter Expression (optional)
                </label>
                <input
                  type="text" value={filterInput} onChange={e => setFilterInput(e.target.value)}
                  placeholder="e.g., spawn_process and ps.name = 'cmd.exe'"
                  className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-4 py-2.5 text-sm font-mono text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
                />
                <p className="mt-1 text-[10px] text-gray-400 dark:text-slate-500">Uses Fibratus QL filter syntax. Leave empty to capture all events.</p>
              </div>

              {/* Duration */}
              <div>
                <label className="block text-xs font-medium text-gray-700 dark:text-slate-300 mb-1.5">
                  <Clock className="w-3 h-3 inline mr-1" />
                  Duration
                </label>
                <div className="flex items-center gap-2">
                  {[1, 5, 15, 30, 60].map(m => (
                    <button key={m} onClick={() => setDurationMin(m)}
                      className={'px-3 py-1.5 rounded-lg text-xs font-medium border transition-colors ' +
                        (durationMin === m
                          ? 'border-fibratus-500 bg-fibratus-50 dark:bg-fibratus-900/30 text-fibratus-700 dark:text-fibratus-400'
                          : 'border-gray-300 dark:border-slate-600 text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700')
                      }
                    >
                      {m < 60 ? `${m}min` : '1hr'}
                    </button>
                  ))}
                </div>
              </div>

              {/* Start button */}
              <button onClick={startCapture} disabled={cmdMutation.isPending}
                className="flex items-center gap-2 px-4 py-2.5 rounded-lg bg-red-600 hover:bg-red-700 text-white text-sm font-medium disabled:opacity-50 transition-colors"
              >
                {cmdMutation.isPending ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
                Start Capture
              </button>
            </>
          ) : (
            <>
              {/* Active capture info */}
              <div className="rounded-lg bg-red-50 dark:bg-red-950/20 border border-red-200 dark:border-red-900/50 p-4 space-y-2">
                <div className="grid grid-cols-2 gap-4 text-sm">
                  <div>
                    <span className="text-[10px] uppercase tracking-wide text-red-400">Capture File</span>
                    <p className="font-mono text-xs text-red-700 dark:text-red-300 break-all">{capture.path}</p>
                  </div>
                  <div>
                    <span className="text-[10px] uppercase tracking-wide text-red-400">Process ID</span>
                    <p className="font-mono text-xs text-red-700 dark:text-red-300">{capture.pid}</p>
                  </div>
                  {capture.filter && (
                    <div className="col-span-2">
                      <span className="text-[10px] uppercase tracking-wide text-red-400">Filter</span>
                      <p className="font-mono text-xs text-red-700 dark:text-red-300">{capture.filter}</p>
                    </div>
                  )}
                </div>
              </div>

              {/* Stop button */}
              <button onClick={stopCapture} disabled={cmdMutation.isPending}
                className="flex items-center gap-2 px-4 py-2.5 rounded-lg bg-gray-900 dark:bg-gray-700 hover:bg-gray-800 dark:hover:bg-gray-600 text-white text-sm font-medium disabled:opacity-50 transition-colors"
              >
                {cmdMutation.isPending ? <Loader2 className="w-4 h-4 animate-spin" /> : <Square className="w-4 h-4" />}
                Stop Capture
              </button>
            </>
          )}

          {error && (
            <div className="rounded-lg bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 px-4 py-2 text-xs text-red-700 dark:text-red-400">{error}</div>
          )}
        </div>
      </div>

      {/* Completed Captures */}
      {completedCaptures.length > 0 && (
        <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
          <div className="flex items-center justify-between px-6 py-4 border-b border-gray-100 dark:border-slate-700">
            <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Capture History</h3>
            <span className="text-xs text-gray-400 dark:text-slate-500">{completedCaptures.length} capture(s)</span>
          </div>
          <div className="divide-y divide-gray-100 dark:divide-slate-700">
            {completedCaptures.map((cap, i) => {
              const duration = Math.floor((new Date(cap.stoppedAt).getTime() - new Date(cap.startedAt).getTime()) / 1000)
              return (
                <div key={i} className="px-6 py-4 flex items-center justify-between">
                  <div className="space-y-1 min-w-0">
                    <div className="flex items-center gap-3">
                      <HardDrive className="w-4 h-4 text-gray-400 dark:text-slate-500 shrink-0" />
                      <span className="font-mono text-xs text-gray-700 dark:text-slate-300 truncate">{cap.path}</span>
                    </div>
                    <div className="flex items-center gap-4 text-[10px] text-gray-400 dark:text-slate-500 ml-7">
                      <span>{new Date(cap.startedAt).toLocaleString()}</span>
                      <span>{Math.floor(duration / 60)}m {duration % 60}s</span>
                      {cap.filter !== 'none' && <span className="font-mono">filter: {cap.filter}</span>}
                    </div>
                  </div>
                  <button
                    onClick={async () => {
                      await api.createCommand(agentId, 'get_file', { path: cap.path })
                    }}
                    className="shrink-0 flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium border border-gray-300 dark:border-slate-600 text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700"
                  >
                    <Download className="w-3.5 h-3.5" />
                    Download
                  </button>
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
