import { useState, useEffect, useRef, useCallback, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import { useTableSort } from '../hooks/useTableSort'
import SortableHeader from '../components/SortableHeader'

// ═════════════════════════════════════════════════
// Types
// ═════════════════════════════════════════════════

interface TelemetryEvent {
  id: number
  org_id: string
  agent_id: string
  agent_hostname: string
  seq: number
  timestamp: string
  event_name: string
  event_category: string
  pid: number
  tid: number
  process_name: string
  process_exe: string
  process_cmdline: string
  parent_pid: number
  parent_name: string
  params: Record<string, unknown>
  metadata: Record<string, unknown>
  raw_event: unknown
}

// ═════════════════════════════════════════════════
// Constants
// ═════════════════════════════════════════════════

const EVENT_COLORS: Record<string, string> = {
  Process: 'bg-blue-500/20 text-blue-400 border border-blue-500/30',
  Thread: 'bg-purple-500/20 text-purple-400 border border-purple-500/30',
  File: 'bg-amber-500/20 text-amber-400 border border-amber-500/30',
  Registry: 'bg-orange-500/20 text-orange-400 border border-orange-500/30',
  Net: 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/30',
  Image: 'bg-cyan-500/20 text-cyan-400 border border-cyan-500/30',
  Mem: 'bg-pink-500/20 text-pink-400 border border-pink-500/30',
  Handle: 'bg-gray-500/20 text-gray-400 border border-gray-500/30',
  DNS: 'bg-teal-500/20 text-teal-400 border border-teal-500/30',
}

const FIELD_SUGGESTIONS = [
  // Event
  { field: 'kevt.name', desc: 'Event name', category: 'Event' },
  { field: 'kevt.category', desc: 'Event category', category: 'Event' },
  { field: 'kevt.seq', desc: 'Sequence number', category: 'Event' },
  // Process
  { field: 'ps.name', desc: 'Process name', category: 'Process' },
  { field: 'ps.exe', desc: 'Executable path', category: 'Process' },
  { field: 'ps.cmdline', desc: 'Command line', category: 'Process' },
  { field: 'ps.pid', desc: 'Process ID', category: 'Process' },
  { field: 'ps.ppid', desc: 'Parent PID', category: 'Process' },
  { field: 'ps.parent.name', desc: 'Parent name', category: 'Process' },
  // Network
  { field: 'net.dip', desc: 'Destination IP', category: 'Network' },
  { field: 'net.sip', desc: 'Source IP', category: 'Network' },
  { field: 'net.dport', desc: 'Destination port', category: 'Network' },
  { field: 'net.sport', desc: 'Source port', category: 'Network' },
  // File
  { field: 'file.name', desc: 'File name', category: 'File' },
  { field: 'file.path', desc: 'File path', category: 'File' },
  { field: 'file.extension', desc: 'File extension', category: 'File' },
  // Registry
  { field: 'registry.key.name', desc: 'Registry key', category: 'Registry' },
  { field: 'registry.value', desc: 'Registry value', category: 'Registry' },
  // Image/Module
  { field: 'image.name', desc: 'Module name', category: 'Image' },
  { field: 'image.path', desc: 'Module path', category: 'Image' },
  { field: 'image.is_signed', desc: 'Signature status', category: 'Image' },
  // DNS
  { field: 'dns.name', desc: 'DNS query name', category: 'DNS' },
  // PE
  { field: 'pe.is_signed', desc: 'PE signed', category: 'PE' },
  { field: 'pe.imphash', desc: 'Import hash', category: 'PE' },
  // Agent
  { field: 'agent.hostname', desc: 'Agent hostname', category: 'Agent' },
  { field: 'agent.id', desc: 'Agent ID', category: 'Agent' },
]

const TIME_PRESETS = [
  { label: '5m', ms: 5 * 60 * 1000 },
  { label: '15m', ms: 15 * 60 * 1000 },
  { label: '1h', ms: 60 * 60 * 1000 },
  { label: '6h', ms: 6 * 60 * 60 * 1000 },
  { label: '24h', ms: 24 * 60 * 60 * 1000 },
  { label: '7d', ms: 7 * 24 * 60 * 60 * 1000 },
]

const QUERY_HISTORY_KEY = 'fibratus_query_history'
const MAX_HISTORY = 10

// ═════════════════════════════════════════════════
// Helpers
// ═════════════════════════════════════════════════

function summarizeParams(evt: TelemetryEvent): string {
  const p = evt.params
  if (!p || typeof p !== 'object') return ''

  if (p.file_name) return String(p.file_name)
  if (p.file_path) return String(p.file_path)
  if (p.key_name) return String(p.key_name)
  if (p.key_handle) return String(p.key_handle)
  if (p.dip) return `${p.dip}:${p.dport}`
  if (p.sip) return `${p.sip}:${p.sport}`
  if (p.exe) return String(p.exe)
  if (p.cmdline) return String(p.cmdline)
  if (p.image_name) return String(p.image_name)
  if (p.name) return String(p.name)

  const firstVal = Object.values(p).find(v => typeof v === 'string' && v.length > 0)
  return firstVal ? String(firstVal) : ''
}

function loadQueryHistory(): string[] {
  try {
    const raw = localStorage.getItem(QUERY_HISTORY_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

function saveQueryHistory(history: string[]) {
  localStorage.setItem(QUERY_HISTORY_KEY, JSON.stringify(history.slice(0, MAX_HISTORY)))
}

function pushToHistory(query: string) {
  if (!query.trim()) return
  const history = loadQueryHistory().filter(q => q !== query)
  history.unshift(query)
  saveQueryHistory(history)
}

function getTimeRange(presetMs: number): { from: string; to: string } {
  const now = new Date()
  const from = new Date(now.getTime() - presetMs)
  return { from: from.toISOString(), to: now.toISOString() }
}

// ═════════════════════════════════════════════════
// Component
// ═════════════════════════════════════════════════

export default function Events() {
  // Query state
  const [queryInput, setQueryInput] = useState('')
  const [activeQuery, setActiveQuery] = useState('')
  const [queryError, setQueryError] = useState('')
  const [showAutocomplete, setShowAutocomplete] = useState(false)
  const [autocompleteIdx, setAutocompleteIdx] = useState(0)
  const [historyIdx, setHistoryIdx] = useState(-1)
  const queryRef = useRef<HTMLInputElement>(null)
  const autocompleteRef = useRef<HTMLDivElement>(null)

  // Time range
  const [timePreset, setTimePreset] = useState(TIME_PRESETS[2]) // default 1h
  const [liveMode, setLiveMode] = useState(true)

  // Table state
  const [expandedId, setExpandedId] = useState<number | null>(null)
  const [detailTab, setDetailTab] = useState<'ancestry' | 'callstack' | 'modules' | 'raw'>('ancestry')
  const [page, setPage] = useState(1)
  const [rawExpanded, setRawExpanded] = useState(false)

  // Compute filtered autocomplete suggestions
  const autocompleteMatches = useMemo(() => {
    if (!queryInput) return []
    // Extract the last token being typed (after space, =, !=, etc.)
    const tokens = queryInput.split(/[\s=!<>()]+/)
    const lastToken = tokens[tokens.length - 1] || ''
    if (!lastToken || lastToken.length < 1) return []
    const lower = lastToken.toLowerCase()
    return FIELD_SUGGESTIONS.filter(s => s.field.toLowerCase().startsWith(lower)).slice(0, 12)
  }, [queryInput])

  // Close autocomplete on outside click
  useEffect(() => {
    function handler(e: MouseEvent) {
      if (autocompleteRef.current && !autocompleteRef.current.contains(e.target as Node) &&
          queryRef.current && !queryRef.current.contains(e.target as Node)) {
        setShowAutocomplete(false)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [])

  // Data fetching
  const { data, isLoading, refetch } = useQuery({
    queryKey: ['telemetry', activeQuery, timePreset.label, page],
    queryFn: () => {
      const params: Record<string, string> = {
        limit: '100',
        offset: String((page - 1) * 100),
      }
      if (activeQuery) params.query = activeQuery
      const range = getTimeRange(timePreset.ms)
      params.from = range.from
      params.to = range.to
      return api.getOrgTelemetry(params)
    },
    refetchInterval: liveMode ? 3000 : false,
  })

  // Reset page on live mode enable
  useEffect(() => {
    if (liveMode) setPage(1)
  }, [liveMode])

  // Handle query errors from backend
  useEffect(() => {
    if (data?.error) {
      setQueryError(data.error.message || 'Query error')
    } else {
      setQueryError('')
    }
  }, [data])

  const events = (data?.data || []) as TelemetryEvent[]
  const total = data?.meta?.total ?? 0

  const { sorted: sortedEvents, sort, toggleSort } = useTableSort(events, 'timestamp', 'desc')

  // Submit query
  const submitQuery = useCallback((q?: string) => {
    const query = q ?? queryInput
    setActiveQuery(query.trim())
    setPage(1)
    setExpandedId(null)
    setShowAutocomplete(false)
    setHistoryIdx(-1)
    if (query.trim()) pushToHistory(query.trim())
  }, [queryInput])

  // Handle keyboard in query bar
  const handleQueryKeyDown = useCallback((e: React.KeyboardEvent<HTMLInputElement>) => {
    const history = loadQueryHistory()

    if (showAutocomplete && autocompleteMatches.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setAutocompleteIdx(i => Math.min(i + 1, autocompleteMatches.length - 1))
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        setAutocompleteIdx(i => Math.max(i - 1, 0))
        return
      }
      if (e.key === 'Tab' || e.key === 'Enter') {
        if (e.key === 'Tab' || (e.key === 'Enter' && showAutocomplete && autocompleteMatches.length > 0 && autocompleteIdx >= 0)) {
          // Only accept autocomplete on Tab, or Enter when user has actively navigated
          if (e.key === 'Tab') {
            e.preventDefault()
            const match = autocompleteMatches[autocompleteIdx]
            if (match) {
              const tokens = queryInput.split(/(\s+|[=!<>()]+)/)
              tokens[tokens.length - 1] = match.field
              setQueryInput(tokens.join(''))
              setShowAutocomplete(false)
              return
            }
          }
        }
      }
      if (e.key === 'Escape') {
        setShowAutocomplete(false)
        return
      }
    }

    if (e.key === 'Enter') {
      e.preventDefault()
      setShowAutocomplete(false)
      submitQuery()
      return
    }

    // History navigation (only when no autocomplete shown)
    if (!showAutocomplete && e.key === 'ArrowUp' && history.length > 0) {
      e.preventDefault()
      const newIdx = Math.min(historyIdx + 1, history.length - 1)
      setHistoryIdx(newIdx)
      setQueryInput(history[newIdx])
      return
    }
    if (!showAutocomplete && e.key === 'ArrowDown') {
      e.preventDefault()
      if (historyIdx <= 0) {
        setHistoryIdx(-1)
        setQueryInput('')
      } else {
        const newIdx = historyIdx - 1
        setHistoryIdx(newIdx)
        setQueryInput(history[newIdx])
      }
      return
    }
  }, [showAutocomplete, autocompleteMatches, autocompleteIdx, queryInput, historyIdx, submitQuery])

  const handleQueryChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value
    setQueryInput(val)
    setHistoryIdx(-1)
    // Show autocomplete when typing field-like tokens
    const tokens = val.split(/[\s=!<>()]+/)
    const lastToken = tokens[tokens.length - 1] || ''
    if (lastToken.length >= 1 && /^[a-z]/.test(lastToken)) {
      setShowAutocomplete(true)
      setAutocompleteIdx(0)
    } else {
      setShowAutocomplete(false)
    }
  }, [])

  const selectAutocomplete = useCallback((field: string) => {
    const tokens = queryInput.split(/(\s+|[=!<>()]+)/)
    tokens[tokens.length - 1] = field
    setQueryInput(tokens.join(''))
    setShowAutocomplete(false)
    queryRef.current?.focus()
  }, [queryInput])

  const clearQuery = useCallback(() => {
    setQueryInput('')
    setActiveQuery('')
    setQueryError('')
    setPage(1)
    setExpandedId(null)
    queryRef.current?.focus()
  }, [])

  const toggleExpand = useCallback((id: number) => {
    setExpandedId(prev => prev === id ? null : id)
    setDetailTab('ancestry')
    setRawExpanded(false)
  }, [])

  // ═════════════════════════════════════════════════
  // Render
  // ═════════════════════════════════════════════════

  return (
    <div className="space-y-0">
      {/* ── Query Bar ── */}
      <div className="rounded-xl bg-slate-950 dark:bg-black border border-slate-700 p-3">
        <div className="relative">
          <div className="flex items-center gap-2">
            <svg className="w-4 h-4 text-cyan-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
            <input
              ref={queryRef}
              type="text"
              value={queryInput}
              onChange={handleQueryChange}
              onKeyDown={handleQueryKeyDown}
              onFocus={() => {
                const tokens = queryInput.split(/[\s=!<>()]+/)
                const lastToken = tokens[tokens.length - 1] || ''
                if (lastToken.length >= 1 && /^[a-z]/.test(lastToken)) setShowAutocomplete(true)
              }}
              placeholder="Type a query... ps.name = 'cmd.exe' and kevt.name = 'CreateProcess'"
              className="flex-1 bg-transparent text-cyan-400 font-mono text-sm placeholder-slate-600 focus:outline-none caret-cyan-400"
              spellCheck={false}
              autoComplete="off"
            />
            <button
              onClick={() => submitQuery()}
              className="rounded-lg bg-cyan-600 hover:bg-cyan-500 px-4 py-1.5 text-xs font-medium text-white transition-colors"
            >
              Search
            </button>
          </div>

          {/* Autocomplete dropdown */}
          {showAutocomplete && autocompleteMatches.length > 0 && (
            <div
              ref={autocompleteRef}
              className="absolute left-6 top-full mt-1 z-50 w-[420px] rounded-lg border border-slate-700 bg-slate-900 shadow-xl overflow-hidden"
            >
              {autocompleteMatches.map((s, i) => (
                <button
                  key={s.field}
                  onMouseDown={(e) => { e.preventDefault(); selectAutocomplete(s.field) }}
                  className={`w-full flex items-center gap-3 px-3 py-2 text-left text-sm transition-colors ${
                    i === autocompleteIdx ? 'bg-cyan-900/40 text-cyan-300' : 'text-slate-300 hover:bg-slate-800'
                  }`}
                >
                  <span className="font-mono text-cyan-400 text-xs min-w-[140px]">{s.field}</span>
                  <span className="text-slate-500 text-xs">{s.desc}</span>
                  <span className="ml-auto rounded bg-slate-800 px-1.5 py-0.5 text-[10px] text-slate-500">{s.category}</span>
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Query error */}
        {queryError && (
          <div className="mt-2 flex items-center gap-2 rounded bg-red-950/50 border border-red-800/50 px-3 py-1.5 text-xs text-red-400">
            <svg className="w-3.5 h-3.5 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
              <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
            </svg>
            {queryError}
          </div>
        )}
      </div>

      {/* ── Time Range Bar ── */}
      <div className="flex items-center justify-between rounded-b-xl bg-slate-900/80 dark:bg-slate-950/80 border-x border-b border-slate-700/50 px-4 py-2 -mt-1">
        <div className="flex items-center gap-1.5">
          {TIME_PRESETS.map(p => (
            <button
              key={p.label}
              onClick={() => { setTimePreset(p); setPage(1) }}
              className={`rounded px-2.5 py-1 text-xs font-medium transition-colors ${
                timePreset.label === p.label
                  ? 'bg-cyan-600 text-white'
                  : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800'
              }`}
            >
              {p.label}
            </button>
          ))}
          <div className="mx-2 h-4 w-px bg-slate-700" />
          <button
            onClick={() => setLiveMode(!liveMode)}
            className={`flex items-center gap-1.5 rounded px-3 py-1 text-xs font-medium transition-colors ${
              liveMode
                ? 'bg-emerald-600/20 text-emerald-400 border border-emerald-500/30'
                : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800'
            }`}
          >
            {liveMode && <span className="h-2 w-2 rounded-full bg-emerald-500 animate-pulse" />}
            Live
          </button>
        </div>

        <div className="flex items-center gap-3">
          {/* Active query tag */}
          {activeQuery && (
            <div className="flex items-center gap-1.5 rounded bg-cyan-900/30 border border-cyan-700/40 px-2.5 py-1">
              <span className="text-xs font-mono text-cyan-400 max-w-[300px] truncate">{activeQuery}</span>
              <button onClick={clearQuery} className="text-cyan-600 hover:text-cyan-400">
                <svg className="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
                </svg>
              </button>
            </div>
          )}
          {/* Event count */}
          <span className="rounded bg-slate-800 px-2 py-0.5 text-xs text-slate-400 font-mono">
            {total.toLocaleString()} events
          </span>
          <button
            onClick={() => refetch()}
            className="rounded border border-slate-700 px-2.5 py-1 text-xs text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition-colors"
          >
            Refresh
          </button>
        </div>
      </div>

      {/* ── Event Stream Table ── */}
      <div className="mt-3 rounded-xl border border-slate-700 bg-slate-800/50 dark:bg-slate-900/50 shadow-sm dark:shadow-slate-900/50 overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs font-mono">
            <thead className="border-b border-slate-700 bg-slate-800/80 dark:bg-slate-900/80 sticky top-0 z-10">
              <tr>
                <th className="w-8 px-2 py-2" />
                <SortableHeader label="Timestamp" sortKey="timestamp" sort={sort} onSort={toggleSort} className="!px-3 !py-2 !text-xs w-[155px]" />
                <SortableHeader label="Type" sortKey="event_name" sort={sort} onSort={toggleSort} className="!px-3 !py-2 !text-xs w-[120px]" />
                <th className="px-3 py-2 font-medium text-slate-400 w-[85px]">Category</th>
                <SortableHeader label="PID" sortKey="pid" sort={sort} onSort={toggleSort} className="!px-3 !py-2 !text-xs w-[60px]" />
                <SortableHeader label="Process" sortKey="process_name" sort={sort} onSort={toggleSort} className="!px-3 !py-2 !text-xs w-[130px]" />
                <th className="px-3 py-2 font-medium text-slate-400">Details</th>
                <SortableHeader label="Agent" sortKey="agent_hostname" sort={sort} onSort={toggleSort} className="!px-3 !py-2 !text-xs w-[110px]" />
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-700/30">
              {isLoading && (
                <tr><td colSpan={8} className="px-3 py-16 text-center text-slate-500 text-sm font-sans">
                  <div className="flex items-center justify-center gap-2">
                    <svg className="animate-spin h-4 w-4 text-slate-500" viewBox="0 0 24 24">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                    </svg>
                    Loading events...
                  </div>
                </td></tr>
              )}
              {!isLoading && sortedEvents.map((evt) => (
                <EventRow
                  key={evt.id}
                  evt={evt}
                  isExpanded={expandedId === evt.id}
                  onToggle={() => toggleExpand(evt.id)}
                  detailTab={detailTab}
                  onTabChange={setDetailTab}
                  rawExpanded={rawExpanded}
                  onRawToggle={() => setRawExpanded(!rawExpanded)}
                />
              ))}
              {!isLoading && events.length === 0 && (
                <tr>
                  <td colSpan={8} className="px-3 py-16 text-center text-slate-500 text-sm font-sans">
                    {activeQuery
                      ? 'No events match your query.'
                      : 'No telemetry data yet. Agents will start streaming events when connected.'}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        {/* Pagination */}
        {total > 100 && (
          <div className="flex items-center justify-between border-t border-slate-700 px-4 py-2">
            <span className="text-xs text-slate-400">{total.toLocaleString()} total events</span>
            <div className="flex gap-2">
              <button
                onClick={() => setPage(Math.max(1, page - 1))}
                disabled={page <= 1}
                className="rounded border border-slate-600 px-2 py-1 text-xs text-slate-300 hover:bg-slate-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
              >
                Prev
              </button>
              <span className="px-2 py-1 text-xs text-slate-400">Page {page}</span>
              <button
                onClick={() => setPage(page + 1)}
                disabled={events.length < 100}
                className="rounded border border-slate-600 px-2 py-1 text-xs text-slate-300 hover:bg-slate-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
              >
                Next
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

// ═════════════════════════════════════════════════
// Event Row (table row + inline expansion)
// ═════════════════════════════════════════════════

interface EventRowProps {
  evt: TelemetryEvent
  isExpanded: boolean
  onToggle: () => void
  detailTab: 'ancestry' | 'callstack' | 'modules' | 'raw'
  onTabChange: (tab: 'ancestry' | 'callstack' | 'modules' | 'raw') => void
  rawExpanded: boolean
  onRawToggle: () => void
}

function EventRow({ evt, isExpanded, onToggle, detailTab, onTabChange, rawExpanded, onRawToggle }: EventRowProps) {
  return (
    <>
      <tr
        className={`cursor-pointer transition-colors ${
          isExpanded
            ? 'bg-slate-800/80 dark:bg-slate-900/80'
            : 'hover:bg-slate-700/20 dark:hover:bg-slate-800/30'
        }`}
        onClick={onToggle}
      >
        <td className="px-2 py-1.5 text-center">
          <svg
            className={`w-3.5 h-3.5 text-slate-500 transition-transform duration-150 ${isExpanded ? 'rotate-90' : ''}`}
            fill="currentColor"
            viewBox="0 0 20 20"
          >
            <path fillRule="evenodd" d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z" clipRule="evenodd" />
          </svg>
        </td>
        <td className="px-3 py-1.5 text-slate-400 whitespace-nowrap">
          {new Date(evt.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', fractionalSecondDigits: 3 } as Intl.DateTimeFormatOptions)}
        </td>
        <td className="px-3 py-1.5 text-slate-100 font-medium">{evt.event_name}</td>
        <td className="px-3 py-1.5">
          <span className={'inline-flex rounded px-1.5 py-0.5 text-[10px] font-medium ' + (EVENT_COLORS[evt.event_category] || 'bg-slate-600/30 text-slate-400')}>
            {evt.event_category}
          </span>
        </td>
        <td className="px-3 py-1.5 text-slate-400">{evt.pid}</td>
        <td className="px-3 py-1.5 text-slate-100 break-all" title={evt.process_exe}>
          {evt.process_name}
        </td>
        <td className="px-3 py-1.5 text-slate-500 break-all whitespace-pre-wrap" title={JSON.stringify(evt.params)}>
          {summarizeParams(evt)}
        </td>
        <td className="px-3 py-1.5 text-slate-400">{evt.agent_hostname}</td>
      </tr>

      {/* Inline expansion */}
      {isExpanded && <EventDetail evt={evt} tab={detailTab} onTabChange={onTabChange} rawExpanded={rawExpanded} onRawToggle={onRawToggle} />}
    </>
  )
}

// ═════════════════════════════════════════════════
// Inline Event Detail
// ═════════════════════════════════════════════════

interface EventDetailProps {
  evt: TelemetryEvent
  tab: 'ancestry' | 'callstack' | 'modules' | 'raw'
  onTabChange: (tab: 'ancestry' | 'callstack' | 'modules' | 'raw') => void
  rawExpanded: boolean
  onRawToggle: () => void
}

function EventDetail({ evt, tab, onTabChange, rawExpanded, onRawToggle }: EventDetailProps) {
  const raw = evt.raw_event as Record<string, unknown> | null
  const psRaw = (raw?.ps || {}) as Record<string, unknown>
  const parentRaw = (psRaw?.parent || {}) as Record<string, unknown>
  const toBool = (v: unknown) => v === true || v === 'true'

  const ps = {
    sha256: String(psRaw.sha256 || ''),
    md5: String(psRaw.md5 || ''),
    is_signed: psRaw.is_signed !== undefined ? toBool(psRaw.is_signed) : undefined,
    is_trusted: psRaw.is_trusted !== undefined ? toBool(psRaw.is_trusted) : undefined,
    cert_subject: String(psRaw.cert_subject || ''),
    cert_issuer: String(psRaw.cert_issuer || ''),
  }

  const parentName = String(parentRaw.name || evt.parent_name || '')
  const parentExe = String(parentRaw.exe || '')
  const parentCmdline = String(parentRaw.cmdline || '')

  const ancestors = (psRaw?.ancestors || (parentRaw?.ancestors)) as string[] | undefined
  const callstack = raw?.callstack as string[] | undefined
  const modules = (psRaw?.modules || []) as { name?: string; size?: number; sha256?: string; md5?: string }[]

  return (
    <tr>
      <td colSpan={8} className="p-0">
        <div className="bg-slate-900/50 dark:bg-black/30 border-t border-b border-slate-700/50 px-4 py-4 space-y-4" onClick={(e) => e.stopPropagation()}>
          {/* ── Top section: 3 cards ── */}
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-3">
            {/* Process Card */}
            <div className="rounded-lg border-l-2 border-blue-500 bg-slate-800/60 dark:bg-slate-900/60 border-y border-r border-slate-700/50 p-3 space-y-2">
              <div className="flex items-center gap-2 mb-1">
                <span className="rounded bg-blue-500/20 text-blue-400 px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wider">Process</span>
                <span className="font-medium text-sm text-slate-100">{evt.process_name}</span>
              </div>
              <div className="flex flex-wrap gap-x-4 gap-y-1 text-[11px]">
                <span className="text-slate-500">PID <span className="text-slate-300 font-mono">{evt.pid}</span></span>
                <span className="text-slate-500">TID <span className="text-slate-300 font-mono">{evt.tid}</span></span>
              </div>
              {evt.process_exe && (
                <div>
                  <span className="text-[10px] text-slate-500 uppercase tracking-wider">Executable</span>
                  <p className="text-xs text-slate-200 font-mono break-all mt-0.5">{evt.process_exe}</p>
                </div>
              )}
              {evt.process_cmdline && (
                <div className="rounded bg-black/40 border border-slate-700/50 px-2 py-1.5 text-[11px] text-slate-200 font-mono break-all whitespace-pre-wrap">
                  {evt.process_cmdline}
                </div>
              )}
              <div className="flex items-center gap-2 flex-wrap">
                {ps.is_signed !== undefined && (
                  <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${
                    ps.is_signed ? 'bg-emerald-500/20 text-emerald-400' : 'bg-red-500/20 text-red-400'
                  }`}>
                    {ps.is_signed ? 'Signed' : 'Unsigned'}
                  </span>
                )}
                {ps.is_trusted !== undefined && ps.is_signed && (
                  <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${
                    ps.is_trusted ? 'bg-emerald-500/20 text-emerald-400' : 'bg-amber-500/20 text-amber-400'
                  }`}>
                    {ps.is_trusted ? 'Trusted' : 'Untrusted'}
                  </span>
                )}
                {ps.cert_subject && <span className="text-[10px] text-slate-500">{ps.cert_subject}</span>}
              </div>
              {(ps.sha256 || ps.md5) && (
                <div className="space-y-1 mt-1">
                  {ps.sha256 && (
                    <div>
                      <span className="text-[10px] text-slate-500">SHA256</span>
                      <p className="text-[10px] text-slate-400 font-mono break-all">{ps.sha256}</p>
                    </div>
                  )}
                  {ps.md5 && (
                    <div>
                      <span className="text-[10px] text-slate-500">MD5</span>
                      <p className="text-[10px] text-slate-400 font-mono break-all">{ps.md5}</p>
                    </div>
                  )}
                </div>
              )}
            </div>

            {/* Parent Card */}
            <div className="rounded-lg border-l-2 border-amber-500 bg-slate-800/60 dark:bg-slate-900/60 border-y border-r border-slate-700/50 p-3 space-y-2">
              <div className="flex items-center gap-2 mb-1">
                <span className="rounded bg-amber-500/20 text-amber-400 px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wider">Parent</span>
                <span className="font-medium text-sm text-slate-100">{parentName || '(unknown)'}</span>
              </div>
              <div className="flex flex-wrap gap-x-4 gap-y-1 text-[11px]">
                <span className="text-slate-500">PID <span className="text-slate-300 font-mono">{evt.parent_pid}</span></span>
              </div>
              {parentExe && (
                <div>
                  <span className="text-[10px] text-slate-500 uppercase tracking-wider">Executable</span>
                  <p className="text-xs text-slate-200 font-mono break-all mt-0.5">{parentExe}</p>
                </div>
              )}
              {parentCmdline && (
                <div className="rounded bg-black/40 border border-slate-700/50 px-2 py-1.5 text-[11px] text-slate-200 font-mono break-all whitespace-pre-wrap">
                  {parentCmdline}
                </div>
              )}
              {(parentRaw.sha256 || parentRaw.md5) && (
                <div className="space-y-1 mt-1">
                  {parentRaw.sha256 && (
                    <div>
                      <span className="text-[10px] text-slate-500">SHA256</span>
                      <p className="text-[10px] text-slate-400 font-mono break-all">{String(parentRaw.sha256)}</p>
                    </div>
                  )}
                  {parentRaw.md5 && (
                    <div>
                      <span className="text-[10px] text-slate-500">MD5</span>
                      <p className="text-[10px] text-slate-400 font-mono break-all">{String(parentRaw.md5)}</p>
                    </div>
                  )}
                </div>
              )}
            </div>

            {/* Event Params Card */}
            <div className="rounded-lg border-l-2 border-slate-500 bg-slate-800/60 dark:bg-slate-900/60 border-y border-r border-slate-700/50 p-3 space-y-2">
              <div className="flex items-center gap-2 mb-1">
                <span className="rounded bg-slate-600/30 text-slate-400 px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wider">Params</span>
                <span className="text-xs text-slate-500">{Object.keys(evt.params || {}).length} fields</span>
              </div>
              {evt.params && Object.keys(evt.params).length > 0 ? (
                <div className="space-y-0.5 max-h-[200px] overflow-auto">
                  {Object.entries(evt.params).map(([key, value]) => (
                    <div key={key} className="flex gap-2 rounded bg-black/30 px-2 py-1">
                      <span className="text-[11px] text-slate-500 whitespace-nowrap min-w-[90px]">{key}</span>
                      <span className="text-[11px] text-slate-300 font-mono break-all">{String(value)}</span>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-xs text-slate-600">No parameters</p>
              )}
            </div>
          </div>

          {/* ── Bottom section: tabs ── */}
          <div>
            <div className="flex items-center gap-0.5 border-b border-slate-700/50 mb-3">
              {(['ancestry', 'callstack', 'modules', 'raw'] as const).map(t => (
                <button
                  key={t}
                  onClick={() => onTabChange(t)}
                  className={`px-3 py-1.5 text-xs font-medium border-b-2 transition-colors capitalize ${
                    tab === t
                      ? 'border-cyan-500 text-cyan-400'
                      : 'border-transparent text-slate-500 hover:text-slate-300'
                  }`}
                >
                  {t === 'callstack' ? 'Call Stack' : t === 'raw' ? 'Raw JSON' : t}
                </button>
              ))}
            </div>

            {/* Ancestry tab */}
            {tab === 'ancestry' && (
              <div>
                {ancestors && ancestors.length > 0 ? (
                  <div className="flex items-center gap-1.5 flex-wrap">
                    {ancestors.map((a, i) => (
                      <span key={i} className="flex items-center gap-1">
                        <span className="rounded bg-blue-500/15 border border-blue-500/25 px-2 py-0.5 text-[11px] font-mono text-blue-400">{a}</span>
                        {i < ancestors.length - 1 && <span className="text-slate-600 text-xs">&larr;</span>}
                      </span>
                    ))}
                  </div>
                ) : (
                  <p className="text-xs text-slate-600">No ancestry data available</p>
                )}
              </div>
            )}

            {/* Call Stack tab */}
            {tab === 'callstack' && (
              <div>
                {callstack && callstack.length > 0 ? (
                  <div className="rounded-lg bg-black/50 border border-slate-700/50 p-3 max-h-60 overflow-auto">
                    {callstack.map((frame, i) => (
                      <div key={i} className="text-[11px] font-mono text-slate-300 py-0.5 hover:bg-slate-800/50">
                        <span className="text-slate-600 mr-3 select-none">{String(callstack.length - i).padStart(2, ' ')}</span>
                        {frame}
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="text-xs text-slate-600">No call stack data available</p>
                )}
              </div>
            )}

            {/* Modules tab */}
            {tab === 'modules' && (
              <div>
                {modules.length > 0 ? (
                  <div className="space-y-0.5 max-h-60 overflow-auto">
                    {modules.slice(0, 50).map((m, i) => (
                      <div key={i} className="flex items-center gap-2 rounded bg-black/30 border border-slate-700/30 px-2 py-1 text-[11px]">
                        <span className="text-slate-300 font-mono break-all flex-1">{m.name}</span>
                        {m.sha256 && <span className="text-slate-600 font-mono text-[9px]">{m.sha256.slice(0, 16)}...</span>}
                      </div>
                    ))}
                    {modules.length > 50 && (
                      <div className="text-[10px] text-slate-600 px-2 py-1">+{modules.length - 50} more modules</div>
                    )}
                  </div>
                ) : (
                  <p className="text-xs text-slate-600">No module data available</p>
                )}
              </div>
            )}

            {/* Raw JSON tab */}
            {tab === 'raw' && (
              <div>
                <button
                  onClick={onRawToggle}
                  className="flex items-center gap-1.5 text-xs text-slate-500 hover:text-slate-300 mb-2 transition-colors"
                >
                  <svg
                    className={`w-3 h-3 transition-transform ${rawExpanded ? 'rotate-90' : ''}`}
                    fill="currentColor"
                    viewBox="0 0 20 20"
                  >
                    <path fillRule="evenodd" d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z" clipRule="evenodd" />
                  </svg>
                  {rawExpanded ? 'Collapse' : 'Expand'} raw event JSON
                </button>
                {rawExpanded && (
                  <pre className="max-h-80 overflow-auto rounded-lg bg-black/50 border border-slate-700/50 p-3 text-[11px] text-slate-300 font-mono">
                    {JSON.stringify(evt.raw_event, null, 2)}
                  </pre>
                )}
              </div>
            )}
          </div>
        </div>
      </td>
    </tr>
  )
}
