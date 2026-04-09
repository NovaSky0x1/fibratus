import { useState, useEffect, useRef, useCallback } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, Capture, CaptureEvent } from '../../lib/api'
import {
  HardDrive, Play, Square, Search, Filter, Loader2, Trash2,
  Clock, ChevronDown, ChevronRight, ArrowDown, X,
  Eye, RotateCw,
} from 'lucide-react'

// ═════════════════════════════════════════════════
// Event name → color mapping for the terminal
// ═════════════════════════════════════════════════

const eventColors: Record<string, string> = {
  CreateProcess: 'text-green-400',
  TerminateProcess: 'text-red-400',
  CreateFile: 'text-blue-400',
  WriteFile: 'text-blue-300',
  DeleteFile: 'text-red-300',
  RenameFile: 'text-yellow-300',
  RegSetValue: 'text-purple-400',
  RegCreateKey: 'text-purple-300',
  RegDeleteKey: 'text-red-300',
  RegDeleteValue: 'text-red-300',
  Connect: 'text-cyan-400',
  Accept: 'text-cyan-300',
  QueryDns: 'text-teal-400',
  ReplyDns: 'text-teal-300',
  LoadImage: 'text-amber-400',
  UnloadImage: 'text-amber-300',
  SetThreadContext: 'text-rose-400',
}

function getEventColor(name: string): string {
  return eventColors[name] || 'text-slate-400'
}

function formatTime(ts: string): string {
  const d = new Date(ts)
  return d.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' }) +
    '.' + String(d.getMilliseconds()).padStart(3, '0')
}

function formatElapsed(startedAt: string): string {
  const elapsed = Math.floor((Date.now() - new Date(startedAt).getTime()) / 1000)
  const m = Math.floor(elapsed / 60)
  const s = elapsed % 60
  return `${m}m ${s}s`
}

function formatDuration(start: string, end: string | null): string {
  if (!end) return '—'
  const sec = Math.floor((new Date(end).getTime() - new Date(start).getTime()) / 1000)
  if (sec < 60) return `${sec}s`
  return `${Math.floor(sec / 60)}m ${sec % 60}s`
}

function truncate(s: string, max: number): string {
  return s.length > max ? s.slice(0, max) + '...' : s
}

// ═════════════════════════════════════════════════
// Quick filter presets based on Fibratus QL
// ═════════════════════════════════════════════════

const quickFilters = [
  { label: 'All Events', filter: '', desc: 'Capture everything (high volume)' },
  { label: 'Process Activity', filter: 'spawn_process or terminate_process', desc: 'Process creation and termination' },
  { label: 'Suspicious Spawns', filter: "spawn_process and (ps.parent.name imatches '(?i)winword|excel|powerpnt|outlook|acrobat' or ps.name imatches '(?i)cmd|powershell|pwsh|wscript|cscript|mshta|certutil|bitsadmin|rundll32')", desc: 'Child processes from Office, script engines, LOLBins' },
  { label: 'PowerShell', filter: "spawn_process and ps.name imatches '(?i)powershell|pwsh'", desc: 'PowerShell execution' },
  { label: 'Network Connections', filter: 'connect_process or accept_process', desc: 'Outbound and inbound TCP/UDP' },
  { label: 'DNS Queries', filter: 'query_dns', desc: 'All DNS lookups' },
  { label: 'File Mutations', filter: 'create_file or delete_file or rename_file', desc: 'File creates, deletes, renames' },
  { label: 'Registry Changes', filter: 'set_reg_value or create_reg_key or delete_reg_key', desc: 'Registry writes and key operations' },
  { label: 'DLL Loads', filter: 'load_image', desc: 'Module/DLL loading events' },
  { label: 'Credential Access', filter: "spawn_process and ps.cmdline imatches '(?i)lsass|sam|ntds|credential|mimikatz|sekurlsa|logonpasswords'", desc: 'LSASS access and credential tools' },
  { label: 'Defense Evasion', filter: "spawn_process and ps.name imatches '(?i)reg|attrib|icacls|takeown|sc|bcdedit|wevtutil'", desc: 'Common defense evasion binaries' },
  { label: 'Lateral Movement', filter: "spawn_process and ps.name imatches '(?i)psexec|wmic|winrm|mstsc|net'", desc: 'Remote execution and admin tools' },
  { label: 'Persistence', filter: "set_reg_value and kevt.arg[key_name] imatches '(?i)run|runonce|startup|services|shell'", desc: 'Registry persistence mechanisms' },
]

// ═════════════════════════════════════════════════
// Main component
// ═════════════════════════════════════════════════

export default function AgentCaptures({ agentId }: { agentId: string }) {
  const queryClient = useQueryClient()
  const [view, setView] = useState<'control' | 'live' | 'history' | 'browse'>('control')
  const [filterInput, setFilterInput] = useState('')
  const [durationMin, setDurationMin] = useState(0) // 0 = unlimited
  const [showQuickFilters, setShowQuickFilters] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [autoScroll, setAutoScroll] = useState(true)
  const [expandedEvent, setExpandedEvent] = useState<number | null>(null)
  const [browsingCapture, setBrowsingCapture] = useState<Capture | null>(null)
  const [browseSearch, setBrowseSearch] = useState('')
  const [browseAfter, setBrowseAfter] = useState(0)
  const terminalRef = useRef<HTMLDivElement>(null)
  const lastEventIdRef = useRef(0)

  // ── Queries ─────────────────────────────────

  const { data: capturesRes, refetch: refetchCaptures } = useQuery({
    queryKey: ['captures', agentId],
    queryFn: () => api.listCaptures(agentId),
    refetchInterval: 5000,
  })
  const captures = (capturesRes?.data || []) as Capture[]
  const activeCapture = captures.find(c => c.status === 'active')
  const completedCaptures = captures.filter(c => c.status !== 'active')

  // Live event polling (only when capture active)
  const { data: liveEventsRes } = useQuery({
    queryKey: ['capture-events-live', activeCapture?.id, lastEventIdRef.current],
    queryFn: () => {
      if (!activeCapture) return { data: [] }
      return api.getCaptureEvents(activeCapture.id, {
        after_id: String(lastEventIdRef.current),
        limit: '200',
      })
    },
    enabled: !!activeCapture,
    refetchInterval: activeCapture ? 2000 : false,
  })

  // Accumulated live events
  const [liveEvents, setLiveEvents] = useState<CaptureEvent[]>([])

  useEffect(() => {
    const newEvents = (liveEventsRes?.data || []) as CaptureEvent[]
    if (newEvents.length > 0) {
      setLiveEvents(prev => {
        const merged = [...prev, ...newEvents]
        // Keep last 2000 events in memory
        return merged.length > 2000 ? merged.slice(-2000) : merged
      })
      lastEventIdRef.current = newEvents[newEvents.length - 1].id
    }
  }, [liveEventsRes])

  // Switch to live view when capture starts
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

  // Auto-scroll terminal
  useEffect(() => {
    if (autoScroll && terminalRef.current) {
      terminalRef.current.scrollTop = terminalRef.current.scrollHeight
    }
  }, [liveEvents, autoScroll])

  // Refresh active capture data
  const { data: activeCaptureDetail } = useQuery({
    queryKey: ['capture-detail', activeCapture?.id],
    queryFn: () => activeCapture ? api.getCapture(activeCapture.id) : null,
    enabled: !!activeCapture,
    refetchInterval: 3000,
  })
  const captureDetail = (activeCaptureDetail?.data || activeCapture) as Capture | undefined

  // Browse events for completed captures
  const { data: browseEventsRes, isLoading: browseLoading } = useQuery({
    queryKey: ['capture-events-browse', browsingCapture?.id, browseSearch, browseAfter],
    queryFn: () => {
      if (!browsingCapture) return { data: [] }
      const params: Record<string, string> = { limit: '5000' }
      if (browseSearch) params.search = browseSearch
      if (browseAfter > 0) params.after_id = String(browseAfter)
      return api.getCaptureEvents(browsingCapture.id, params)
    },
    enabled: !!browsingCapture,
  })
  const browseEvents = (browseEventsRes?.data || []) as CaptureEvent[]

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
    setExpandedEvent(null)
    setView('browse')
  }, [])

  // ── Render helpers ────────────────────────────

  const renderEventRow = (evt: CaptureEvent) => {
    const isExpanded = expandedEvent === evt.id
    return (
      <div key={evt.id}>
        <div
          onClick={() => setExpandedEvent(isExpanded ? null : evt.id)}
          className={'flex items-center gap-3 px-3 py-1 text-xs font-mono cursor-pointer transition-colors ' +
            (isExpanded
              ? 'bg-slate-700/50'
              : 'hover:bg-slate-800/50')
          }
        >
          <span className="text-slate-500 w-[70px] shrink-0 tabular-nums">{formatTime(evt.timestamp)}</span>
          <span className={`w-[130px] shrink-0 font-medium ${getEventColor(evt.event_name)}`}>{evt.event_name}</span>
          <span className="text-slate-300 w-[120px] shrink-0 truncate">{evt.process_name || '—'}</span>
          <span className="text-slate-500 w-[50px] shrink-0 tabular-nums">{evt.pid || ''}</span>
          <span className="text-slate-400 truncate flex-1 min-w-0">
            {summarizeEvent(evt)}
          </span>
          {isExpanded ? <ChevronDown className="w-3 h-3 text-slate-500 shrink-0" /> : <ChevronRight className="w-3 h-3 text-slate-500 shrink-0" />}
        </div>
        {isExpanded && (
          <div className="px-3 py-3 bg-slate-900/80 border-t border-b border-slate-700/50">
            <div className="grid grid-cols-2 gap-x-6 gap-y-2 text-xs font-mono mb-3">
              <div>
                <span className="text-slate-500">Process: </span>
                <span className="text-slate-200">{evt.process_name} ({evt.pid})</span>
              </div>
              <div>
                <span className="text-slate-500">Parent: </span>
                <span className="text-slate-200">{evt.parent_name || '—'} ({evt.parent_pid || '—'})</span>
              </div>
              <div className="col-span-2">
                <span className="text-slate-500">Exe: </span>
                <span className="text-slate-200 break-all">{evt.process_exe || '—'}</span>
              </div>
              <div className="col-span-2">
                <span className="text-slate-500">Cmdline: </span>
                <span className="text-slate-200 break-all">{evt.process_cmdline || '—'}</span>
              </div>
            </div>
            {evt.params && Object.keys(evt.params).length > 0 && (
              <div className="mt-2">
                <span className="text-[10px] uppercase tracking-wider text-slate-500 block mb-1">Parameters</span>
                <pre className="text-[11px] text-slate-300 bg-slate-950/60 rounded p-2 overflow-x-auto max-h-48">
                  {JSON.stringify(evt.params, null, 2)}
                </pre>
              </div>
            )}
          </div>
        )}
      </div>
    )
  }

  // ═════════════════════════════════════════════════
  // VIEWS
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
                  Filter Expression
                </label>
                <button
                  onClick={() => setShowQuickFilters(!showQuickFilters)}
                  className="text-[10px] text-fibratus-500 hover:text-fibratus-400 font-medium"
                >
                  {showQuickFilters ? 'Hide presets' : 'Quick filters'}
                </button>
              </div>
              <input
                type="text" value={filterInput} onChange={e => setFilterInput(e.target.value)}
                placeholder="e.g., spawn_process and ps.name imatches '(?i)cmd|powershell'"
                className="w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-900 px-4 py-2.5 text-sm font-mono text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-600 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
              />
              <p className="mt-1 text-[10px] text-gray-400 dark:text-slate-500">Fibratus QL syntax. Empty = capture all events (high volume).</p>
            </div>

            {/* Quick filter presets */}
            {showQuickFilters && (
              <div className="rounded-lg border border-slate-700 bg-slate-900/50 overflow-hidden">
                <div className="px-3 py-2 text-[10px] uppercase tracking-wider text-slate-500 font-medium border-b border-slate-700/50">
                  Quick Filter Presets
                </div>
                <div className="max-h-64 overflow-y-auto divide-y divide-slate-800/50">
                  {quickFilters.map((qf, i) => (
                    <button
                      key={i}
                      onClick={() => { setFilterInput(qf.filter); setShowQuickFilters(false) }}
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
                  }
                >
                  Manual
                </button>
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
              <p className="mt-1 text-[10px] text-gray-400 dark:text-slate-500">
                {durationMin === 0 ? 'Capture runs until you manually stop it.' : `Auto-stops after ${durationMin} minute${durationMin > 1 ? 's' : ''}.`}
              </p>
            </div>

            {/* Start */}
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
        <div className="rounded-xl border border-slate-700 bg-slate-900 shadow-lg overflow-hidden">
          {/* Header bar */}
          <div className="flex items-center justify-between px-4 py-2.5 bg-slate-800 border-b border-slate-700">
            <div className="flex items-center gap-3">
              <span className="relative flex h-2.5 w-2.5">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75" />
                <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-red-500" />
              </span>
              <span className="text-xs font-medium text-red-400">RECORDING</span>
              <span className="text-xs text-slate-500">|</span>
              <span className="text-xs text-slate-400 tabular-nums">{formatElapsed(activeCapture.started_at)}</span>
              <span className="text-xs text-slate-500">|</span>
              <span className="text-xs text-slate-400 tabular-nums">{(captureDetail?.event_count || liveEvents.length).toLocaleString()} events</span>
              {activeCapture.filter && (
                <>
                  <span className="text-xs text-slate-500">|</span>
                  <span className="text-xs text-slate-500 font-mono truncate max-w-[200px]" title={activeCapture.filter}>
                    filter: {activeCapture.filter}
                  </span>
                </>
              )}
            </div>
            <div className="flex items-center gap-2">
              <button
                onClick={() => setAutoScroll(!autoScroll)}
                className={'px-2 py-1 rounded text-[10px] font-medium border transition-colors ' +
                  (autoScroll
                    ? 'border-green-700 text-green-400 bg-green-950/30'
                    : 'border-slate-600 text-slate-400 hover:bg-slate-700')
                }
                title={autoScroll ? 'Auto-scroll on' : 'Auto-scroll off'}
              >
                <ArrowDown className="w-3 h-3" />
              </button>
              <button onClick={() => stopMutation.mutate()} disabled={stopMutation.isPending}
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-slate-700 hover:bg-slate-600 text-slate-200 text-xs font-medium disabled:opacity-50 transition-colors"
              >
                {stopMutation.isPending ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Square className="w-3.5 h-3.5" />}
                Stop
              </button>
            </div>
          </div>

          {/* Column header */}
          <div className="flex items-center gap-3 px-3 py-1.5 text-[10px] uppercase tracking-wider text-slate-500 border-b border-slate-800 bg-slate-850 font-medium">
            <span className="w-[70px] shrink-0">Time</span>
            <span className="w-[130px] shrink-0">Event</span>
            <span className="w-[120px] shrink-0">Process</span>
            <span className="w-[50px] shrink-0">PID</span>
            <span className="flex-1">Details</span>
          </div>

          {/* Event stream */}
          <div ref={terminalRef} className="overflow-y-auto overflow-x-hidden" style={{ maxHeight: '500px', minHeight: '300px' }}>
            {liveEvents.length === 0 && (
              <div className="flex items-center justify-center py-16 text-sm text-slate-500">
                <Loader2 className="w-4 h-4 animate-spin mr-2" />
                Waiting for events...
              </div>
            )}
            {liveEvents.map(evt => renderEventRow(evt))}
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
              No completed captures yet. Start a capture to begin recording kernel events.
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
                      {cap.filter && <span className="font-mono truncate max-w-[250px]" title={cap.filter}>filter: {cap.filter}</span>}
                    </div>
                  </div>
                  <div className="flex items-center gap-2 shrink-0 ml-4">
                    <button
                      onClick={() => handleBrowse(cap)}
                      className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium border border-fibratus-500/50 text-fibratus-500 hover:bg-fibratus-950/30 transition-colors"
                    >
                      <Eye className="w-3.5 h-3.5" />
                      Browse
                    </button>
                    <button
                      onClick={() => { if (confirm('Delete this capture and all its events?')) deleteMutation.mutate(cap.id) }}
                      className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs font-medium border border-slate-600 text-slate-400 hover:text-red-400 hover:border-red-700 transition-colors"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* ── BROWSE CAPTURE EVENTS ──────────── */}
      {view === 'browse' && browsingCapture && (
        <div className="rounded-xl border border-slate-700 bg-slate-900 shadow-lg overflow-hidden">
          {/* Browse header */}
          <div className="flex items-center justify-between px-4 py-2.5 bg-slate-800 border-b border-slate-700">
            <div className="flex items-center gap-3">
              <button onClick={() => { setView('history'); setBrowsingCapture(null) }} className="text-slate-400 hover:text-slate-200">
                <X className="w-4 h-4" />
              </button>
              <HardDrive className="w-4 h-4 text-slate-400" />
              <span className="text-xs font-medium text-slate-200">
                Capture — {browsingCapture.event_count.toLocaleString()} events
              </span>
              <span className="text-xs text-slate-500">|</span>
              <span className="text-xs text-slate-400">{new Date(browsingCapture.started_at).toLocaleString()}</span>
              <span className="text-xs text-slate-500">|</span>
              <span className="text-xs text-slate-400">{formatDuration(browsingCapture.started_at, browsingCapture.completed_at)}</span>
              {browsingCapture.filter && (
                <>
                  <span className="text-xs text-slate-500">|</span>
                  <span className="text-xs text-slate-500 font-mono truncate max-w-[200px]" title={browsingCapture.filter}>
                    filter: {browsingCapture.filter}
                  </span>
                </>
              )}
            </div>
          </div>

          {/* Search bar */}
          <div className="px-4 py-2 border-b border-slate-800 bg-slate-850">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-slate-500" />
              <input
                type="text" value={browseSearch}
                onChange={e => { setBrowseSearch(e.target.value); setBrowseAfter(0) }}
                placeholder="Search events (process name, command line, event name...)"
                className="w-full pl-9 pr-3 py-2 rounded-lg border border-slate-700 bg-slate-900 text-xs font-mono text-slate-200 placeholder-slate-600 focus:border-fibratus-500 focus:outline-none"
              />
            </div>
          </div>

          {/* Column header */}
          <div className="flex items-center gap-3 px-3 py-1.5 text-[10px] uppercase tracking-wider text-slate-500 border-b border-slate-800 font-medium">
            <span className="w-[70px] shrink-0">Time</span>
            <span className="w-[130px] shrink-0">Event</span>
            <span className="w-[120px] shrink-0">Process</span>
            <span className="w-[50px] shrink-0">PID</span>
            <span className="flex-1">Details</span>
          </div>

          {/* Events */}
          <div className="overflow-y-auto" style={{ maxHeight: '500px', minHeight: '200px' }}>
            {browseLoading && (
              <div className="flex items-center justify-center py-12 text-sm text-slate-500">
                <Loader2 className="w-4 h-4 animate-spin mr-2" />
                Loading events...
              </div>
            )}
            {!browseLoading && browseEvents.length === 0 && (
              <div className="flex items-center justify-center py-12 text-sm text-slate-500">
                {browseSearch ? 'No events match your search' : 'No events in this capture'}
              </div>
            )}
            {browseEvents.map(evt => renderEventRow(evt))}
            {browseEvents.length >= 5000 && (
              <div className="flex justify-center py-3 border-t border-slate-800">
                <button
                  onClick={() => setBrowseAfter(browseEvents[browseEvents.length - 1].id)}
                  className="px-4 py-1.5 rounded-lg text-xs font-medium text-fibratus-400 hover:bg-slate-800 transition-colors"
                >
                  Load more events...
                </button>
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
    >
      {children}
    </button>
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

function summarizeEvent(evt: CaptureEvent): string {
  const p = evt.params || {}
  switch (evt.event_name) {
    case 'CreateProcess':
    case 'TerminateProcess':
      return evt.process_cmdline ? truncate(evt.process_cmdline, 80) : evt.process_exe || ''
    case 'CreateFile':
    case 'WriteFile':
    case 'DeleteFile':
    case 'RenameFile':
      return (p.file_path || p.file_name || '') as string
    case 'RegSetValue':
    case 'RegCreateKey':
    case 'RegDeleteKey':
    case 'RegDeleteValue':
      return (p.key_name || p.key_handle || '') as string
    case 'Connect':
    case 'Accept':
      return `${p.dip || ''}:${p.dport || ''} (${p.l4_proto || ''})`
    case 'QueryDns':
    case 'ReplyDns':
      return (p.dns_name || p.name || '') as string
    case 'LoadImage':
    case 'UnloadImage':
      return (p.file_name || p.image_name || '') as string
    default:
      return ''
  }
}
