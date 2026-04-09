import { useState, useEffect, useRef, useCallback, useMemo } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, Capture, CaptureEvent } from '../../lib/api'
import {
  HardDrive, Play, Square, Search, Filter, Loader2, Trash2,
  Clock, ArrowDown, X, Eye, RotateCw, ChevronDown,
} from 'lucide-react'

// ═════════════════════════════════════════════════
// Quick filter presets — real Fibratus QL syntax
// ═════════════════════════════════════════════════

const quickFilters = [
  { label: 'All Events', filter: '', desc: 'Capture everything (high volume)' },
  { label: 'Process Activity', filter: 'spawn_process or terminate_process', desc: 'Process creation and termination' },
  { label: 'Suspicious Spawns', filter: "spawn_process and (ps.parent.name imatches '(?i)winword|excel|powerpnt|outlook|acrobat' or ps.name imatches '(?i)cmd|powershell|pwsh|wscript|cscript|mshta|certutil|bitsadmin|rundll32')", desc: 'Office children, LOLBins' },
  { label: 'PowerShell', filter: "spawn_process and ps.name imatches '(?i)powershell|pwsh'", desc: 'PowerShell execution' },
  { label: 'Network Connections', filter: 'connect_process or accept_process', desc: 'Outbound + inbound TCP/UDP' },
  { label: 'DNS Queries', filter: 'query_dns', desc: 'All DNS lookups' },
  { label: 'File Mutations', filter: 'create_file or delete_file or rename_file', desc: 'File creates, deletes, renames' },
  { label: 'Registry Changes', filter: 'set_reg_value or create_reg_key or delete_reg_key', desc: 'Registry writes and key ops' },
  { label: 'DLL Loads', filter: 'load_image', desc: 'Module/DLL loading' },
  { label: 'Credential Access', filter: "spawn_process and ps.cmdline imatches '(?i)lsass|sam|ntds|credential|mimikatz|sekurlsa'", desc: 'LSASS access, credential tools' },
  { label: 'Defense Evasion', filter: "spawn_process and ps.name imatches '(?i)reg|attrib|icacls|takeown|sc|bcdedit|wevtutil'", desc: 'Common defense evasion binaries' },
  { label: 'Lateral Movement', filter: "spawn_process and ps.name imatches '(?i)psexec|wmic|winrm|mstsc|net'", desc: 'Remote execution and admin tools' },
]

// ═════════════════════════════════════════════════
// Format events to match local `fibratus run` output
// Template: {{ .Seq }} {{ .Timestamp }} - {{ .CPU }} {{ .Process }} ({{ .Pid }}) - {{ .Type }} ({{ .Params }})
// ═════════════════════════════════════════════════

function formatEventLine(evt: CaptureEvent): string {
  const ts = evt.timestamp ? new Date(evt.timestamp).toISOString() : ''
  const proc = evt.process_name || '?'
  const pid = evt.pid || 0
  const name = evt.event_name || '?'
  const params = formatParams(evt.params)
  return `${evt.seq} ${ts} ${proc} (${pid}) - ${name} (${params})`
}

function formatParams(params: Record<string, unknown> | null | undefined): string {
  if (!params || typeof params !== 'object') return ''
  const entries = Object.entries(params)
    .filter(([k]) => !k.startsWith('_')) // skip internal fields
    .sort(([a], [b]) => a.localeCompare(b))
  if (entries.length === 0) return ''
  return entries.map(([k, v]) => {
    const val = typeof v === 'object' ? JSON.stringify(v) : String(v ?? '')
    return `${k}\u27A0 ${val}`
  }).join(', ')
}

function formatElapsed(startedAt: string): string {
  const elapsed = Math.floor((Date.now() - new Date(startedAt).getTime()) / 1000)
  const m = Math.floor(elapsed / 60)
  const s = elapsed % 60
  return `${m}m ${s}s`
}

function formatDuration(start: string, end: string | null): string {
  if (!end) return '\u2014'
  const sec = Math.floor((new Date(end).getTime() - new Date(start).getTime()) / 1000)
  if (sec < 60) return `${sec}s`
  return `${Math.floor(sec / 60)}m ${sec % 60}s`
}

// ═════════════════════════════════════════════════
// Main component
// ═════════════════════════════════════════════════

export default function AgentCaptures({ agentId }: { agentId: string }) {
  const queryClient = useQueryClient()
  const [view, setView] = useState<'control' | 'live' | 'history' | 'browse'>('control')
  const [filterInput, setFilterInput] = useState('')
  const [durationMin, setDurationMin] = useState(0)
  const [showQuickFilters, setShowQuickFilters] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [autoScroll, setAutoScroll] = useState(true)
  const [browsingCapture, setBrowsingCapture] = useState<Capture | null>(null)
  const [browseSearch, setBrowseSearch] = useState('')
  const [browseAfter, setBrowseAfter] = useState(0)
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null)
  const terminalRef = useRef<HTMLDivElement>(null)
  const lastEventIdRef = useRef(0)
  const [searchTerm, setSearchTerm] = useState('')

  // ── Queries ─────────────────────────────────

  const { data: capturesRes, refetch: refetchCaptures } = useQuery({
    queryKey: ['captures', agentId],
    queryFn: () => api.listCaptures(agentId),
    refetchInterval: 5000,
  })
  const captures = (capturesRes?.data || []) as Capture[]
  const activeCapture = captures.find(c => c.status === 'active')
  const completedCaptures = captures.filter(c => c.status !== 'active')

  // Live event polling
  const { data: liveEventsRes } = useQuery({
    queryKey: ['capture-events-live', activeCapture?.id, lastEventIdRef.current],
    queryFn: () => {
      if (!activeCapture) return { data: [] }
      return api.getCaptureEvents(activeCapture.id, {
        after_id: String(lastEventIdRef.current),
        limit: '500',
      })
    },
    enabled: !!activeCapture,
    refetchInterval: activeCapture ? 2000 : false,
  })

  const [liveEvents, setLiveEvents] = useState<CaptureEvent[]>([])

  useEffect(() => {
    const newEvents = (liveEventsRes?.data || []) as CaptureEvent[]
    if (newEvents.length > 0) {
      setLiveEvents(prev => {
        const merged = [...prev, ...newEvents]
        return merged.length > 5000 ? merged.slice(-5000) : merged
      })
      lastEventIdRef.current = newEvents[newEvents.length - 1].id
    }
  }, [liveEventsRes])

  useEffect(() => {
    if (activeCapture && view === 'control') {
      setView('live')
      setLiveEvents([])
      lastEventIdRef.current = 0
    }
    if (!activeCapture && view === 'live') {
      setView('control')
    }
  }, [activeCapture?.id])

  useEffect(() => {
    if (autoScroll && terminalRef.current) {
      terminalRef.current.scrollTop = terminalRef.current.scrollHeight
    }
  }, [liveEvents, autoScroll])

  const { data: activeCaptureDetail } = useQuery({
    queryKey: ['capture-detail', activeCapture?.id],
    queryFn: () => activeCapture ? api.getCapture(activeCapture.id) : null,
    enabled: !!activeCapture,
    refetchInterval: 3000,
  })
  const captureDetail = (activeCaptureDetail?.data || activeCapture) as Capture | undefined

  // Browse events for completed captures
  const { data: browseEventsRes, isLoading: browseLoading } = useQuery({
    queryKey: ['capture-events-browse', browsingCapture?.id, browseAfter],
    queryFn: () => {
      if (!browsingCapture) return { data: [] }
      return api.getCaptureEvents(browsingCapture.id, {
        limit: '10000',
        ...(browseAfter > 0 ? { after_id: String(browseAfter) } : {}),
      })
    },
    enabled: !!browsingCapture,
  })
  const browseEvents = (browseEventsRes?.data || []) as CaptureEvent[]

  // Format all lines for terminal display + search
  const liveLines = useMemo(() =>
    liveEvents.map(evt => ({ id: evt.id, line: formatEventLine(evt) })),
    [liveEvents]
  )

  const browseLines = useMemo(() =>
    browseEvents.map(evt => ({ id: evt.id, line: formatEventLine(evt) })),
    [browseEvents]
  )

  // Client-side search filter
  const filteredLiveLines = useMemo(() => {
    if (!searchTerm) return liveLines
    const lower = searchTerm.toLowerCase()
    return liveLines.filter(l => l.line.toLowerCase().includes(lower))
  }, [liveLines, searchTerm])

  const filteredBrowseLines = useMemo(() => {
    if (!browseSearch) return browseLines
    const lower = browseSearch.toLowerCase()
    return browseLines.filter(l => l.line.toLowerCase().includes(lower))
  }, [browseLines, browseSearch])

  // ── Mutations ────────────────────────────────

  const startMutation = useMutation({
    mutationFn: async () => {
      const res = await api.createCapture(agentId, {
        filter: filterInput || undefined,
        duration_sec: durationMin > 0 ? durationMin * 60 : 0,
      })
      if (res.error) throw new Error(res.error.message)
      return res.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['captures', agentId] })
      setError(null)
    },
    onError: (e: Error) => setError(e.message),
  })

  const stopMutation = useMutation({
    mutationFn: async () => {
      if (!activeCapture) return
      const res = await api.stopCapture(activeCapture.id)
      if (res.error) throw new Error(res.error.message)
      return res.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['captures', agentId] })
      setError(null)
    },
    onError: (e: Error) => setError(e.message),
  })

  const deleteMutation = useMutation({
    mutationFn: async (captureId: string) => {
      const res = await api.deleteCapture(captureId)
      if (res.error) throw new Error(res.error.message)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['captures', agentId] })
      setDeleteConfirm(null)
      if (browsingCapture) {
        setBrowsingCapture(null)
        setView('history')
      }
    },
  })

  const handleBrowse = useCallback((cap: Capture) => {
    setBrowsingCapture(cap)
    setBrowseSearch('')
    setBrowseAfter(0)
    setView('browse')
  }, [])

  // ═════════════════════════════════════════════════
  // RENDER
  // ═════════════════════════════════════════════════

  return (
    <div className="space-y-4">
      {/* View tabs */}
      <div className="flex items-center gap-1">
        <TabButton active={view === 'control' || view === 'live'} onClick={() => setView(activeCapture ? 'live' : 'control')}>
          <HardDrive className="w-3.5 h-3.5" />
          {activeCapture ? 'Live Capture' : 'New Capture'}
        </TabButton>
        <TabButton active={view === 'history' || view === 'browse'} onClick={() => { setView('history'); setBrowsingCapture(null) }}>
          <Clock className="w-3.5 h-3.5" />
          History
          {completedCaptures.length > 0 && (
            <span className="ml-1 px-1.5 py-0.5 text-[10px] rounded-full bg-slate-700 text-slate-300">{completedCaptures.length}</span>
          )}
        </TabButton>
      </div>

      {error && (
        <div className="rounded-lg bg-red-900/20 border border-red-800 px-4 py-2 text-xs text-red-400 flex items-center justify-between">
          {error}
          <button onClick={() => setError(null)}><X className="w-3 h-3" /></button>
        </div>
      )}

      {/* ── NEW CAPTURE ────────────────────────── */}
      {view === 'control' && !activeCapture && (
        <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50 overflow-hidden">
          <div className="flex items-center gap-2 px-6 py-4 border-b border-gray-100 dark:border-slate-700">
            <HardDrive className="w-4 h-4 text-gray-500 dark:text-slate-400" />
            <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Start Kernel Capture</h3>
          </div>
          <div className="p-6 space-y-5">
            {/* Filter */}
            <div>
              <div className="flex items-center justify-between mb-1.5">
                <label className="text-xs font-medium text-gray-700 dark:text-slate-300">
                  <Filter className="w-3 h-3 inline mr-1" />
                  Filter Expression (Fibratus QL)
                </label>
                <button onClick={() => setShowQuickFilters(!showQuickFilters)}
                  className="text-[10px] text-fibratus-500 hover:text-fibratus-400 font-medium flex items-center gap-1"
                >
                  Quick filters <ChevronDown className={`w-3 h-3 transition-transform ${showQuickFilters ? 'rotate-180' : ''}`} />
                </button>
              </div>
              <input
                type="text" value={filterInput} onChange={e => setFilterInput(e.target.value)}
                placeholder="e.g., query_dns   |   spawn_process and ps.name = 'cmd.exe'   |   kevt.name = 'Connect' and net.dip != '127.0.0.1'"
                className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-900 px-4 py-2.5 text-sm font-mono text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-600 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
              />
              <p className="mt-1 text-[10px] text-gray-400 dark:text-slate-500">
                Full Fibratus QL — same syntax as local <code className="text-[10px]">fibratus run</code>. Event macros (query_dns, spawn_process) and field expressions (ps.name, file.path, net.dip) both work. Empty = all events.
              </p>
            </div>

            {/* Quick filter presets */}
            {showQuickFilters && (
              <div className="rounded-lg border border-slate-700 bg-slate-900/50 overflow-hidden">
                <div className="px-3 py-2 text-[10px] uppercase tracking-wider text-slate-500 font-medium border-b border-slate-700/50">
                  Presets
                </div>
                <div className="max-h-64 overflow-y-auto divide-y divide-slate-800/50">
                  {quickFilters.map((qf, i) => (
                    <button key={i} onClick={() => { setFilterInput(qf.filter); setShowQuickFilters(false) }}
                      className="w-full text-left px-3 py-2 hover:bg-slate-800/50 transition-colors group"
                    >
                      <div className="flex items-center justify-between">
                        <span className="text-xs font-medium text-slate-200 group-hover:text-fibratus-400">{qf.label}</span>
                        <span className="text-[10px] text-slate-600">{qf.desc}</span>
                      </div>
                      {qf.filter && (
                        <div className="mt-0.5 text-[10px] font-mono text-slate-500 truncate">{qf.filter}</div>
                      )}
                    </button>
                  ))}
                </div>
              </div>
            )}

            {/* Duration */}
            <div>
              <label className="block text-xs font-medium text-gray-700 dark:text-slate-300 mb-1.5">
                <Clock className="w-3 h-3 inline mr-1" />
                Duration
              </label>
              <div className="flex items-center gap-2 flex-wrap">
                <button onClick={() => setDurationMin(0)}
                  className={'px-3 py-1.5 rounded-lg text-xs font-medium border transition-colors ' +
                    (durationMin === 0
                      ? 'border-fibratus-500 bg-fibratus-50 dark:bg-fibratus-900/30 text-fibratus-700 dark:text-fibratus-400'
                      : 'border-gray-300 dark:border-slate-600 text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700')
                  }>Manual</button>
                {[1, 5, 15, 30, 60].map(m => (
                  <button key={m} onClick={() => setDurationMin(m)}
                    className={'px-3 py-1.5 rounded-lg text-xs font-medium border transition-colors ' +
                      (durationMin === m
                        ? 'border-fibratus-500 bg-fibratus-50 dark:bg-fibratus-900/30 text-fibratus-700 dark:text-fibratus-400'
                        : 'border-gray-300 dark:border-slate-600 text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700')
                    }>{m < 60 ? `${m}min` : '1hr'}</button>
                ))}
              </div>
              <p className="mt-1 text-[10px] text-gray-400 dark:text-slate-500">
                {durationMin === 0 ? 'Runs until you stop it.' : `Auto-stops after ${durationMin} minute${durationMin > 1 ? 's' : ''}.`}
              </p>
            </div>

            <button onClick={() => startMutation.mutate()} disabled={startMutation.isPending}
              className="flex items-center gap-2 px-5 py-2.5 rounded-lg bg-red-600 hover:bg-red-700 text-white text-sm font-medium disabled:opacity-50 transition-colors"
            >
              {startMutation.isPending ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
              Start Capture
            </button>
          </div>
        </div>
      )}

      {/* ── LIVE CAPTURE TERMINAL ────────────── */}
      {(view === 'live' && activeCapture) && (
        <div className="rounded-xl border border-slate-700 bg-black shadow-lg overflow-hidden font-mono">
          {/* Header */}
          <div className="flex items-center justify-between px-4 py-2 bg-slate-900 border-b border-slate-700">
            <div className="flex items-center gap-3">
              <span className="relative flex h-2.5 w-2.5">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75" />
                <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-red-500" />
              </span>
              <span className="text-xs font-medium text-red-400">REC</span>
              <span className="text-xs text-slate-500">{formatElapsed(activeCapture.started_at)}</span>
              <span className="text-xs text-slate-500">{(captureDetail?.event_count || liveEvents.length).toLocaleString()} events</span>
              {activeCapture.filter && (
                <span className="text-[10px] text-slate-600 truncate max-w-[300px]" title={activeCapture.filter}>
                  {activeCapture.filter}
                </span>
              )}
            </div>
            <div className="flex items-center gap-2">
              {/* Search in live */}
              <div className="relative">
                <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3 h-3 text-slate-600" />
                <input type="text" value={searchTerm} onChange={e => setSearchTerm(e.target.value)}
                  placeholder="search..."
                  className="pl-7 pr-2 py-1 w-36 rounded border border-slate-700 bg-slate-900 text-[10px] text-slate-300 placeholder-slate-700 focus:border-slate-500 focus:outline-none"
                />
              </div>
              <button onClick={() => setAutoScroll(!autoScroll)}
                className={'px-1.5 py-1 rounded text-[10px] border ' +
                  (autoScroll ? 'border-green-800 text-green-500' : 'border-slate-700 text-slate-500')}
                title={autoScroll ? 'Auto-scroll on' : 'Auto-scroll off'}
              ><ArrowDown className="w-3 h-3" /></button>
              <button onClick={() => stopMutation.mutate()} disabled={stopMutation.isPending}
                className="flex items-center gap-1 px-3 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs disabled:opacity-50"
              >
                {stopMutation.isPending ? <Loader2 className="w-3 h-3 animate-spin" /> : <Square className="w-3 h-3" />}
                Stop
              </button>
            </div>
          </div>

          {/* Terminal output */}
          <div ref={terminalRef} className="overflow-y-auto overflow-x-auto p-2 text-[11px] leading-[18px] text-green-400 select-text"
            style={{ maxHeight: '600px', minHeight: '300px' }}
          >
            {filteredLiveLines.length === 0 && liveEvents.length === 0 && (
              <div className="text-slate-600 py-8 text-center text-xs">Waiting for events...</div>
            )}
            {filteredLiveLines.length === 0 && liveEvents.length > 0 && searchTerm && (
              <div className="text-slate-600 py-4 text-center text-xs">No events match "{searchTerm}"</div>
            )}
            {filteredLiveLines.map(({ id, line }) => (
              <div key={id} className="whitespace-pre hover:bg-slate-900/80">{line}</div>
            ))}
          </div>
        </div>
      )}

      {/* ── CAPTURE HISTORY ─────────────────── */}
      {view === 'history' && (
        <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50 overflow-hidden">
          <div className="flex items-center justify-between px-6 py-4 border-b border-gray-100 dark:border-slate-700">
            <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Capture History</h3>
            <button onClick={() => refetchCaptures()} className="text-slate-400 hover:text-slate-300">
              <RotateCw className="w-3.5 h-3.5" />
            </button>
          </div>
          {completedCaptures.length === 0 ? (
            <div className="px-6 py-12 text-center text-sm text-gray-400 dark:text-slate-500">
              No completed captures yet.
            </div>
          ) : (
            <div className="divide-y divide-gray-100 dark:divide-slate-700">
              {completedCaptures.map(cap => (
                <div key={cap.id} className="px-6 py-4 flex items-center justify-between hover:bg-gray-50 dark:hover:bg-slate-750 transition-colors">
                  <div className="space-y-1 min-w-0 flex-1">
                    <div className="flex items-center gap-3">
                      <HardDrive className="w-4 h-4 text-gray-400 dark:text-slate-500 shrink-0" />
                      <span className="text-sm font-medium text-gray-900 dark:text-slate-200">
                        {cap.event_count.toLocaleString()} events
                      </span>
                      <StatusBadge status={cap.status} />
                    </div>
                    <div className="flex items-center gap-4 text-[10px] text-gray-400 dark:text-slate-500 ml-7">
                      <span>{new Date(cap.started_at).toLocaleString()}</span>
                      <span>{formatDuration(cap.started_at, cap.completed_at)}</span>
                      {cap.filter && <span className="font-mono truncate max-w-[300px]" title={cap.filter}>{cap.filter}</span>}
                    </div>
                  </div>
                  <div className="flex items-center gap-2 shrink-0 ml-4">
                    <button onClick={() => handleBrowse(cap)}
                      className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium border border-fibratus-500/50 text-fibratus-500 hover:bg-fibratus-950/30 transition-colors"
                    ><Eye className="w-3.5 h-3.5" />Browse</button>
                    {deleteConfirm === cap.id ? (
                      <div className="flex items-center gap-1">
                        <button onClick={() => deleteMutation.mutate(cap.id)}
                          className="px-2 py-1 rounded text-[10px] font-medium bg-red-600 text-white hover:bg-red-700"
                        >Delete</button>
                        <button onClick={() => setDeleteConfirm(null)}
                          className="px-2 py-1 rounded text-[10px] font-medium border border-slate-600 text-slate-400 hover:bg-slate-700"
                        >Cancel</button>
                      </div>
                    ) : (
                      <button onClick={() => setDeleteConfirm(cap.id)}
                        className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs border border-slate-600 text-slate-400 hover:text-red-400 hover:border-red-700 transition-colors"
                      ><Trash2 className="w-3.5 h-3.5" /></button>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* ── BROWSE CAPTURE EVENTS ──────────── */}
      {view === 'browse' && browsingCapture && (
        <div className="rounded-xl border border-slate-700 bg-black shadow-lg overflow-hidden font-mono">
          {/* Header */}
          <div className="flex items-center justify-between px-4 py-2 bg-slate-900 border-b border-slate-700">
            <div className="flex items-center gap-3">
              <button onClick={() => { setView('history'); setBrowsingCapture(null) }} className="text-slate-400 hover:text-slate-200">
                <X className="w-4 h-4" />
              </button>
              <span className="text-xs text-slate-300">{browsingCapture.event_count.toLocaleString()} events</span>
              <span className="text-xs text-slate-500">{new Date(browsingCapture.started_at).toLocaleString()}</span>
              <span className="text-xs text-slate-500">{formatDuration(browsingCapture.started_at, browsingCapture.completed_at)}</span>
              {browsingCapture.filter && (
                <span className="text-[10px] text-slate-600 truncate max-w-[300px]" title={browsingCapture.filter}>{browsingCapture.filter}</span>
              )}
            </div>
          </div>

          {/* Search */}
          <div className="px-4 py-2 border-b border-slate-800 bg-slate-900/50">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-slate-600" />
              <input type="text" value={browseSearch}
                onChange={e => setBrowseSearch(e.target.value)}
                placeholder="Search events..."
                className="w-full pl-9 pr-3 py-1.5 rounded border border-slate-700 bg-black text-[11px] text-slate-300 placeholder-slate-700 focus:border-slate-500 focus:outline-none"
              />
              {browseSearch && (
                <span className="absolute right-3 top-1/2 -translate-y-1/2 text-[10px] text-slate-600">
                  {filteredBrowseLines.length}/{browseLines.length}
                </span>
              )}
            </div>
          </div>

          {/* Terminal output */}
          <div className="overflow-y-auto overflow-x-auto p-2 text-[11px] leading-[18px] text-green-400 select-text"
            style={{ maxHeight: '600px', minHeight: '200px' }}
          >
            {browseLoading && (
              <div className="text-slate-600 py-8 text-center text-xs"><Loader2 className="w-4 h-4 animate-spin inline mr-2" />Loading...</div>
            )}
            {!browseLoading && filteredBrowseLines.length === 0 && (
              <div className="text-slate-600 py-8 text-center text-xs">
                {browseSearch ? `No events match "${browseSearch}"` : 'No events'}
              </div>
            )}
            {filteredBrowseLines.map(({ id, line }) => (
              <div key={id} className="whitespace-pre hover:bg-slate-900/80">{line}</div>
            ))}
            {browseEvents.length >= 10000 && (
              <div className="text-center py-2">
                <button onClick={() => setBrowseAfter(browseEvents[browseEvents.length - 1].id)}
                  className="text-[10px] text-fibratus-500 hover:text-fibratus-400"
                >Load more...</button>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

// ═════════════════════════════════════════════════
// Sub-components
// ═════════════════════════════════════════════════

function TabButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button onClick={onClick}
      className={'flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-medium transition-colors ' +
        (active
          ? 'bg-slate-800 text-white border border-slate-600'
          : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/50 border border-transparent')
      }
    >{children}</button>
  )
}

function StatusBadge({ status }: { status: string }) {
  const styles: Record<string, string> = {
    completed: 'bg-green-900/30 text-green-400 border-green-800',
    failed: 'bg-red-900/30 text-red-400 border-red-800',
    cancelled: 'bg-yellow-900/30 text-yellow-400 border-yellow-800',
    active: 'bg-red-900/30 text-red-400 border-red-800',
  }
  return (
    <span className={`px-1.5 py-0.5 rounded text-[10px] font-medium border ${styles[status] || 'bg-slate-800 text-slate-400 border-slate-600'}`}>
      {status}
    </span>
  )
}
