import { useState, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import SlidePanel from '../components/SlidePanel'

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

const EVENT_COLORS: Record<string, string> = {
  Process: 'bg-blue-100 text-blue-800',
  Thread: 'bg-purple-100 text-purple-800',
  File: 'bg-amber-100 text-amber-800',
  Registry: 'bg-orange-100 text-orange-800',
  Net: 'bg-emerald-100 text-emerald-800',
  Image: 'bg-cyan-100 text-cyan-800',
  Mem: 'bg-pink-100 text-pink-800',
  Handle: 'bg-gray-100 text-gray-800',
  DNS: 'bg-teal-100 text-teal-800',
}

export default function Events() {
  const [agentFilter, setAgentFilter] = useState('')
  const [eventNameFilter, setEventNameFilter] = useState('')
  const [processFilter, setProcessFilter] = useState('')
  const [searchQuery, setSearchQuery] = useState('')
  const [liveMode, setLiveMode] = useState(true)
  const [selectedEvent, setSelectedEvent] = useState<TelemetryEvent | null>(null)
  const [page, setPage] = useState(1)

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['telemetry', agentFilter, eventNameFilter, processFilter, searchQuery, page],
    queryFn: () => {
      const params: Record<string, string> = {
        limit: '100',
        offset: String((page - 1) * 100),
      }
      if (agentFilter) params.agent_id = agentFilter
      if (eventNameFilter) params.event_name = eventNameFilter
      if (processFilter) params.process_name = processFilter
      if (searchQuery) params.search = searchQuery
      return api.getOrgTelemetry(params)
    },
    refetchInterval: liveMode ? 3000 : false,
  })

  // Auto-scroll behavior in live mode
  useEffect(() => {
    if (liveMode) setPage(1)
  }, [liveMode])

  const events = (data?.data || []) as TelemetryEvent[]
  const total = data?.meta?.total ?? 0

  return (
    <div>
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Live Events</h1>
          <p className="mt-1 text-sm text-gray-500">
            {total.toLocaleString()} events
            {liveMode && <span className="ml-2 inline-flex items-center gap-1 text-emerald-600"><span className="h-2 w-2 rounded-full bg-emerald-500 animate-pulse" />Live</span>}
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={() => setLiveMode(!liveMode)}
            className={'rounded-lg px-4 py-2 text-sm font-medium ' +
              (liveMode
                ? 'bg-emerald-600 text-white hover:bg-emerald-700'
                : 'bg-gray-200 text-gray-700 hover:bg-gray-300')}
          >
            {liveMode ? 'Pause' : 'Resume Live'}
          </button>
          <button
            onClick={() => refetch()}
            className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
          >
            Refresh
          </button>
        </div>
      </div>

      {/* Filters */}
      <div className="mt-4 flex flex-wrap gap-3">
        <input
          type="text"
          placeholder="Search processes, files, commands..."
          value={searchQuery}
          onChange={(e) => { setSearchQuery(e.target.value); setPage(1) }}
          className="min-w-[280px] rounded-lg border border-gray-300 px-3 py-2 text-sm focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
        />
        <select
          value={eventNameFilter}
          onChange={(e) => { setEventNameFilter(e.target.value); setPage(1) }}
          className="rounded-lg border border-gray-300 px-3 py-2 text-sm"
        >
          <option value="">All event types</option>
          <optgroup label="Process">
            <option value="CreateProcess">CreateProcess</option>
            <option value="TerminateProcess">TerminateProcess</option>
            <option value="OpenProcess">OpenProcess</option>
          </optgroup>
          <optgroup label="File">
            <option value="CreateFile">CreateFile</option>
            <option value="WriteFile">WriteFile</option>
            <option value="DeleteFile">DeleteFile</option>
            <option value="RenameFile">RenameFile</option>
          </optgroup>
          <optgroup label="Registry">
            <option value="RegSetValue">RegSetValue</option>
            <option value="RegCreateKey">RegCreateKey</option>
            <option value="RegDeleteKey">RegDeleteKey</option>
          </optgroup>
          <optgroup label="Network">
            <option value="Send">Send</option>
            <option value="Recv">Recv</option>
            <option value="Connect">Connect</option>
            <option value="Accept">Accept</option>
          </optgroup>
          <optgroup label="Image">
            <option value="LoadImage">LoadImage</option>
            <option value="UnloadImage">UnloadImage</option>
          </optgroup>
        </select>
        <input
          type="text"
          placeholder="Process name..."
          value={processFilter}
          onChange={(e) => { setProcessFilter(e.target.value); setPage(1) }}
          className="rounded-lg border border-gray-300 px-3 py-2 text-sm"
        />
        <input
          type="text"
          placeholder="Agent ID..."
          value={agentFilter}
          onChange={(e) => { setAgentFilter(e.target.value); setPage(1) }}
          className="rounded-lg border border-gray-300 px-3 py-2 text-sm"
        />
      </div>

      {/* Event stream */}
      <div className="mt-4 rounded-xl border border-gray-200 bg-white shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs font-mono">
            <thead className="border-b border-gray-100 bg-gray-50/50 sticky top-0">
              <tr>
                <th className="px-3 py-2 font-medium text-gray-500 w-[160px]">Timestamp</th>
                <th className="px-3 py-2 font-medium text-gray-500 w-[100px]">Type</th>
                <th className="px-3 py-2 font-medium text-gray-500 w-[80px]">Category</th>
                <th className="px-3 py-2 font-medium text-gray-500 w-[60px]">PID</th>
                <th className="px-3 py-2 font-medium text-gray-500 w-[140px]">Process</th>
                <th className="px-3 py-2 font-medium text-gray-500">Details</th>
                <th className="px-3 py-2 font-medium text-gray-500 w-[100px]">Host</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-50">
              {isLoading && (
                <tr><td colSpan={7} className="px-3 py-12 text-center text-gray-400 text-sm font-sans">Loading events...</td></tr>
              )}
              {!isLoading && events.map((evt) => (
                <tr
                  key={evt.id}
                  className="cursor-pointer hover:bg-gray-50/80 transition-colors"
                  onClick={() => setSelectedEvent(evt)}
                >
                  <td className="px-3 py-1.5 text-gray-500 whitespace-nowrap">
                    {new Date(evt.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', fractionalSecondDigits: 3 } as Intl.DateTimeFormatOptions)}
                  </td>
                  <td className="px-3 py-1.5 text-gray-900 font-medium">{evt.event_name}</td>
                  <td className="px-3 py-1.5">
                    <span className={'inline-flex rounded px-1.5 py-0.5 text-xs font-medium ' + (EVENT_COLORS[evt.event_category] || 'bg-gray-100 text-gray-600')}>
                      {evt.event_category}
                    </span>
                  </td>
                  <td className="px-3 py-1.5 text-gray-600">{evt.pid}</td>
                  <td className="px-3 py-1.5 text-gray-900 break-all" title={evt.process_exe}>
                    {evt.process_name}
                  </td>
                  <td className="px-3 py-1.5 text-gray-500 break-all whitespace-pre-wrap" title={JSON.stringify(evt.params)}>
                    {summarizeParams(evt)}
                  </td>
                  <td className="px-3 py-1.5 text-gray-500">{evt.agent_hostname}</td>
                </tr>
              ))}
              {!isLoading && events.length === 0 && (
                <tr>
                  <td colSpan={7} className="px-3 py-12 text-center text-gray-400 text-sm font-sans">
                    {searchQuery || eventNameFilter || processFilter
                      ? 'No events match your filters.'
                      : 'No telemetry data yet. Agents will start streaming events when connected.'}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        {/* Pagination */}
        {total > 100 && (
          <div className="flex items-center justify-between border-t border-gray-100 px-4 py-2">
            <span className="text-xs text-gray-500">{total.toLocaleString()} total events</span>
            <div className="flex gap-2">
              <button onClick={() => setPage(Math.max(1, page - 1))} disabled={page <= 1} className="rounded border border-gray-300 px-2 py-1 text-xs disabled:opacity-50">Prev</button>
              <span className="px-2 py-1 text-xs text-gray-500">Page {page}</span>
              <button onClick={() => setPage(page + 1)} disabled={events.length < 100} className="rounded border border-gray-300 px-2 py-1 text-xs disabled:opacity-50">Next</button>
            </div>
          </div>
        )}
      </div>

      {/* Event detail */}
      <SlidePanel open={!!selectedEvent} title="Event Detail" onClose={() => setSelectedEvent(null)}>
        {selectedEvent && (
          <div className="space-y-4">
            {/* Header */}
            <div>
              <div className="flex items-center gap-2">
                <span className={'inline-flex rounded px-2 py-0.5 text-xs font-medium ' + (EVENT_COLORS[selectedEvent.event_category] || 'bg-gray-100 text-gray-600')}>
                  {selectedEvent.event_category}
                </span>
                <span className="text-lg font-bold text-gray-900">{selectedEvent.event_name}</span>
              </div>
              <p className="mt-1 text-xs text-gray-500">
                {new Date(selectedEvent.timestamp).toLocaleString()} | Seq: {selectedEvent.seq}
              </p>
            </div>

            {/* Process info */}
            <div>
              <h4 className="text-xs font-medium text-gray-500 uppercase tracking-wider mb-2">Process</h4>
              {(() => {
                const raw = selectedEvent.raw_event as Record<string, unknown> | null
                const psRaw = (raw?.ps || {}) as Record<string, unknown>
                const parentRaw = (psRaw?.parent || {}) as Record<string, unknown>
                const ps = { sha256: String(psRaw.sha256 || ''), md5: String(psRaw.md5 || ''),
                  is_signed: psRaw.is_signed as boolean | undefined, is_trusted: psRaw.is_trusted as boolean | undefined,
                  cert_subject: String(psRaw.cert_subject || ''), cert_issuer: String(psRaw.cert_issuer || '') }
                const parent = { sha256: String(parentRaw.sha256 || ''), md5: String(parentRaw.md5 || '') }
                return (<>
                  <div className="grid grid-cols-2 gap-2">
                    {[
                      ['Name', selectedEvent.process_name],
                      ['PID', String(selectedEvent.pid)],
                      ['TID', String(selectedEvent.tid)],
                      ['Parent PID', String(selectedEvent.parent_pid)],
                      ['Parent', selectedEvent.parent_name],
                      ['Host', selectedEvent.agent_hostname],
                    ].map(([label, value]) => (
                      <div key={label} className="rounded bg-gray-50 px-2 py-1.5">
                        <span className="text-xs text-gray-500">{label}</span>
                        <p className="text-sm font-medium text-gray-900 font-mono break-all">{value || '-'}</p>
                      </div>
                    ))}
                  </div>
                  {selectedEvent.process_exe && (
                    <div className="mt-2 rounded bg-gray-50 px-2 py-1.5">
                      <span className="text-xs text-gray-500">Executable</span>
                      <p className="text-sm text-gray-900 font-mono break-all">{selectedEvent.process_exe}</p>
                    </div>
                  )}
                  {selectedEvent.process_cmdline && (
                    <div className="mt-2 rounded bg-gray-50 px-2 py-1.5">
                      <span className="text-xs text-gray-500">Command Line</span>
                      <p className="text-sm text-gray-900 font-mono break-all">{selectedEvent.process_cmdline}</p>
                    </div>
                  )}
                  {(ps.sha256 || ps.md5) && (
                    <div className="mt-2 space-y-1">
                      {ps.sha256 && (
                        <div className="rounded bg-gray-50 px-2 py-1.5">
                          <span className="text-xs text-gray-500">SHA256</span>
                          <p className="text-xs text-gray-900 font-mono break-all">{String(ps.sha256)}</p>
                        </div>
                      )}
                      {ps.md5 && (
                        <div className="rounded bg-gray-50 px-2 py-1.5">
                          <span className="text-xs text-gray-500">MD5</span>
                          <p className="text-xs text-gray-900 font-mono break-all">{String(ps.md5)}</p>
                        </div>
                      )}
                    </div>
                  )}
                  {(ps.is_signed !== undefined || ps.cert_subject) && (
                    <div className="mt-2 space-y-1">
                      <div className="flex gap-2">
                        {ps.is_signed !== undefined && (
                          <span className={'rounded px-1.5 py-0.5 text-[10px] font-medium ' +
                            (ps.is_signed ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-700')}>
                            {ps.is_signed ? 'Signed' : 'Unsigned'}
                          </span>
                        )}
                        {ps.is_trusted !== undefined && ps.is_signed && (
                          <span className={'rounded px-1.5 py-0.5 text-[10px] font-medium ' +
                            (ps.is_trusted ? 'bg-emerald-50 text-emerald-700' : 'bg-amber-50 text-amber-700')}>
                            {ps.is_trusted ? 'Trusted' : 'Untrusted'}
                          </span>
                        )}
                      </div>
                      {ps.cert_subject && (
                        <div className="rounded bg-gray-50 px-2 py-1.5">
                          <span className="text-xs text-gray-500">Certificate Subject</span>
                          <p className="text-xs text-gray-900 break-all">{String(ps.cert_subject)}</p>
                        </div>
                      )}
                      {ps.cert_issuer && (
                        <div className="rounded bg-gray-50 px-2 py-1.5">
                          <span className="text-xs text-gray-500">Certificate Issuer</span>
                          <p className="text-xs text-gray-900 break-all">{String(ps.cert_issuer)}</p>
                        </div>
                      )}
                    </div>
                  )}
                  {(parent.sha256 || parent.md5) && (
                    <div className="mt-2">
                      <span className="text-[10px] text-gray-400 uppercase">Parent Hashes</span>
                      <div className="space-y-1 mt-1">
                        {parent.sha256 && (
                          <div className="rounded bg-gray-50 px-2 py-1.5">
                            <span className="text-xs text-gray-500">SHA256</span>
                            <p className="text-xs text-gray-900 font-mono break-all">{String(parent.sha256)}</p>
                          </div>
                        )}
                        {parent.md5 && (
                          <div className="rounded bg-gray-50 px-2 py-1.5">
                            <span className="text-xs text-gray-500">MD5</span>
                            <p className="text-xs text-gray-900 font-mono break-all">{String(parent.md5)}</p>
                          </div>
                        )}
                      </div>
                    </div>
                  )}
                </>)
              })()}
            </div>

            {/* Parameters */}
            {selectedEvent.params && Object.keys(selectedEvent.params).length > 0 && (
              <div>
                <h4 className="text-xs font-medium text-gray-500 uppercase tracking-wider mb-2">Parameters</h4>
                <div className="space-y-1">
                  {Object.entries(selectedEvent.params).map(([key, value]) => (
                    <div key={key} className="flex gap-2 rounded bg-gray-50 px-2 py-1.5">
                      <span className="text-xs text-gray-500 whitespace-nowrap min-w-[120px]">{key}</span>
                      <span className="text-xs text-gray-900 font-mono break-all">{String(value)}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Raw event JSON */}
            <div>
              <h4 className="text-xs font-medium text-gray-500 uppercase tracking-wider mb-2">Raw Event</h4>
              <pre className="max-h-80 overflow-auto rounded-lg bg-gray-900 p-3 text-xs text-gray-100 font-mono">
                {JSON.stringify(selectedEvent.raw_event, null, 2)}
              </pre>
            </div>
          </div>
        )}
      </SlidePanel>
    </div>
  )
}

// Summarize event params into a one-line string for the table
function summarizeParams(evt: TelemetryEvent): string {
  const p = evt.params
  if (!p || typeof p !== 'object') return ''

  // File events
  if (p.file_name) return String(p.file_name)
  if (p.file_path) return String(p.file_path)

  // Registry events
  if (p.key_name) return String(p.key_name)
  if (p.key_handle) return String(p.key_handle)

  // Network events
  if (p.dip) return `${p.dip}:${p.dport}`
  if (p.sip) return `${p.sip}:${p.sport}`

  // Process events
  if (p.exe) return String(p.exe)
  if (p.cmdline) return String(p.cmdline)

  // Image events
  if (p.image_name) return String(p.image_name)

  // DNS
  if (p.name) return String(p.name)

  // Fallback: first string value
  const firstVal = Object.values(p).find(v => typeof v === 'string' && v.length > 0)
  return firstVal ? String(firstVal) : ''
}
