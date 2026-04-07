import { useState, useCallback, useMemo } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api, type Detection } from '../lib/api'
import ProcessChain from '../components/ProcessChain'

interface TelemetryEvent {
  id: number; timestamp: string; event_name: string; event_category: string
  pid: number; tid: number; process_name: string; process_exe: string
  process_cmdline: string; parent_pid: number; parent_name: string
  params: Record<string, unknown>
}

export default function ProcessTreePage() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const detectionId = params.get('detection')
  const agentId = params.get('agent')
  const pidParam = params.get('pid')
  const tsParam = params.get('ts')

  // Fetch detection process tree (stable, no refetch)
  const { data: treeRes, isLoading } = useQuery({
    queryKey: ['process-tree-page', detectionId || `${agentId}-${pidParam}-${tsParam}`],
    queryFn: async (): Promise<{ data?: unknown }> => {
      if (detectionId) {
        const r = await api.getDetectionProcessTree(detectionId)
        return { data: r.data }
      }
      const r = await api.getTelemetryProcessTree(agentId!, Number(pidParam), tsParam || new Date().toISOString())
      return { data: r.data }
    },
    enabled: !!detectionId || (!!agentId && !!pidParam),
    staleTime: Infinity,
    refetchOnWindowFocus: false,
    refetchOnMount: false,
    refetchOnReconnect: false,
  })

  // Fetch detection metadata for header
  const { data: detRes } = useQuery({
    queryKey: ['detection-meta', detectionId],
    queryFn: () => api.getDetection(detectionId!),
    enabled: !!detectionId,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  })
  const detection = detRes?.data as Detection | undefined

  // Extra events loaded via "Load parent"
  const [extraEvents, setExtraEvents] = useState<TelemetryEvent[]>([])
  const [loadingPid, setLoadingPid] = useState<number | null>(null)

  const loadContext = useCallback(async (pid: number) => {
    setLoadingPid(pid)
    try {
      if (detectionId) {
        const res = await api.getDetectionProcessContext(detectionId, pid)
        const d = (res.data || {}) as Record<string, unknown>
        if (d.events) setExtraEvents(prev => [...prev, ...(d.events as TelemetryEvent[])])
      } else if (agentId) {
        const res = await api.getTelemetryProcessTree(agentId, pid, tsParam || new Date().toISOString())
        const d = (res.data || {}) as Record<string, unknown>
        if (d.events) setExtraEvents(prev => [...prev, ...(d.events as TelemetryEvent[])])
      }
    } finally {
      setLoadingPid(null)
    }
  }, [detectionId, agentId, tsParam])

  // Combine base events + extra loaded events (dedup by id)
  const treeDataObj = (treeRes?.data || {}) as Record<string, unknown>
  const baseEvents = (treeDataObj.events || []) as TelemetryEvent[]
  const allEvents = useMemo(() => {
    if (extraEvents.length === 0) return baseEvents
    const seen = new Set<number>()
    const merged: TelemetryEvent[] = []
    for (const e of [...baseEvents, ...extraEvents]) {
      if (!seen.has(e.id)) { seen.add(e.id); merged.push(e) }
    }
    return merged
  }, [baseEvents, extraEvents])

  const focusPids = (treeDataObj.focus_pids || {}) as Record<number, boolean>
  if (pidParam) focusPids[Number(pidParam)] = true

  const title = detection?.rule_name || detection?.title || `Process Tree — PID ${pidParam}`

  return (
    <div className="flex flex-col h-[calc(100vh-4rem)]">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shrink-0">
        <div className="flex items-center gap-3">
          <button onClick={() => navigate(-1)}
            className="rounded-lg border border-gray-300 dark:border-slate-600 px-2.5 py-1.5 text-xs text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700">
            Back
          </button>
          <div>
            <h1 className="text-lg font-semibold text-gray-900 dark:text-slate-100">{title}</h1>
            {detection && (
              <p className="text-xs text-gray-500 dark:text-slate-400">
                {detection.agent_hostname} &middot; {new Date(detection.timestamp).toLocaleString()}
              </p>
            )}
          </div>
        </div>
      </div>

      {/* Full-screen process tree */}
      <div className="flex-1 min-h-0">
        {isLoading ? (
          <div className="flex items-center justify-center h-full text-sm text-gray-400">Loading process tree...</div>
        ) : (
          <ProcessChain events={allEvents} focusPids={focusPids} onLoadContext={loadContext} loadingPid={loadingPid} />
        )}
      </div>
    </div>
  )
}

