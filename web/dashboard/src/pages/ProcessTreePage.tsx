import { useState, useEffect, useMemo } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api, type Detection } from '../lib/api'
import DetectionProcessGraph from '../components/DetectionProcessGraph'

interface TelemetryEvent {
  id: number; timestamp: string; event_name: string; event_category: string
  pid: number; tid: number; process_name: string; process_exe: string
  process_cmdline: string; parent_pid: number; parent_name: string
  params: Record<string, unknown>; metadata: Record<string, unknown>
  raw_event: unknown
}

const catColors: Record<string, string> = {
  process: 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-400',
  file: 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-400',
  registry: 'bg-orange-100 text-orange-700 dark:bg-orange-900/40 dark:text-orange-400',
  net: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-400',
  image: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-400',
  dns: 'bg-teal-100 text-teal-700 dark:bg-teal-900/40 dark:text-teal-400',
  thread: 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-400',
  mem: 'bg-pink-100 text-pink-700 dark:bg-pink-900/40 dark:text-pink-400',
  handle: 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300',
}

export default function ProcessTreePage() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const detectionId = params.get('detection')
  const agentId = params.get('agent')
  const pidParam = params.get('pid')
  const tsParam = params.get('ts')

  // ── Detection mode ──
  const { data: detData } = useQuery({
    queryKey: ['detection-detail', detectionId],
    queryFn: () => api.getDetection(detectionId!),
    enabled: !!detectionId,
    staleTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
  })
  const detection = detData?.data as Detection | undefined

  // ── Event mode — fetch process tree telemetry ──
  const { data: treeData } = useQuery({
    queryKey: ['telemetry-process-tree', agentId, pidParam, tsParam],
    queryFn: () => api.getTelemetryProcessTree(agentId!, Number(pidParam), tsParam || new Date().toISOString()),
    enabled: !!agentId && !!pidParam,
    staleTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
  })

  const [selectedPid, setSelectedPid] = useState<number | null>(null)

  // For event mode, build a synthetic detection-like object so DetectionProcessGraph works
  const syntheticDetection = useMemo((): Detection | null => {
    if (detection) return null // using real detection
    if (!treeData?.data) return null
    const data = treeData.data as { events?: TelemetryEvent[] }
    if (!data.events?.length) return null
    const firstEvt = data.events[0]
    return {
      id: 'synthetic',
      org_id: '',
      agent_id: agentId || '',
      agent_hostname: firstEvt?.process_name || '',
      rule_name: `Process Tree — PID ${pidParam}`,
      rule_id: '',
      severity: 'medium',
      output: '',
      events: '[]',
      timestamp: tsParam || new Date().toISOString(),
      created_at: tsParam || new Date().toISOString(),
    } as Detection
  }, [detection, treeData, agentId, pidParam, tsParam])

  // Events for the detail panel (from either mode)
  const allEvents = useMemo((): TelemetryEvent[] => {
    if (treeData?.data) {
      const d = treeData.data as { events?: TelemetryEvent[] }
      return d.events || []
    }
    return []
  }, [treeData])

  // Events for the selected PID
  const selectedEvents = useMemo(() => {
    if (!selectedPid) return []
    return allEvents.filter(e => e.pid === selectedPid).sort((a, b) => a.timestamp.localeCompare(b.timestamp))
  }, [allEvents, selectedPid])

  const selectedProc = useMemo(() => {
    if (!selectedPid) return null
    return allEvents.find(e => e.pid === selectedPid)
  }, [allEvents, selectedPid])

  const activeDetection = detection || syntheticDetection
  const title = detection ? detection.rule_name : `Process Tree — PID ${pidParam}`

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

      {/* Main content: tree + detail panel */}
      <div className="flex flex-1 min-h-0">
        {/* Process Tree (left, takes most space) */}
        <div className="flex-1 min-w-0">
          {activeDetection ? (
            <DetectionProcessGraph detection={activeDetection} focusPid={Number(pidParam) || undefined} />
          ) : (
            <div className="flex items-center justify-center h-full text-sm text-gray-400 dark:text-slate-500">
              Loading process tree...
            </div>
          )}
        </div>

        {/* Detail Panel (right sidebar) */}
        <div className="w-96 border-l border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 overflow-y-auto shrink-0">
          {selectedPid && selectedProc ? (
            <div className="p-4 space-y-4">
              <div>
                <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">PID {selectedPid}</h3>
                <p className="text-xs font-mono text-gray-500 dark:text-slate-400 mt-1">{selectedProc.process_name}</p>
              </div>
              <div className="space-y-2">
                <DetailRow label="Executable" value={selectedProc.process_exe} mono />
                <DetailRow label="Command Line" value={selectedProc.process_cmdline} mono />
                <DetailRow label="Parent" value={`${selectedProc.parent_name} (${selectedProc.parent_pid})`} />
              </div>
              {selectedEvents.length > 0 && (
                <div>
                  <h4 className="text-xs font-semibold text-gray-700 dark:text-slate-300 mb-2">
                    Events ({selectedEvents.length})
                  </h4>
                  <div className="space-y-1 max-h-96 overflow-auto">
                    {selectedEvents.map(evt => (
                      <div key={evt.id} className="rounded-lg border border-gray-100 dark:border-slate-700 p-2 text-xs">
                        <div className="flex items-center gap-2">
                          <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${catColors[evt.event_category] || 'bg-gray-100 text-gray-600'}`}>
                            {evt.event_name}
                          </span>
                          <span className="text-gray-400 dark:text-slate-500 text-[10px]">
                            {new Date(evt.timestamp).toLocaleTimeString()}
                          </span>
                        </div>
                        {evt.params && Object.keys(evt.params).length > 0 && (
                          <div className="mt-1 space-y-0.5">
                            {Object.entries(evt.params).slice(0, 5).map(([k, v]) => (
                              <div key={k} className="flex gap-2 font-mono text-[10px]">
                                <span className="text-gray-400 dark:text-slate-500 shrink-0">{k}:</span>
                                <span className="text-gray-700 dark:text-slate-300 truncate">{String(v)}</span>
                              </div>
                            ))}
                          </div>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          ) : (
            <div className="flex items-center justify-center h-full text-sm text-gray-400 dark:text-slate-500">
              {detection ? (
                <div className="text-center space-y-2 px-6">
                  <p className="font-medium text-gray-900 dark:text-slate-100">{detection.rule_name}</p>
                  <p className="text-xs">{detection.output}</p>
                  <p className="text-[10px] text-gray-400">Click a process node to see its events</p>
                </div>
              ) : (
                <p>Click a process node to see details</p>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function DetailRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  if (!value) return null
  return (
    <div>
      <dt className="text-[10px] text-gray-400 dark:text-slate-500 uppercase tracking-wider">{label}</dt>
      <dd className={`text-xs text-gray-900 dark:text-slate-100 mt-0.5 break-all ${mono ? 'font-mono' : ''}`}>{value}</dd>
    </div>
  )
}
