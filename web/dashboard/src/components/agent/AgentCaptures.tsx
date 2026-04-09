import { useState, useEffect, useRef, useCallback } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, Capture, CaptureEvent } from '../../lib/api'
import {
  HardDrive, Play, Square, Search, Filter, Loader2, Trash2,
  Clock, ArrowDown, X, Eye, RotateCw, ChevronDown, Download,
} from 'lucide-react'

// ═════════════════════════════════════════════════
// Color palette matching local `fibratus run` output
// Based on pkg/event/formatter.go ColorFormatter + types_windows.go
// ═════════════════════════════════════════════════

const typeColors: Record<string, string> = {
  // File — cyan/teal/amber/red
  CreateFile: 'text-cyan-400 font-bold', ReadFile: 'text-cyan-400 font-bold',
  CloseFile: 'text-cyan-400 font-bold', SetFileInformation: 'text-cyan-400 font-bold',
  MapViewFile: 'text-cyan-400 font-bold', UnmapViewFile: 'text-cyan-400 font-bold',
  RenameFile: 'text-amber-400 font-bold',
  WriteFile: 'text-teal-400 font-bold',
  DeleteFile: 'text-red-400 font-bold',
  // Registry — yellow/amber/red
  RegOpenKey: 'text-yellow-400 font-bold', RegCreateKey: 'text-yellow-400 font-bold',
  RegQueryValue: 'text-yellow-400 font-bold', RegQueryKey: 'text-yellow-400 font-bold',
  RegDeleteKey: 'text-red-400 font-bold', RegDeleteValue: 'text-red-400 font-bold',
  RegSetValue: 'text-amber-400 font-bold',
  // Process — green/red
  CreateProcess: 'text-green-400 font-bold', OpenProcess: 'text-green-400 font-bold',
  TerminateProcess: 'text-red-400 font-bold',
  // Thread — green/red/amber
  CreateThread: 'text-green-400 font-bold', OpenThread: 'text-green-400 font-bold',
  TerminateThread: 'text-red-400 font-bold',
  SetThreadContext: 'text-amber-400 font-bold',
  // Image — magenta
  LoadImage: 'text-fuchsia-400 font-bold', UnloadImage: 'text-fuchsia-400 font-bold',
  // Network — blue/teal
  Connect: 'text-teal-400 font-bold', Accept: 'text-teal-400 font-bold',
  Send: 'text-blue-400 font-bold', Recv: 'text-blue-400 font-bold',
  Disconnect: 'text-blue-400 font-bold',
  // DNS — indigo
  QueryDns: 'text-indigo-400 font-bold', ReplyDns: 'text-indigo-400 font-bold',
  // Handle — gray/amber
  CreateHandle: 'text-slate-400 font-bold', CloseHandle: 'text-slate-400 font-bold',
  DuplicateHandle: 'text-amber-400 font-bold',
  // Memory — magenta
  VirtualAlloc: 'text-fuchsia-400 font-bold', VirtualFree: 'text-fuchsia-400 font-bold',
  // Threadpool — lavender
  SubmitThreadpoolCallback: 'text-purple-300 font-bold',
  SubmitThreadpoolWork: 'text-purple-300 font-bold',
  SetThreadpoolTimer: 'text-purple-300 font-bold',
}

// Arrow prefix color based on severity (destructive/mutate/read/housekeeping)
function arrowColor(name: string): string {
  // Destructive — red
  if (['TerminateProcess','TerminateThread','DeleteFile','RegDeleteKey','RegDeleteValue','UnloadImage','VirtualFree','UnmapViewFile'].includes(name))
    return 'text-red-500'
  // Mutate — amber
  if (['CreateProcess','CreateFile','WriteFile','RenameFile','SetFileInformation','RegCreateKey','RegSetValue','CreateThread','SetThreadContext','VirtualAlloc','MapViewFile','DuplicateHandle','Connect','Accept'].includes(name))
    return 'text-amber-500'
  // DNS — indigo
  if (['QueryDns','ReplyDns'].includes(name)) return 'text-indigo-400'
  // Read — teal
  if (['ReadFile','EnumDirectory','LoadImage','RegOpenKey','RegQueryKey','RegQueryValue','OpenProcess','OpenThread','CreateHandle'].includes(name))
    return 'text-teal-500'
  // Housekeeping — gray
  return 'text-slate-600'
}

// ═════════════════════════════════════════════════
// Quick filter presets
// ═════════════════════════════════════════════════

const quickFilters = [
  { label: 'All Events', filter: '', desc: 'Everything (high volume)' },
  { label: 'Process Activity', filter: 'spawn_process or terminate_process', desc: 'Process creation/termination' },
  { label: 'Suspicious Spawns', filter: "spawn_process and (ps.parent.name imatches '(?i)winword|excel|powerpnt|outlook|acrobat' or ps.name imatches '(?i)cmd|powershell|pwsh|wscript|cscript|mshta|certutil|bitsadmin|rundll32')", desc: 'Office children, LOLBins' },
  { label: 'PowerShell', filter: "spawn_process and ps.name imatches '(?i)powershell|pwsh'", desc: 'PowerShell execution' },
  { label: 'Network', filter: 'connect_process or accept_process', desc: 'TCP/UDP connections' },
  { label: 'DNS', filter: 'query_dns', desc: 'DNS lookups' },
  { label: 'File Mutations', filter: 'create_file or delete_file or rename_file', desc: 'File creates/deletes/renames' },
  { label: 'Registry', filter: 'set_reg_value or create_reg_key or delete_reg_key', desc: 'Registry writes' },
  { label: 'DLL Loads', filter: 'load_image', desc: 'Module loading' },
  { label: 'Credential Access', filter: "spawn_process and ps.cmdline imatches '(?i)lsass|sam|ntds|credential|mimikatz|sekurlsa'", desc: 'LSASS/credential tools' },
  { label: 'Lateral Movement', filter: "spawn_process and ps.name imatches '(?i)psexec|wmic|winrm|mstsc|net'", desc: 'Remote execution' },
]

// ═════════════════════════════════════════════════
// Format helpers
// ═════════════════════════════════════════════════

function formatParams(params: Record<string, unknown> | null | undefined): string {
  if (!params || typeof params !== 'object') return ''
  return Object.entries(params)
    .filter(([k, v]) => !k.startsWith('_') && v !== '' && v !== 0 && v !== null && v !== undefined)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([k, v]) => {
      const val = typeof v === 'object' ? JSON.stringify(v) : String(v ?? '')
      return `${k}\u27A0 ${val}`
    }).join(', ')
}

// Extract the full params from raw_event (which has everything)
function getRawParams(evt: CaptureEvent): Record<string, unknown> {
  const raw = evt.raw_event as Record<string, unknown> | null
  if (!raw) return evt.params || {}
  return (raw.params || evt.params || {}) as Record<string, unknown>
}

// Get process cmdline from raw_event for richer display
function getRawCmdline(evt: CaptureEvent): string {
  const raw = evt.raw_event as Record<string, unknown> | null
  if (!raw) return evt.process_cmdline || ''
  const ps = raw.ps as Record<string, unknown> | null
  return (ps?.cmdline || evt.process_cmdline || '') as string
}

function getRawExe(evt: CaptureEvent): string {
  const raw = evt.raw_event as Record<string, unknown> | null
  if (!raw) return evt.process_exe || ''
  const ps = raw.ps as Record<string, unknown> | null
  return (ps?.exe || evt.process_exe || '') as string
}

function formatElapsed(startedAt: string): string {
  const s = Math.floor((Date.now() - new Date(startedAt).getTime()) / 1000)
  return `${Math.floor(s / 60)}m ${s % 60}s`
}

function formatDuration(start: string, end: string | null): string {
  if (!end) return '\u2014'
  const sec = Math.floor((new Date(end).getTime() - new Date(start).getTime()) / 1000)
  return sec < 60 ? `${sec}s` : `${Math.floor(sec / 60)}m ${sec % 60}s`
}

function formatTS(ts: string): string {
  const d = new Date(ts)
  const date = d.toISOString().slice(0, 10)
  const time = d.toISOString().slice(11, 23)
  return `${date} ${time}`
}

// ═════════════════════════════════════════════════
// Render a single event line matching local fibratus output
// ═════════════════════════════════════════════════

function EventLine({ evt, searchTerm }: { evt: CaptureEvent; searchTerm?: string }) {
  const arrow = arrowColor(evt.event_name)
  const typeClr = typeColors[evt.event_name] || 'text-white font-bold'
  const rawParams = getRawParams(evt)
  const p = formatParams(rawParams)

  // Build the plain text for search matching
  const plain = `${evt.seq} ${evt.timestamp} ${evt.process_name} ${evt.pid} ${evt.event_name} ${p} ${getRawCmdline(evt)} ${getRawExe(evt)}`
  if (searchTerm && !plain.toLowerCase().includes(searchTerm.toLowerCase())) return null

  return (
    <div className="whitespace-pre hover:bg-white/[0.03] leading-[20px]">
      <span className={arrow}>{'\u203A '}</span>
      <span className="text-slate-500/60">{evt.seq} </span>
      <span className="text-blue-400">{formatTS(evt.timestamp).slice(0, 10)}</span>
      {' '}
      <span className="text-cyan-400">{formatTS(evt.timestamp).slice(11)}</span>
      <span className="text-slate-600"> - </span>
      <span className="text-green-400 font-bold">{evt.process_name || '?'}</span>
      {' '}
      <span className="text-slate-600">(</span>
      <span className="text-green-400">{evt.pid}</span>
      <span className="text-slate-600">)</span>
      <span className="text-slate-600"> - </span>
      <span className={typeClr}>{evt.event_name}</span>
      {p && (
        <>
          <span className="text-slate-600"> (</span>
          <span className="text-slate-400">{p}</span>
          <span className="text-slate-600">)</span>
        </>
      )}
    </div>
  )
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

  const { data: liveEventsRes } = useQuery({
    queryKey: ['capture-events-live', activeCapture?.id, lastEventIdRef.current],
    queryFn: () => activeCapture ? api.getCaptureEvents(activeCapture.id, { after_id: String(lastEventIdRef.current), limit: '500' }) : { data: [] },
    enabled: !!activeCapture,
    refetchInterval: activeCapture ? 2000 : false,
  })

  const [liveEvents, setLiveEvents] = useState<CaptureEvent[]>([])

  useEffect(() => {
    const ne = (liveEventsRes?.data || []) as CaptureEvent[]
    if (ne.length > 0) {
      setLiveEvents(prev => { const m = [...prev, ...ne]; return m.length > 5000 ? m.slice(-5000) : m })
      lastEventIdRef.current = ne[ne.length - 1].id
    }
  }, [liveEventsRes])

  useEffect(() => {
    if (activeCapture && view === 'control') { setView('live'); setLiveEvents([]); lastEventIdRef.current = 0 }
    if (!activeCapture && view === 'live') setView('control')
  }, [activeCapture?.id])

  useEffect(() => {
    if (autoScroll && terminalRef.current) terminalRef.current.scrollTop = terminalRef.current.scrollHeight
  }, [liveEvents, autoScroll])

  const { data: activeCaptureDetail } = useQuery({
    queryKey: ['capture-detail', activeCapture?.id],
    queryFn: () => activeCapture ? api.getCapture(activeCapture.id) : null,
    enabled: !!activeCapture,
    refetchInterval: 3000,
  })
  const captureDetail = (activeCaptureDetail?.data || activeCapture) as Capture | undefined

  const { data: browseEventsRes, isLoading: browseLoading } = useQuery({
    queryKey: ['capture-events-browse', browsingCapture?.id, browseAfter],
    queryFn: () => browsingCapture ? api.getCaptureEvents(browsingCapture.id, { limit: '10000', ...(browseAfter > 0 ? { after_id: String(browseAfter) } : {}) }) : { data: [] },
    enabled: !!browsingCapture,
  })
  const browseEvents = (browseEventsRes?.data || []) as CaptureEvent[]

  // ── Mutations ────────────────────────────────

  const startMutation = useMutation({
    mutationFn: async () => {
      const res = await api.createCapture(agentId, { filter: filterInput || undefined, duration_sec: durationMin > 0 ? durationMin * 60 : 0 })
      if (res.error) throw new Error(res.error.message)
      return res.data
    },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['captures', agentId] }); setError(null) },
    onError: (e: Error) => setError(e.message),
  })

  const stopMutation = useMutation({
    mutationFn: async () => {
      if (!activeCapture) return
      const res = await api.stopCapture(activeCapture.id)
      if (res.error) throw new Error(res.error.message)
      return res.data
    },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['captures', agentId] }); setError(null) },
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
      if (browsingCapture) { setBrowsingCapture(null); setView('history') }
    },
  })

  const handleBrowse = useCallback((cap: Capture) => {
    setBrowsingCapture(cap); setBrowseSearch(''); setBrowseAfter(0); setView('browse')
  }, [])

  const [downloadMenu, setDownloadMenu] = useState<string | null>(null)

  const fetchEvents = useCallback(async (cap: Capture) => {
    const res = await api.getCaptureEvents(cap.id, { limit: '50000' })
    return (res?.data || []) as CaptureEvent[]
  }, [])

  const downloadFile = useCallback((name: string, content: string, type: string) => {
    const blob = new Blob([content], { type })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url; a.download = name; a.click()
    URL.revokeObjectURL(url)
  }, [])

  const handleDownloadJSON = useCallback(async (cap: Capture) => {
    setDownloadMenu(null)
    const events = await fetchEvents(cap)
    const ts = new Date(cap.started_at).toISOString().slice(0, 19).replace(/:/g, '')
    downloadFile(`capture-${cap.id.slice(0, 8)}-${ts}.json`, JSON.stringify({ capture: cap, events }, null, 2), 'application/json')
  }, [fetchEvents, downloadFile])

  const handleDownloadKcap = useCallback(async (cap: Capture) => {
    setDownloadMenu(null)
    if (!cap.kcap_path) {
      setError('No .kcap file available for this capture. The agent may not have written one.')
      return
    }
    // Send get_file command to agent to retrieve the .kcap
    await api.createCommand(cap.agent_id, 'get_file', { path: cap.kcap_path })
    setError(null)
    // The file will be delivered via command result — show notification
    alert(`Requested .kcap download from agent. Check command history for the file transfer.`)
  }, [])

  const handleDownloadCSV = useCallback(async (cap: Capture) => {
    setDownloadMenu(null)
    const events = await fetchEvents(cap)
    const headers = ['seq', 'timestamp', 'event_name', 'event_category', 'pid', 'process_name', 'process_exe', 'process_cmdline', 'parent_pid', 'parent_name', 'params']
    const rows = events.map(e =>
      [e.seq, e.timestamp, e.event_name, e.event_category || '', e.pid, e.process_name, e.process_exe, `"${(e.process_cmdline || '').replace(/"/g, '""')}"`, e.parent_pid, e.parent_name, `"${JSON.stringify(e.params || {}).replace(/"/g, '""')}"`].join(',')
    )
    const csv = [headers.join(','), ...rows].join('\n')
    const ts = new Date(cap.started_at).toISOString().slice(0, 19).replace(/:/g, '')
    downloadFile(`capture-${cap.id.slice(0, 8)}-${ts}.csv`, csv, 'text/csv')
  }, [fetchEvents, downloadFile])

  // ═════════════════════════════════════════════════
  // RENDER
  // ═════════════════════════════════════════════════

  return (
    <div className="space-y-4">
      {/* Tabs */}
      <div className="flex items-center gap-1">
        <TabBtn active={view === 'control' || view === 'live'} onClick={() => setView(activeCapture ? 'live' : 'control')}>
          <HardDrive className="w-3.5 h-3.5" />
          {activeCapture ? 'Live Capture' : 'New Capture'}
        </TabBtn>
        <TabBtn active={view === 'history' || view === 'browse'} onClick={() => { setView('history'); setBrowsingCapture(null) }}>
          <Clock className="w-3.5 h-3.5" />
          History{completedCaptures.length > 0 && <span className="ml-1 px-1.5 py-0.5 text-[10px] rounded-full bg-slate-700 text-slate-300">{completedCaptures.length}</span>}
        </TabBtn>
      </div>

      {error && (
        <div className="rounded-lg bg-red-900/20 border border-red-800 px-4 py-2 text-xs text-red-400 flex items-center justify-between">
          {error}<button onClick={() => setError(null)}><X className="w-3 h-3" /></button>
        </div>
      )}

      {/* ── NEW CAPTURE ── */}
      {view === 'control' && !activeCapture && (
        <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50 overflow-hidden">
          <div className="flex items-center gap-2 px-6 py-4 border-b border-gray-100 dark:border-slate-700">
            <HardDrive className="w-4 h-4 text-gray-500 dark:text-slate-400" />
            <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Start Kernel Capture</h3>
          </div>
          <div className="p-6 space-y-5">
            <div>
              <div className="flex items-center justify-between mb-1.5">
                <label className="text-xs font-medium text-gray-700 dark:text-slate-300">
                  <Filter className="w-3 h-3 inline mr-1" />Filter Expression (Fibratus QL)
                </label>
                <button onClick={() => setShowQuickFilters(!showQuickFilters)}
                  className="text-[10px] text-fibratus-500 hover:text-fibratus-400 font-medium flex items-center gap-1">
                  Quick filters <ChevronDown className={`w-3 h-3 transition-transform ${showQuickFilters ? 'rotate-180' : ''}`} />
                </button>
              </div>
              <input type="text" value={filterInput} onChange={e => setFilterInput(e.target.value)}
                placeholder="query_dns  |  spawn_process and ps.name = 'cmd.exe'  |  kevt.name = 'Connect' and net.dip != '127.0.0.1'"
                className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-900 px-4 py-2.5 text-sm font-mono text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-600 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
              />
              <p className="mt-1 text-[10px] text-gray-400 dark:text-slate-500">
                Same syntax as local <code className="text-[10px]">fibratus run</code> / <code className="text-[10px]">fibratus capture</code>. Macros + field expressions. Empty = all events.
              </p>
            </div>

            {showQuickFilters && (
              <div className="rounded-lg border border-slate-700 bg-slate-900/50 overflow-hidden">
                <div className="px-3 py-2 text-[10px] uppercase tracking-wider text-slate-500 font-medium border-b border-slate-700/50">Presets</div>
                <div className="max-h-64 overflow-y-auto divide-y divide-slate-800/50">
                  {quickFilters.map((qf, i) => (
                    <button key={i} onClick={() => { setFilterInput(qf.filter); setShowQuickFilters(false) }}
                      className="w-full text-left px-3 py-2 hover:bg-slate-800/50 transition-colors group">
                      <div className="flex items-center justify-between">
                        <span className="text-xs font-medium text-slate-200 group-hover:text-fibratus-400">{qf.label}</span>
                        <span className="text-[10px] text-slate-600">{qf.desc}</span>
                      </div>
                      {qf.filter && <div className="mt-0.5 text-[10px] font-mono text-slate-500 truncate">{qf.filter}</div>}
                    </button>
                  ))}
                </div>
              </div>
            )}

            <div>
              <label className="block text-xs font-medium text-gray-700 dark:text-slate-300 mb-1.5">
                <Clock className="w-3 h-3 inline mr-1" />Duration
              </label>
              <div className="flex items-center gap-2 flex-wrap">
                <DurBtn active={durationMin === 0} onClick={() => setDurationMin(0)}>Manual</DurBtn>
                {[1, 5, 15, 30, 60].map(m => (
                  <DurBtn key={m} active={durationMin === m} onClick={() => setDurationMin(m)}>{m < 60 ? `${m}min` : '1hr'}</DurBtn>
                ))}
              </div>
              <p className="mt-1 text-[10px] text-gray-400 dark:text-slate-500">
                {durationMin === 0 ? 'Runs until you stop it.' : `Auto-stops after ${durationMin}min.`}
              </p>
            </div>

            <button onClick={() => startMutation.mutate()} disabled={startMutation.isPending}
              className="flex items-center gap-2 px-5 py-2.5 rounded-lg bg-red-600 hover:bg-red-700 text-white text-sm font-medium disabled:opacity-50 transition-colors">
              {startMutation.isPending ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
              Start Capture
            </button>
          </div>
        </div>
      )}

      {/* ── LIVE TERMINAL ── */}
      {view === 'live' && activeCapture && (
        <Terminal
          ref={terminalRef}
          header={
            <div className="flex items-center justify-between px-4 py-2 bg-[#1a1a2e] border-b border-slate-700/50">
              <div className="flex items-center gap-3">
                <span className="relative flex h-2.5 w-2.5">
                  <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75" />
                  <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-red-500" />
                </span>
                <span className="text-[11px] font-medium text-red-400 font-mono">REC</span>
                <span className="text-[11px] text-slate-500 font-mono">{formatElapsed(activeCapture.started_at)}</span>
                <span className="text-[11px] text-slate-500 font-mono">{(captureDetail?.event_count || liveEvents.length).toLocaleString()} events</span>
                {activeCapture.filter && <span className="text-[10px] text-slate-600 font-mono truncate max-w-[300px]">{activeCapture.filter}</span>}
              </div>
              <div className="flex items-center gap-2">
                <div className="relative">
                  <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3 h-3 text-slate-600" />
                  <input type="text" value={searchTerm} onChange={e => setSearchTerm(e.target.value)} placeholder="search..."
                    className="pl-7 pr-2 py-1 w-36 rounded border border-slate-700/50 bg-[#0d0d1a] text-[10px] font-mono text-slate-300 placeholder-slate-700 focus:border-slate-500 focus:outline-none" />
                </div>
                <button onClick={() => setAutoScroll(!autoScroll)}
                  className={'px-1.5 py-1 rounded text-[10px] border ' + (autoScroll ? 'border-green-800 text-green-500' : 'border-slate-700 text-slate-600')}
                  title={autoScroll ? 'Auto-scroll on' : 'Auto-scroll off'}><ArrowDown className="w-3 h-3" /></button>
                <button onClick={() => stopMutation.mutate()} disabled={stopMutation.isPending}
                  className="flex items-center gap-1 px-3 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-mono disabled:opacity-50">
                  {stopMutation.isPending ? <Loader2 className="w-3 h-3 animate-spin" /> : <Square className="w-3 h-3" />} Stop
                </button>
              </div>
            </div>
          }
        >
          {liveEvents.length === 0 && <div className="text-slate-600 py-8 text-center text-xs font-mono">Waiting for events...</div>}
          {liveEvents.map(evt => <EventLine key={evt.id} evt={evt} searchTerm={searchTerm || undefined} />)}
        </Terminal>
      )}

      {/* ── HISTORY ── */}
      {view === 'history' && (
        <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
          <div className="flex items-center justify-between px-6 py-4 border-b border-gray-100 dark:border-slate-700">
            <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Capture History</h3>
            <button onClick={() => refetchCaptures()} className="text-slate-400 hover:text-slate-300"><RotateCw className="w-3.5 h-3.5" /></button>
          </div>
          {completedCaptures.length === 0 ? (
            <div className="px-6 py-12 text-center text-sm text-gray-400 dark:text-slate-500">No completed captures yet.</div>
          ) : (
            <div className="divide-y divide-gray-100 dark:divide-slate-700">
              {completedCaptures.map(cap => (
                <div key={cap.id} className="px-6 py-4 flex items-center justify-between hover:bg-gray-50 dark:hover:bg-slate-750 transition-colors">
                  <div className="space-y-1 min-w-0 flex-1">
                    <div className="flex items-center gap-3">
                      <HardDrive className="w-4 h-4 text-gray-400 dark:text-slate-500 shrink-0" />
                      <span className="text-sm font-medium text-gray-900 dark:text-slate-200">{cap.event_count.toLocaleString()} events</span>
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
                      className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium border border-fibratus-500/50 text-fibratus-500 hover:bg-fibratus-950/30 transition-colors">
                      <Eye className="w-3.5 h-3.5" />Replay
                    </button>
                    <div className="relative">
                      <button onClick={() => setDownloadMenu(downloadMenu === cap.id ? null : cap.id)}
                        className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium border border-slate-600 text-slate-400 hover:text-cyan-400 hover:border-cyan-700 transition-colors">
                        <Download className="w-3.5 h-3.5" />
                      </button>
                      {downloadMenu === cap.id && (
                        <div className="absolute right-0 bottom-full mb-1 z-50 rounded-lg border border-slate-700 bg-slate-800 shadow-xl py-1 min-w-[120px]">
                          <button onClick={() => handleDownloadJSON(cap)} className="w-full text-left px-3 py-1.5 text-xs text-slate-300 hover:bg-slate-700 font-mono">JSON</button>
                          <button onClick={() => handleDownloadCSV(cap)} className="w-full text-left px-3 py-1.5 text-xs text-slate-300 hover:bg-slate-700 font-mono">CSV</button>
                          <button onClick={() => handleDownloadKcap(cap)} className={'w-full text-left px-3 py-1.5 text-xs font-mono ' + (cap.kcap_path ? 'text-slate-300 hover:bg-slate-700' : 'text-slate-600 cursor-not-allowed')}>
                            .kcap {!cap.kcap_path && <span className="text-[9px] text-slate-600">(n/a)</span>}
                          </button>
                        </div>
                      )}
                    </div>
                    {deleteConfirm === cap.id ? (
                      <div className="flex items-center gap-1">
                        <button onClick={() => deleteMutation.mutate(cap.id)} className="px-2 py-1 rounded text-[10px] font-medium bg-red-600 text-white hover:bg-red-700">Delete</button>
                        <button onClick={() => setDeleteConfirm(null)} className="px-2 py-1 rounded text-[10px] font-medium border border-slate-600 text-slate-400 hover:bg-slate-700">Cancel</button>
                      </div>
                    ) : (
                      <button onClick={() => setDeleteConfirm(cap.id)}
                        className="flex items-center px-2.5 py-1.5 rounded-lg text-xs border border-slate-600 text-slate-400 hover:text-red-400 hover:border-red-700 transition-colors">
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* ── REPLAY / BROWSE ── */}
      {view === 'browse' && browsingCapture && (
        <Terminal
          header={
            <div className="flex items-center justify-between px-4 py-2 bg-[#1a1a2e] border-b border-slate-700/50">
              <div className="flex items-center gap-3">
                <button onClick={() => { setView('history'); setBrowsingCapture(null) }} className="text-slate-400 hover:text-slate-200"><X className="w-4 h-4" /></button>
                <span className="text-[11px] text-slate-300 font-mono">{browsingCapture.event_count.toLocaleString()} events</span>
                <span className="text-[11px] text-slate-600 font-mono">{new Date(browsingCapture.started_at).toLocaleString()}</span>
                <span className="text-[11px] text-slate-600 font-mono">{formatDuration(browsingCapture.started_at, browsingCapture.completed_at)}</span>
                {browsingCapture.filter && <span className="text-[10px] text-slate-600 font-mono truncate max-w-[200px]">{browsingCapture.filter}</span>}
              </div>
              <div className="relative">
                <button onClick={() => setDownloadMenu(downloadMenu === 'replay' ? null : 'replay')}
                  className="flex items-center gap-1 px-2 py-1 rounded text-[10px] font-mono border border-slate-700 text-slate-400 hover:text-cyan-400 hover:border-cyan-700 transition-colors">
                  <Download className="w-3 h-3" /> Export
                </button>
                {downloadMenu === 'replay' && (
                  <div className="absolute right-0 bottom-full mb-1 z-50 rounded-lg border border-slate-700 bg-slate-800 shadow-xl py-1 min-w-[120px]">
                    <button onClick={() => handleDownloadJSON(browsingCapture)} className="w-full text-left px-3 py-1.5 text-xs text-slate-300 hover:bg-slate-700 font-mono">JSON</button>
                    <button onClick={() => handleDownloadCSV(browsingCapture)} className="w-full text-left px-3 py-1.5 text-xs text-slate-300 hover:bg-slate-700 font-mono">CSV</button>
                    <button onClick={() => handleDownloadKcap(browsingCapture)} className={'w-full text-left px-3 py-1.5 text-xs font-mono ' + (browsingCapture.kcap_path ? 'text-slate-300 hover:bg-slate-700' : 'text-slate-600 cursor-not-allowed')}>
                      .kcap {!browsingCapture.kcap_path && <span className="text-[9px] text-slate-600">(n/a)</span>}
                    </button>
                  </div>
                )}
              </div>
            </div>
          }
          search={
            <div className="px-4 py-2 border-b border-slate-800/50 bg-[#12121f]">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-slate-600" />
                <input type="text" value={browseSearch} onChange={e => setBrowseSearch(e.target.value)}
                  placeholder="Search / filter replay events..."
                  className="w-full pl-9 pr-20 py-1.5 rounded border border-slate-700/50 bg-[#0d0d1a] text-[11px] font-mono text-slate-300 placeholder-slate-700 focus:border-slate-500 focus:outline-none" />
                {browseSearch && (
                  <span className="absolute right-3 top-1/2 -translate-y-1/2 text-[10px] text-slate-600 font-mono">
                    filtering...
                  </span>
                )}
              </div>
            </div>
          }
        >
          {browseLoading && <div className="text-slate-600 py-8 text-center text-xs font-mono"><Loader2 className="w-4 h-4 animate-spin inline mr-2" />Loading capture...</div>}
          {!browseLoading && browseEvents.length === 0 && <div className="text-slate-600 py-8 text-center text-xs font-mono">No events</div>}
          {browseEvents.map(evt => <EventLine key={evt.id} evt={evt} searchTerm={browseSearch || undefined} />)}
          {browseEvents.length >= 10000 && (
            <div className="text-center py-2">
              <button onClick={() => setBrowseAfter(browseEvents[browseEvents.length - 1].id)}
                className="text-[10px] text-fibratus-500 hover:text-fibratus-400 font-mono">Load more...</button>
            </div>
          )}
        </Terminal>
      )}
    </div>
  )
}

// ═════════════════════════════════════════════════
// Sub-components
// ═════════════════════════════════════════════════

import { forwardRef } from 'react'

const Terminal = forwardRef<HTMLDivElement, { header: React.ReactNode; search?: React.ReactNode; children: React.ReactNode }>(
  ({ header, search, children }, ref) => (
    <div className="rounded-xl border border-slate-700/50 bg-[#0d0d1a] shadow-lg overflow-hidden">
      {header}
      {search}
      <div ref={ref} className="overflow-y-auto overflow-x-auto px-3 py-2 font-mono text-[11px] select-text"
        style={{ maxHeight: '650px', minHeight: '250px' }}>
        {children}
      </div>
    </div>
  )
)

function TabBtn({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button onClick={onClick}
      className={'flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-medium transition-colors ' +
        (active ? 'bg-slate-800 text-white border border-slate-600' : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/50 border border-transparent')
      }>{children}</button>
  )
}

function DurBtn({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button onClick={onClick}
      className={'px-3 py-1.5 rounded-lg text-xs font-medium border transition-colors ' +
        (active ? 'border-fibratus-500 bg-fibratus-50 dark:bg-fibratus-900/30 text-fibratus-700 dark:text-fibratus-400'
          : 'border-gray-300 dark:border-slate-600 text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700')
      }>{children}</button>
  )
}

function StatusBadge({ status }: { status: string }) {
  const s: Record<string, string> = {
    completed: 'bg-green-900/30 text-green-400 border-green-800',
    failed: 'bg-red-900/30 text-red-400 border-red-800',
    cancelled: 'bg-yellow-900/30 text-yellow-400 border-yellow-800',
    active: 'bg-red-900/30 text-red-400 border-red-800',
  }
  return <span className={`px-1.5 py-0.5 rounded text-[10px] font-medium border ${s[status] || 'bg-slate-800 text-slate-400 border-slate-600'}`}>{status}</span>
}
