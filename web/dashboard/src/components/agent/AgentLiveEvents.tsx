import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../../lib/api'
import { RefreshCw, Search, ChevronDown, ChevronRight } from 'lucide-react'

interface TelemetryEvent {
  id: number
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
  params: unknown
  raw_event: unknown
}

type Category = 'All' | 'Process' | 'File' | 'Registry' | 'Network' | 'DNS' | 'Module'

const EVENT_CATEGORY_MAP: Record<string, Category> = {
  CreateProcess: 'Process',
  TerminateProcess: 'Process',
  OpenProcess: 'Process',
  CreateFile: 'File',
  WriteFile: 'File',
  DeleteFile: 'File',
  RenameFile: 'File',
  RegSetValue: 'Registry',
  RegCreateKey: 'Registry',
  RegDeleteKey: 'Registry',
  RegDeleteValue: 'Registry',
  Connect: 'Network',
  Accept: 'Network',
  QueryDns: 'DNS',
  ReplyDns: 'DNS',
  LoadImage: 'Module',
  UnloadImage: 'Module',
}

const CATEGORY_COLORS: Record<string, { badge: string; chip: string; chipActive: string }> = {
  Process: {
    badge: 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300',
    chip: 'border-blue-200 dark:border-blue-800 text-blue-600 dark:text-blue-400',
    chipActive: 'bg-blue-100 dark:bg-blue-900/40 border-blue-400 dark:border-blue-600 text-blue-700 dark:text-blue-300',
  },
  File: {
    badge: 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300',
    chip: 'border-green-200 dark:border-green-800 text-green-600 dark:text-green-400',
    chipActive: 'bg-green-100 dark:bg-green-900/40 border-green-400 dark:border-green-600 text-green-700 dark:text-green-300',
  },
  Registry: {
    badge: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/40 dark:text-yellow-300',
    chip: 'border-yellow-200 dark:border-yellow-800 text-yellow-600 dark:text-yellow-400',
    chipActive: 'bg-yellow-100 dark:bg-yellow-900/40 border-yellow-400 dark:border-yellow-600 text-yellow-700 dark:text-yellow-300',
  },
  Network: {
    badge: 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300',
    chip: 'border-purple-200 dark:border-purple-800 text-purple-600 dark:text-purple-400',
    chipActive: 'bg-purple-100 dark:bg-purple-900/40 border-purple-400 dark:border-purple-600 text-purple-700 dark:text-purple-300',
  },
  DNS: {
    badge: 'bg-cyan-100 text-cyan-700 dark:bg-cyan-900/40 dark:text-cyan-300',
    chip: 'border-cyan-200 dark:border-cyan-800 text-cyan-600 dark:text-cyan-400',
    chipActive: 'bg-cyan-100 dark:bg-cyan-900/40 border-cyan-400 dark:border-cyan-600 text-cyan-700 dark:text-cyan-300',
  },
  Module: {
    badge: 'bg-orange-100 text-orange-700 dark:bg-orange-900/40 dark:text-orange-300',
    chip: 'border-orange-200 dark:border-orange-800 text-orange-600 dark:text-orange-400',
    chipActive: 'bg-orange-100 dark:bg-orange-900/40 border-orange-400 dark:border-orange-600 text-orange-700 dark:text-orange-300',
  },
}

function resolveCategory(evt: TelemetryEvent): Category {
  if (EVENT_CATEGORY_MAP[evt.event_name]) return EVENT_CATEGORY_MAP[evt.event_name]
  // Fallback: try to match the ETW event_category field
  const cat = evt.event_category?.toLowerCase()
  if (cat === 'process') return 'Process'
  if (cat === 'file') return 'File'
  if (cat === 'registry') return 'Registry'
  if (cat === 'net' || cat === 'network') return 'Network'
  if (cat === 'dns') return 'DNS'
  if (cat === 'image' || cat === 'module') return 'Module'
  return 'Process'
}

function truncateParams(params: unknown, maxLen = 120): string {
  if (!params) return '-'
  const s = typeof params === 'string' ? params : JSON.stringify(params)
  return s.length > maxLen ? s.slice(0, maxLen) + '...' : s
}

function Skeleton({ className = '' }: { className?: string }) {
  return <div className={`animate-pulse bg-gray-200 dark:bg-slate-700 rounded ${className}`} />
}

export default function AgentLiveEvents({ agentId }: { agentId: string }) {
  const [activeCategories, setActiveCategories] = useState<Set<Category>>(new Set(['All']))
  const [search, setSearch] = useState('')
  const [expandedId, setExpandedId] = useState<number | null>(null)

  const { data: res, isLoading, refetch } = useQuery({
    queryKey: ['agent-live-events', agentId],
    queryFn: () => api.getAgentEvents(agentId, 500),
    refetchInterval: 5000,
  })

  const allEvents = (res?.data || []) as TelemetryEvent[]

  const toggleCategory = (cat: Category) => {
    setActiveCategories(prev => {
      const next = new Set(prev)
      if (cat === 'All') {
        return new Set(['All'])
      }
      next.delete('All')
      if (next.has(cat)) {
        next.delete(cat)
        if (next.size === 0) next.add('All')
      } else {
        next.add(cat)
      }
      return next
    })
  }

  const filtered = useMemo(() => {
    let events = allEvents

    // Category filter
    if (!activeCategories.has('All')) {
      events = events.filter(evt => activeCategories.has(resolveCategory(evt)))
    }

    // Search filter
    if (search.trim()) {
      const q = search.toLowerCase()
      events = events.filter(evt =>
        evt.event_name.toLowerCase().includes(q) ||
        evt.process_name.toLowerCase().includes(q) ||
        (evt.process_cmdline || '').toLowerCase().includes(q)
      )
    }

    return events
  }, [allEvents, activeCategories, search])

  const categories: Category[] = ['All', 'Process', 'File', 'Registry', 'Network', 'DNS', 'Module']

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Live Events</h3>
        <div className="flex items-center gap-3">
          <span className="text-xs text-gray-400 dark:text-slate-500 tabular-nums">{filtered.length} events</span>
          <button
            onClick={() => refetch()}
            className="p-1.5 rounded text-gray-400 dark:text-slate-500 hover:text-gray-600 dark:hover:text-slate-300 hover:bg-gray-100 dark:hover:bg-slate-700 transition-colors"
          >
            <RefreshCw className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {/* Category filter chips */}
      <div className="flex items-center gap-2 flex-wrap">
        {categories.map(cat => {
          const isActive = activeCategories.has(cat)
          const colors = cat === 'All' ? null : CATEGORY_COLORS[cat]
          return (
            <button
              key={cat}
              onClick={() => toggleCategory(cat)}
              className={`px-3 py-1 text-xs font-medium rounded-full border transition-colors ${
                cat === 'All'
                  ? isActive
                    ? 'bg-gray-900 dark:bg-slate-100 text-white dark:text-slate-900 border-gray-900 dark:border-slate-100'
                    : 'border-gray-300 dark:border-slate-600 text-gray-500 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700'
                  : isActive
                    ? colors?.chipActive || ''
                    : `${colors?.chip || ''} hover:bg-gray-50 dark:hover:bg-slate-700/50`
              }`}
            >
              {cat}
            </button>
          )
        })}
      </div>

      {/* Search */}
      <div className="relative">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400 dark:text-slate-500" />
        <input
          type="text"
          value={search}
          onChange={e => setSearch(e.target.value)}
          placeholder="Filter by event name, process name..."
          className="w-full pl-9 pr-4 py-2 text-sm rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
        />
      </div>

      {/* Events table */}
      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50 overflow-hidden">
        <div className="overflow-auto" style={{ maxHeight: 'calc(100vh - 340px)' }}>
          <table className="w-full text-left text-xs">
            <thead className="bg-gray-50 dark:bg-slate-900 sticky top-0 z-10">
              <tr>
                <th className="px-4 py-2.5 font-medium text-gray-500 dark:text-slate-400 w-5" />
                <th className="px-4 py-2.5 font-medium text-gray-500 dark:text-slate-400 w-24">Time</th>
                <th className="px-4 py-2.5 font-medium text-gray-500 dark:text-slate-400 w-32">Event</th>
                <th className="px-4 py-2.5 font-medium text-gray-500 dark:text-slate-400 w-20">Category</th>
                <th className="px-4 py-2.5 font-medium text-gray-500 dark:text-slate-400 w-14">PID</th>
                <th className="px-4 py-2.5 font-medium text-gray-500 dark:text-slate-400 w-36">Process</th>
                <th className="px-4 py-2.5 font-medium text-gray-500 dark:text-slate-400">Details</th>
              </tr>
            </thead>
            <tbody>
              {isLoading && (
                <tr><td colSpan={7} className="p-6">
                  <div className="space-y-2">
                    {[...Array(8)].map((_, i) => <Skeleton key={i} className="h-6 w-full" />)}
                  </div>
                </td></tr>
              )}
              {!isLoading && filtered.map((evt, idx) => {
                const cat = resolveCategory(evt)
                const colors = CATEGORY_COLORS[cat]
                const isExpanded = expandedId === idx

                return (
                  <tr
                    key={`${evt.id}-${idx}`}
                    className={`border-t border-gray-100 dark:border-slate-700 cursor-pointer transition-colors ${
                      isExpanded ? 'bg-gray-50 dark:bg-slate-700/50' : 'hover:bg-gray-50/50 dark:hover:bg-slate-700/30'
                    }`}
                    onClick={() => setExpandedId(isExpanded ? null : idx)}
                  >
                    <td className="px-4 py-2 align-top text-gray-400 dark:text-slate-500">
                      {isExpanded ? <ChevronDown className="w-3.5 h-3.5" /> : <ChevronRight className="w-3.5 h-3.5" />}
                    </td>
                    <td className="px-4 py-2 text-gray-500 dark:text-slate-400 tabular-nums whitespace-nowrap font-mono align-top">
                      {new Date(evt.timestamp).toLocaleTimeString()}
                    </td>
                    <td className="px-4 py-2 align-top font-mono text-gray-900 dark:text-slate-100">{evt.event_name}</td>
                    <td className="px-4 py-2 align-top">
                      <span className={`inline-block px-2 py-0.5 rounded text-xs font-medium ${colors?.badge || 'bg-gray-100 dark:bg-slate-700 text-gray-700 dark:text-slate-300'}`}>
                        {cat}
                      </span>
                    </td>
                    <td className="px-4 py-2 text-gray-500 dark:text-slate-400 tabular-nums font-mono align-top">{evt.pid}</td>
                    <td className="px-4 py-2 font-mono text-gray-700 dark:text-slate-300 align-top truncate max-w-[160px]">{evt.process_name}</td>
                    <td className="px-4 py-2 text-gray-500 dark:text-slate-400 font-mono align-top">
                      {!isExpanded ? (
                        <span className="break-all line-clamp-1">{truncateParams(evt.params)}</span>
                      ) : (
                        <div className="space-y-2 py-1" onClick={e => e.stopPropagation()}>
                          {evt.process_cmdline && (
                            <div>
                              <span className="text-gray-400 dark:text-slate-500 text-[10px] uppercase tracking-wide">Command Line</span>
                              <p className="text-gray-700 dark:text-slate-300 break-all whitespace-pre-wrap mt-0.5">{evt.process_cmdline}</p>
                            </div>
                          )}
                          {evt.parent_name && (
                            <div className="text-gray-400 dark:text-slate-500">
                              Parent: {evt.parent_name} (PID {evt.parent_pid})
                            </div>
                          )}
                          <div>
                            <span className="text-gray-400 dark:text-slate-500 text-[10px] uppercase tracking-wide">Parameters</span>
                            <pre className="mt-1 text-xs bg-gray-900 dark:bg-gray-950 text-gray-100 rounded-lg p-3 max-h-64 overflow-auto whitespace-pre-wrap break-all">
{JSON.stringify(evt.params, null, 2)}
                            </pre>
                          </div>
                        </div>
                      )}
                    </td>
                  </tr>
                )
              })}
              {!isLoading && filtered.length === 0 && (
                <tr>
                  <td colSpan={7} className="px-4 py-12 text-center text-gray-400 dark:text-slate-500">
                    No events match the current filters
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
