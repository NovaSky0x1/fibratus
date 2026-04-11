import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { api, Command } from '../../lib/api'
import { ChevronRight, Download, Search, RefreshCw, AlertTriangle, Info, AlertCircle, XCircle } from 'lucide-react'

interface EventViewerProps {
  agentId: string
}

interface EventLogEntry {
  event_id: string
  timestamp: string
  provider: string
  level: string
  channel: string
  computer: string
  record_id: string
  user_id: string
  task: string
  opcode: string
  keywords: string
  data: Record<string, string>
}

interface ChannelInfo {
  name: string
  records: number
  enabled: boolean
  max_size: number
}

const COMMON_CHANNELS = [
  'Security',
  'System',
  'Application',
  'Microsoft-Windows-Sysmon/Operational',
  'Microsoft-Windows-PowerShell/Operational',
  'Microsoft-Windows-Windows Defender/Operational',
  'Microsoft-Windows-TaskScheduler/Operational',
  'Microsoft-Windows-TerminalServices-LocalSessionManager/Operational',
  'Microsoft-Windows-CodeIntegrity/Operational',
  'Microsoft-Windows-WMI-Activity/Operational',
]

const LEVEL_NAMES: Record<string, { label: string; color: string; icon: typeof Info }> = {
  '0': { label: 'LogAlways', color: 'text-gray-600 dark:text-slate-400', icon: Info },
  '1': { label: 'Critical', color: 'text-red-700 dark:text-red-400', icon: XCircle },
  '2': { label: 'Error', color: 'text-red-600 dark:text-red-400', icon: AlertCircle },
  '3': { label: 'Warning', color: 'text-amber-600 dark:text-amber-400', icon: AlertTriangle },
  '4': { label: 'Information', color: 'text-blue-600 dark:text-blue-400', icon: Info },
  '5': { label: 'Verbose', color: 'text-gray-500 dark:text-slate-500', icon: Info },
}

export default function AgentEventViewer({ agentId }: EventViewerProps) {
  const [selectedChannel, setSelectedChannel] = useState('Security')
  const [events, setEvents] = useState<EventLogEntry[]>([])
  const [expandedIdx, setExpandedIdx] = useState<number | null>(null)
  const [eventIdFilter, setEventIdFilter] = useState('')
  const [levelFilter, setLevelFilter] = useState(0)
  const [count, setCount] = useState(100)
  const [channels, setChannels] = useState<ChannelInfo[]>([])
  const [showAllChannels, setShowAllChannels] = useState(false)

  // Poll for a command result by ID
  const pollCommand = async (cmdId: string, maxAttempts: number, intervalMs: number): Promise<Command | null> => {
    for (let i = 0; i < maxAttempts; i++) {
      await new Promise(r => setTimeout(r, intervalMs))
      try {
        const res = await api.getCommand(cmdId)
        const cmd = res.data as Command | undefined
        if (cmd && (cmd.status === 'completed' || cmd.status === 'failed')) return cmd
      } catch { /* continue polling */ }
    }
    return null
  }

  const queryMutation = useMutation({
    mutationFn: async (params: { channel: string; count: number; event_id: number; level: number }) => {
      const res = await api.createCommand(agentId, 'query_eventlog', params)
      const cmd = (res as { data: Command }).data
      const result = await pollCommand(cmd.id, 30, 1000)
      if (result?.status === 'completed' && result.result) {
        const r = result.result as { events: EventLogEntry[] }
        setEvents(r.events || [])
      } else {
        setEvents([])
      }
    },
  })

  const channelsMutation = useMutation({
    mutationFn: async () => {
      const res = await api.createCommand(agentId, 'list_eventlog_channels', {})
      const cmd = (res as { data: Command }).data
      const result = await pollCommand(cmd.id, 15, 1000)
      if (result?.status === 'completed' && result.result) {
        const r = result.result as { channels: ChannelInfo[] }
        setChannels(r.channels || [])
        setShowAllChannels(true)
      }
    },
  })

  const exportMutation = useMutation({
    mutationFn: async (channel: string) => {
      const res = await api.createCommand(agentId, 'export_evtx', { channel })
      const cmd = (res as { data: Command }).data
      const result = await pollCommand(cmd.id, 60, 2000)
      if (result?.status === 'completed' && result.result) {
        const r = result.result as { data: string; channel: string }
        if (r.data) {
          const binary = atob(r.data)
          const bytes = new Uint8Array(binary.length)
          for (let j = 0; j < binary.length; j++) bytes[j] = binary.charCodeAt(j)
          const blob = new Blob([bytes], { type: 'application/octet-stream' })
          const url = URL.createObjectURL(blob)
          const a = document.createElement('a')
          a.href = url
          a.download = `${r.channel.replace(/\//g, '-')}.evtx`
          a.click()
          URL.revokeObjectURL(url)
        }
      }
    },
  })

  const handleQuery = () => {
    setEvents([])
    setExpandedIdx(null)
    queryMutation.mutate({
      channel: selectedChannel,
      count,
      event_id: eventIdFilter ? parseInt(eventIdFilter, 10) : 0,
      level: levelFilter,
    })
  }

  const levelInfo = (level: string) => LEVEL_NAMES[level] || LEVEL_NAMES['4']

  const displayChannels = showAllChannels && channels.length > 0
    ? channels.filter(c => c.records > 0 || COMMON_CHANNELS.includes(c.name)).slice(0, 50)
    : COMMON_CHANNELS.map(name => ({ name, records: 0, enabled: true, max_size: 0 }))

  return (
    <div className="flex gap-4 h-[calc(100vh-200px)]">
      {/* Channel Sidebar */}
      <div className="w-64 flex-shrink-0 rounded-lg border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-900/60 overflow-auto">
        <div className="p-3 border-b border-gray-200 dark:border-slate-700 flex items-center justify-between">
          <span className="text-xs font-bold uppercase tracking-wider text-gray-600 dark:text-slate-400">Channels</span>
          <button
            onClick={() => channelsMutation.mutate()}
            className="text-xs text-blue-600 dark:text-blue-400 hover:underline"
            disabled={channelsMutation.isPending}
          >
            {channelsMutation.isPending ? 'Loading...' : showAllChannels ? 'Refresh' : 'Show All'}
          </button>
        </div>
        <div className="divide-y divide-gray-100 dark:divide-slate-800">
          {displayChannels.map(ch => (
            <button
              key={ch.name}
              onClick={() => { setSelectedChannel(ch.name); setEvents([]); setExpandedIdx(null) }}
              className={`w-full text-left px-3 py-2 text-xs transition-colors ${
                selectedChannel === ch.name
                  ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400 font-medium'
                  : 'text-gray-700 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-800/50'
              }`}
            >
              <div className="truncate">{ch.name.split('/').pop() || ch.name}</div>
              {ch.name !== ch.name.split('/').pop() && (
                <div className="text-[10px] text-gray-400 dark:text-slate-600 truncate">{ch.name}</div>
              )}
              {ch.records > 0 && (
                <div className="text-[10px] text-gray-400 dark:text-slate-600">{ch.records.toLocaleString()} records</div>
              )}
            </button>
          ))}
        </div>
      </div>

      {/* Main Content */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Toolbar */}
        <div className="flex items-center gap-3 mb-3 flex-wrap">
          <div className="flex items-center gap-2 bg-white dark:bg-slate-900/60 rounded-lg border border-gray-200 dark:border-slate-700 px-3 py-1.5">
            <Search className="w-3.5 h-3.5 text-gray-400" />
            <input
              type="text"
              placeholder="Event ID"
              value={eventIdFilter}
              onChange={e => setEventIdFilter(e.target.value.replace(/\D/g, ''))}
              className="bg-transparent text-sm w-20 outline-none text-gray-900 dark:text-slate-100 placeholder:text-gray-400"
              onKeyDown={e => e.key === 'Enter' && handleQuery()}
            />
          </div>
          <select
            value={levelFilter}
            onChange={e => setLevelFilter(Number(e.target.value))}
            className="bg-white dark:bg-slate-900/60 rounded-lg border border-gray-200 dark:border-slate-700 px-3 py-1.5 text-sm text-gray-900 dark:text-slate-100"
          >
            <option value={0}>All Levels</option>
            <option value={1}>Critical</option>
            <option value={2}>Error</option>
            <option value={3}>Warning</option>
            <option value={4}>Information</option>
          </select>
          <select
            value={count}
            onChange={e => setCount(Number(e.target.value))}
            className="bg-white dark:bg-slate-900/60 rounded-lg border border-gray-200 dark:border-slate-700 px-3 py-1.5 text-sm text-gray-900 dark:text-slate-100"
          >
            <option value={50}>50 events</option>
            <option value={100}>100 events</option>
            <option value={250}>250 events</option>
            <option value={500}>500 events</option>
          </select>
          <button
            onClick={handleQuery}
            disabled={queryMutation.isPending}
            className="flex items-center gap-1.5 rounded-lg bg-blue-600 dark:bg-blue-700 px-4 py-1.5 text-sm font-medium text-white hover:bg-blue-700 dark:hover:bg-blue-600 disabled:opacity-50 transition-colors"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${queryMutation.isPending ? 'animate-spin' : ''}`} />
            {queryMutation.isPending ? 'Querying...' : 'Query'}
          </button>
          <button
            onClick={() => exportMutation.mutate(selectedChannel)}
            disabled={exportMutation.isPending}
            className="flex items-center gap-1.5 rounded-lg bg-gray-100 dark:bg-slate-800 border border-gray-200 dark:border-slate-700 px-3 py-1.5 text-sm text-gray-700 dark:text-slate-300 hover:bg-gray-200 dark:hover:bg-slate-700 disabled:opacity-50 transition-colors"
          >
            <Download className="w-3.5 h-3.5" />
            {exportMutation.isPending ? 'Exporting...' : 'Export .evtx'}
          </button>
          <span className="text-xs text-gray-500 dark:text-slate-500 ml-auto">
            {selectedChannel} {events.length > 0 && `\u2014 ${events.length} events`}
          </span>
        </div>

        {/* Events Table */}
        <div className="flex-1 overflow-auto rounded-lg border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-900/60">
          {events.length === 0 && !queryMutation.isPending ? (
            <div className="flex items-center justify-center h-full text-gray-400 dark:text-slate-600 text-sm">
              Select a channel and click Query to browse events
            </div>
          ) : queryMutation.isPending ? (
            <div className="flex items-center justify-center h-full text-gray-400 dark:text-slate-600 text-sm">
              <RefreshCw className="w-4 h-4 animate-spin mr-2" /> Querying {selectedChannel}...
            </div>
          ) : (
            <table className="w-full text-xs">
              <thead className="sticky top-0 bg-gray-50 dark:bg-slate-800/80 border-b border-gray-200 dark:border-slate-700">
                <tr>
                  <th className="w-6 px-1"></th>
                  <th className="px-3 py-2 text-left font-semibold text-gray-600 dark:text-slate-400">Time</th>
                  <th className="px-3 py-2 text-left font-semibold text-gray-600 dark:text-slate-400 w-20">Event ID</th>
                  <th className="px-3 py-2 text-left font-semibold text-gray-600 dark:text-slate-400 w-20">Level</th>
                  <th className="px-3 py-2 text-left font-semibold text-gray-600 dark:text-slate-400">Provider</th>
                  <th className="px-3 py-2 text-left font-semibold text-gray-600 dark:text-slate-400">Record</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100 dark:divide-slate-800/50">
                {events.map((evt, idx) => {
                  const lvl = levelInfo(evt.level)
                  const LvlIcon = lvl.icon
                  const isExpanded = expandedIdx === idx
                  return (
                    <>
                      <tr
                        key={idx}
                        onClick={() => setExpandedIdx(isExpanded ? null : idx)}
                        className={`cursor-pointer transition-colors ${
                          isExpanded ? 'bg-blue-50/50 dark:bg-slate-800/80' : 'hover:bg-gray-50 dark:hover:bg-slate-800/40'
                        }`}
                      >
                        <td className="px-1 text-center">
                          <ChevronRight className={`w-3 h-3 text-gray-400 transition-transform ${isExpanded ? 'rotate-90' : ''}`} />
                        </td>
                        <td className="px-3 py-1.5 text-gray-500 dark:text-slate-400 whitespace-nowrap font-mono">
                          {evt.timestamp ? new Date(evt.timestamp).toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }) : ''}
                        </td>
                        <td className="px-3 py-1.5">
                          <span className="rounded bg-sky-100 dark:bg-sky-500/20 px-1.5 py-0.5 text-[11px] font-bold text-sky-800 dark:text-sky-300">
                            {evt.event_id}
                          </span>
                        </td>
                        <td className="px-3 py-1.5">
                          <span className={`flex items-center gap-1 ${lvl.color}`}>
                            <LvlIcon className="w-3 h-3" />
                            <span className="text-[11px] font-medium">{lvl.label}</span>
                          </span>
                        </td>
                        <td className="px-3 py-1.5 text-gray-700 dark:text-slate-300 truncate max-w-[250px]" title={evt.provider}>
                          {evt.provider}
                        </td>
                        <td className="px-3 py-1.5 text-gray-400 dark:text-slate-500 font-mono">
                          {evt.record_id}
                        </td>
                      </tr>
                      {isExpanded && (
                        <tr key={`${idx}-detail`}>
                          <td colSpan={6} className="p-0">
                            <div className="bg-blue-50/30 dark:bg-black/30 border-t border-blue-100 dark:border-slate-700/50 px-4 py-3 space-y-3">
                              {/* Event metadata */}
                              <div className="grid grid-cols-2 md:grid-cols-4 gap-x-4 gap-y-1 text-[11px]">
                                <div><span className="text-gray-500 dark:text-slate-500">Channel </span><span className="text-gray-800 dark:text-slate-300 font-mono">{evt.channel}</span></div>
                                <div><span className="text-gray-500 dark:text-slate-500">Event ID </span><span className="text-gray-800 dark:text-slate-300 font-bold">{evt.event_id}</span></div>
                                <div><span className="text-gray-500 dark:text-slate-500">Provider </span><span className="text-gray-800 dark:text-slate-300 font-mono">{evt.provider}</span></div>
                                <div><span className="text-gray-500 dark:text-slate-500">Computer </span><span className="text-gray-800 dark:text-slate-300 font-mono">{evt.computer}</span></div>
                                {evt.user_id && <div><span className="text-gray-500 dark:text-slate-500">User SID </span><span className="text-gray-800 dark:text-slate-300 font-mono">{evt.user_id}</span></div>}
                                {evt.keywords && <div><span className="text-gray-500 dark:text-slate-500">Keywords </span><span className="text-gray-800 dark:text-slate-300 font-mono">{evt.keywords}</span></div>}
                                {evt.task && <div><span className="text-gray-500 dark:text-slate-500">Task </span><span className="text-gray-800 dark:text-slate-300 font-mono">{evt.task}</span></div>}
                                {evt.opcode && <div><span className="text-gray-500 dark:text-slate-500">Opcode </span><span className="text-gray-800 dark:text-slate-300 font-mono">{evt.opcode}</span></div>}
                              </div>
                              {/* Event Data */}
                              {evt.data && Object.keys(evt.data).length > 0 && (
                                <div className="rounded-lg border border-sky-200 dark:border-sky-800/50 bg-white dark:bg-slate-900/60 p-3">
                                  <div className="text-[10px] font-bold uppercase tracking-wider text-sky-600 dark:text-sky-400 mb-2">Event Data</div>
                                  <div className="grid grid-cols-1 md:grid-cols-2 gap-x-4 gap-y-0.5 max-h-[300px] overflow-auto">
                                    {Object.entries(evt.data).map(([k, v]) => (
                                      <div key={k} className="flex gap-2 rounded bg-sky-50 dark:bg-black/30 px-2 py-1">
                                        <span className="text-[11px] text-sky-700 dark:text-sky-400 whitespace-nowrap min-w-[120px] font-medium">{k}</span>
                                        <span className="text-[11px] text-gray-800 dark:text-slate-300 font-mono break-all">{v}</span>
                                      </div>
                                    ))}
                                  </div>
                                </div>
                              )}
                            </div>
                          </td>
                        </tr>
                      )}
                    </>
                  )
                })}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  )
}
