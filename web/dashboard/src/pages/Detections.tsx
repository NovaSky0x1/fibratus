import { useState, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { api, type Detection, type Rule } from '../lib/api'
import SeverityBadge from '../components/SeverityBadge'
import SlidePanel from '../components/SlidePanel'
// ProcessChain used only in full-screen /process-tree page
import { useTableSort } from '../hooks/useTableSort'
import SortableHeader from '../components/SortableHeader'

interface DetectionEvent {
  name?: string; category?: string; timestamp?: string
  params?: Record<string, unknown>; callstack?: string[] | string
  modules?: { name?: string; size?: number; sha256?: string; md5?: string }[]
  proc?: {
    pid?: number; tid?: number; ppid?: number; name?: string; exe?: string
    cmdline?: string; parent_name?: string; parent_cmdline?: string
    cwd?: string; sid?: string; username?: string; domain?: string
    session_id?: number; integrity_level?: string; ancestors?: string[]
    sha256?: string; md5?: string
    is_signed?: boolean; is_trusted?: boolean
    cert_subject?: string; cert_issuer?: string
    is_wow64?: boolean; is_protected?: boolean
  }
}

function parseEvents(events: unknown): DetectionEvent[] {
  if (!events) return []
  // Already an array of events
  if (Array.isArray(events)) return events as DetectionEvent[]
  // JSON string — parse it
  if (typeof events === 'string') {
    try {
      const parsed = JSON.parse(events)
      if (Array.isArray(parsed)) return parsed
      // Single alert object with nested events: {id, text, events: [...]}
      if (parsed && Array.isArray(parsed.events)) return parsed.events
      return [parsed]
    } catch { return [] }
  }
  // Single alert object with nested events
  if (typeof events === 'object' && events !== null) {
    const obj = events as Record<string, unknown>
    if (Array.isArray(obj.events)) return obj.events as DetectionEvent[]
    // Single event object — wrap in array
    return [events as DetectionEvent]
  }
  return []
}

type DetailView = 'detail' | 'tree'

export default function Detections() {
  const [page, setPage] = useState(1)
  const [severityFilter, setSeverityFilter] = useState('')
  const [search, setSearch] = useState('')
  const [selectedDet, setSelectedDet] = useState<Detection | null>(null)
  const [detailView, setDetailView] = useState<DetailView>('detail')
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()

  const { data, isLoading } = useQuery({
    queryKey: ['detections', page, severityFilter],
    queryFn: () => api.getDetections({ page: String(page), severity: severityFilter }),
    refetchInterval: 30000,
  })

  // Fetch rules to resolve rule links
  const { data: rulesData } = useQuery({
    queryKey: ['rules-for-lookup'],
    queryFn: () => api.getRules({ per_page: '1000' }),
    staleTime: 60000,
  })
  const rulesMap = new Map<string, Rule>()
  if (rulesData?.data) {
    for (const r of rulesData.data as Rule[]) { rulesMap.set(r.id, r) }
  }

  const allDetections = (data?.data || []) as Detection[]
  const detections = search
    ? allDetections.filter(d =>
        (d.title || d.rule_name || '').toLowerCase().includes(search.toLowerCase()) ||
        (d.text || '').toLowerCase().includes(search.toLowerCase()) ||
        (d.agent_hostname || '').toLowerCase().includes(search.toLowerCase()))
    : allDetections
  const { sorted: sortedDetections, sort, toggleSort } = useTableSort(detections, 'timestamp', 'desc')
  const total = data?.meta?.total ?? 0
  const perPage = data?.meta?.per_page ?? 50
  const totalPages = Math.ceil(total / perPage)

  // Sync selected detection with URL ?id= param.
  // If the detection isn't in the current page, fetch it directly by ID.
  const urlDetId = searchParams.get('id')
  useEffect(() => {
    if (!urlDetId || selectedDet) return
    // Try to find in current page first
    const found = allDetections.find(d => d.id === urlDetId)
    if (found) {
      setSelectedDet(found)
      return
    }
    // Not in current page — fetch directly
    api.getDetection(urlDetId).then(res => {
      if (res.data) setSelectedDet(res.data)
    }).catch(() => {})
  }, [urlDetId, allDetections, selectedDet])

  const selectDetection = (det: Detection | null) => {
    setSelectedDet(det)
    if (det) {
      setSearchParams({ id: det.id }, { replace: true })
    } else {
      setSearchParams({}, { replace: true })
    }
  }

  const selectedEvents = selectedDet ? parseEvents(selectedDet.events) : []
  const focusProc = selectedEvents[0]?.proc
  const matchedRule = selectedDet?.rule_id ? rulesMap.get(selectedDet.rule_id) : null

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 dark:text-slate-100">Detections</h1>
      <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">{total} detection(s)</p>

      <div className="mt-6 flex gap-4">
        <input
          type="text"
          placeholder="Search by rule name, text, or agent..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="flex-1 rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-800 px-4 py-2 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
        />
        <select value={severityFilter} onChange={e => { setSeverityFilter(e.target.value); setPage(1) }}
          className="rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-800 px-4 py-2 text-sm text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500">
          <option value="">All severities</option>
          <option value="critical">Critical</option>
          <option value="high">High</option>
          <option value="medium">Medium</option>
          <option value="low">Low</option>
        </select>
      </div>

      {/* Detection table */}
      <div className="mt-4 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 dark:border-slate-700 bg-gray-50/50 dark:bg-slate-800/50">
              <tr>
                <SortableHeader label="Rule" sortKey="rule_name" sort={sort} onSort={toggleSort} />
                <SortableHeader label="Agent" sortKey="agent_hostname" sort={sort} onSort={toggleSort} />
                <SortableHeader label="Severity" sortKey="severity" sort={sort} onSort={toggleSort} />
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">MITRE</th>
                <SortableHeader label="Time" sortKey="timestamp" sort={sort} onSort={toggleSort} />
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {isLoading && <tr><td colSpan={5} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">Loading...</td></tr>}
              {!isLoading && sortedDetections.map(det => (
                <tr key={det.id} className="cursor-pointer hover:bg-gray-50/50 dark:hover:bg-slate-700/30"
                  onClick={() => { selectDetection(det); setDetailView('detail') }}>
                  <td className="px-6 py-3">
                    <div className="font-medium text-gray-900 dark:text-slate-100">{det.title || det.rule_name}</div>
                    {det.text && <div className="mt-0.5 text-xs text-gray-500 dark:text-slate-400 max-w-lg whitespace-pre-line line-clamp-2">{det.text.trim()}</div>}
                  </td>
                  <td className="px-6 py-3 text-gray-600 dark:text-slate-400">{det.agent_hostname || det.agent_id?.slice(0, 8)}</td>
                  <td className="px-6 py-3"><SeverityBadge severity={det.severity} /></td>
                  <td className="px-6 py-3 text-gray-600 dark:text-slate-400">
                    {det.labels?.['technique.id'] && <span className="rounded bg-gray-100 dark:bg-slate-700 px-1.5 py-0.5 text-xs font-mono">{det.labels['technique.id']}</span>}
                  </td>
                  <td className="px-6 py-3 text-gray-500 dark:text-slate-400 whitespace-nowrap">{new Date(det.timestamp).toLocaleString()}</td>
                </tr>
              ))}
              {!isLoading && sortedDetections.length === 0 && (
                <tr><td colSpan={5} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">{search ? 'No detections match your search.' : 'No detections yet.'}</td></tr>
              )}
            </tbody>
          </table>
        </div>
        {total > perPage && (
          <div className="flex items-center justify-between border-t border-gray-100 dark:border-slate-700 px-6 py-3">
            <span className="text-sm text-gray-500 dark:text-slate-400">Page {page} of {totalPages}</span>
            <div className="flex gap-2">
              <button onClick={() => setPage(Math.max(1, page - 1))} disabled={page <= 1} className="rounded-lg border border-gray-300 dark:border-slate-600 px-3 py-1.5 text-sm text-gray-700 dark:text-slate-300 disabled:opacity-50">Previous</button>
              <button onClick={() => setPage(page + 1)} disabled={page >= totalPages} className="rounded-lg border border-gray-300 dark:border-slate-600 px-3 py-1.5 text-sm text-gray-700 dark:text-slate-300 disabled:opacity-50">Next</button>
            </div>
          </div>
        )}
      </div>

      {/* Detection detail panel */}
      <SlidePanel open={!!selectedDet} title="Detection" onClose={() => selectDetection(null)} wide>
        {selectedDet && (
          <div>
            {/* View tabs */}
            <div className="flex gap-1 border-b border-gray-200 dark:border-slate-700 mb-4">
              <button onClick={() => setDetailView('detail')}
                className={'px-4 py-2 text-sm font-medium border-b-2 -mb-px ' +
                  (detailView === 'detail' ? 'border-fibratus-600 text-fibratus-600' : 'border-transparent text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-300')}>
                Details
              </button>
              <button onClick={() => navigate(`/process-tree?detection=${selectedDet.id}${focusProc?.pid ? `&pid=${focusProc.pid}` : ''}`)}
                className="px-4 py-2 text-sm font-medium border-b-2 -mb-px border-transparent text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-300">
                Process Tree
              </button>
            </div>

            {detailView === 'detail' && (
              <div className="space-y-5">
                {/* Header */}
                <div>
                  <h3 className="text-lg font-bold text-gray-900 dark:text-slate-100">{selectedDet.title}</h3>
                  <div className="mt-2 flex items-center gap-3 flex-wrap">
                    <SeverityBadge severity={selectedDet.severity} />
                    <span className="text-sm text-gray-500 dark:text-slate-400">{selectedDet.agent_hostname}</span>
                    <span className="text-sm text-gray-400 dark:text-slate-500">{new Date(selectedDet.timestamp).toLocaleString()}</span>
                  </div>
                </div>

                {/* Alert text */}
                {selectedDet.text && (
                  <div>
                    <h4 className="text-xs font-medium text-gray-500 dark:text-slate-400 uppercase tracking-wider">Alert</h4>
                    <p className="mt-1 text-sm text-gray-700 dark:text-slate-300">{selectedDet.text}</p>
                  </div>
                )}

                {selectedDet.description && (
                  <div>
                    <h4 className="text-xs font-medium text-gray-500 dark:text-slate-400 uppercase tracking-wider">Description</h4>
                    <p className="mt-1 text-sm text-gray-700 dark:text-slate-300">{selectedDet.description}</p>
                  </div>
                )}

                {/* MITRE ATT&CK */}
                {selectedDet.labels && Object.keys(selectedDet.labels).length > 0 && (
                  <div>
                    <h4 className="text-xs font-medium text-gray-500 dark:text-slate-400 uppercase tracking-wider">MITRE ATT&CK</h4>
                    <div className="mt-2 flex flex-wrap gap-2">
                      {Object.entries(selectedDet.labels).map(([k, v]) => (
                        <div key={k} className="rounded bg-gray-50 dark:bg-slate-900 px-2.5 py-1">
                          <span className="text-[10px] text-gray-400 dark:text-slate-500 uppercase">{k.replace('tactic.', '').replace('technique.', '')}</span>
                          <p className="text-xs font-medium text-gray-800 dark:text-slate-200">{v}</p>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* Rule actions */}
                {matchedRule && (
                  <div>
                    <h4 className="text-xs font-medium text-gray-500 dark:text-slate-400 uppercase tracking-wider">Rule Actions</h4>
                    <div className="mt-2 space-y-2">
                      <a href={`/rules?rule=${matchedRule.id}`}
                        className="flex items-center gap-2 rounded-lg border border-gray-200 dark:border-slate-700 px-3 py-2 hover:bg-gray-50 dark:hover:bg-slate-700 text-sm">
                        <span className="text-fibratus-600 font-medium">{matchedRule.name}</span>
                        <span className="text-xs text-gray-400 dark:text-slate-500">v{matchedRule.version}</span>
                        <span className={'ml-auto rounded-full px-2 py-0.5 text-[10px] font-medium ' +
                          (matchedRule.enabled ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400' : 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400')}>
                          {matchedRule.enabled ? 'Active' : 'Disabled'}
                        </span>
                      </a>
                      {matchedRule.raw_yaml?.includes('action:') ? (
                        <div className="rounded-lg bg-amber-50 border border-amber-200 px-3 py-2">
                          <span className="text-xs font-medium text-amber-800">Response Actions Configured</span>
                          <p className="text-xs text-amber-700 mt-0.5">
                            {matchedRule.raw_yaml.includes('kill') && 'Kill process '}
                            {matchedRule.raw_yaml.includes('isolate') && 'Isolate host '}
                            — executed automatically on the endpoint when this rule triggers
                          </p>
                        </div>
                      ) : (
                        <div className="rounded-lg bg-gray-50 dark:bg-slate-900 border border-gray-200 dark:border-slate-700 px-3 py-2">
                          <span className="text-xs text-gray-500 dark:text-slate-400">No response actions — alert only</span>
                        </div>
                      )}
                    </div>
                  </div>
                )}
                {!matchedRule && selectedDet.rule_id && (
                  <div>
                    <h4 className="text-xs font-medium text-gray-500 dark:text-slate-400 uppercase tracking-wider">Rule</h4>
                    <p className="text-xs text-gray-400 dark:text-slate-500 mt-1">Rule {selectedDet.rule_id.slice(0, 12)}... (not found in current ruleset)</p>
                  </div>
                )}

                {/* Triggering events */}
                {selectedEvents.length > 0 && (
                  <div>
                    <h4 className="text-xs font-medium text-gray-500 dark:text-slate-400 uppercase tracking-wider">
                      Triggering Event{selectedEvents.length > 1 ? 's' : ''}
                    </h4>
                    <div className="mt-2 space-y-3">
                      {selectedEvents.map((evt, i) => (
                        <EventCard key={i} evt={evt} />
                      ))}
                    </div>
                  </div>
                )}

                {selectedEvents.length === 0 && selectedDet.events != null && (
                  <div>
                    <h4 className="text-xs font-medium text-gray-500 dark:text-slate-400 uppercase tracking-wider">Event Data</h4>
                    <pre className="mt-2 max-h-64 overflow-auto rounded bg-gray-900 p-3 text-xs text-gray-100 font-mono">
                      {JSON.stringify(selectedDet.events, null, 2)}
                    </pre>
                  </div>
                )}
              </div>
            )}

            {/* Process Tree tab navigates directly to full screen */}
          </div>
        )}
      </SlidePanel>
    </div>
  )
}


function EventCard({ evt }: { evt: DetectionEvent }) {
  const [showRaw, setShowRaw] = useState(false)
  const proc = evt.proc
  const callstack = evt.callstack
    ? (Array.isArray(evt.callstack) ? evt.callstack : (typeof evt.callstack === 'string' ? (evt.callstack as string).split('\n').filter(Boolean) : []))
    : []
  const params = evt.params || {}
  const paramEntries = Object.entries(params).filter(([k]) => k !== 'callstack' && k !== 'modules')

  return (
    <div className="rounded-lg border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 text-sm">
      <div className="flex items-center gap-2 border-b border-gray-100 dark:border-slate-700 px-3 py-2 bg-gray-50/50 dark:bg-slate-900/40 flex-wrap">
        <span className="rounded bg-blue-100 dark:bg-blue-900/40 text-blue-700 dark:text-blue-300 px-1.5 py-0.5 text-xs font-medium">{evt.name || 'Unknown'}</span>
        {evt.category && <span className="text-xs text-gray-400 dark:text-slate-500">{evt.category}</span>}
        {/* Key detail inline: DNS name, file path, registry key, network dest */}
        {params.name && (evt.name === 'QueryDns' || evt.name === 'ReplyDns') && (
          <span className="rounded bg-cyan-100 dark:bg-cyan-900/30 text-cyan-800 dark:text-cyan-300 px-2 py-0.5 text-xs font-mono font-medium">{String(params.name)}</span>
        )}
        {!!params.file_path && (
          <span className="text-xs font-mono text-gray-600 dark:text-slate-400 truncate max-w-md">{String(params.file_path)}</span>
        )}
        {!!params.file_name && !params.file_path && (
          <span className="text-xs font-mono text-gray-600 dark:text-slate-400 truncate max-w-md">{String(params.file_name)}</span>
        )}
        {!!params.key_name && (
          <span className="text-xs font-mono text-gray-600 dark:text-slate-400 truncate max-w-md">{String(params.key_name)}</span>
        )}
        {params.dip && (
          <span className="text-xs font-mono text-gray-600 dark:text-slate-400">{String(params.dip)}:{String(params.dport || '')}</span>
        )}
        {evt.timestamp && <span className="ml-auto text-xs text-gray-400 dark:text-slate-500">{new Date(evt.timestamp).toLocaleString()}</span>}
      </div>
      <div className="px-3 py-2.5 space-y-3">
        {proc && (
          <>
            {/* ── Process Card ── */}
            <div className="rounded-lg border border-blue-200 dark:border-blue-800/50 bg-blue-50/30 dark:bg-blue-950/30 p-3 space-y-2">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="rounded bg-blue-100 dark:bg-blue-900/50 text-blue-700 dark:text-blue-300 px-1.5 py-0.5 text-[10px] font-bold uppercase">Process</span>
                <span className="font-medium text-sm text-gray-900 dark:text-slate-100">{proc.name}</span>
                {proc.pid != null && <span className="text-xs text-gray-400 dark:text-slate-500 font-mono">PID {proc.pid}</span>}
                {proc.tid != null && <span className="text-xs text-gray-400 dark:text-slate-500 font-mono">TID {proc.tid}</span>}
                {proc.username && <span className="text-xs text-gray-500 dark:text-slate-400">{proc.domain}\\{proc.username}</span>}
                {proc.integrity_level && <span className="rounded bg-gray-100 dark:bg-slate-700 text-gray-600 dark:text-slate-400 px-1 py-0.5 text-[10px]">{proc.integrity_level}</span>}
                {proc.is_wow64 && <span className="rounded bg-yellow-50 dark:bg-yellow-900/30 text-yellow-700 dark:text-yellow-400 px-1 py-0.5 text-[10px] font-medium">WOW64</span>}
                {proc.is_protected && <span className="rounded bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400 px-1 py-0.5 text-[10px] font-medium">Protected</span>}
              </div>
              {proc.exe && <div><span className="text-[10px] text-gray-400 dark:text-slate-500">Executable</span><p className="text-xs text-gray-800 dark:text-slate-200 font-mono break-all">{proc.exe}</p></div>}
              {proc.cmdline && (
                <div className="rounded bg-gray-900 dark:bg-black px-2 py-1.5 text-[11px] text-gray-100 font-mono break-all whitespace-pre-wrap">{proc.cmdline}</div>
              )}
              {proc.cwd && <div><span className="text-[10px] text-gray-400 dark:text-slate-500">CWD</span><p className="text-[10px] text-gray-700 dark:text-slate-300 font-mono break-all">{proc.cwd}</p></div>}
              {proc.session_id != null && <div><span className="text-[10px] text-gray-400 dark:text-slate-500">Session</span><span className="ml-1 text-[10px] text-gray-700 dark:text-slate-300 font-mono">{proc.session_id}</span></div>}
              <div className="flex items-center gap-2 flex-wrap">
                {proc.is_signed !== undefined && (
                  <span className={'rounded px-1.5 py-0.5 text-[10px] font-medium ' + (proc.is_signed ? 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400' : 'bg-red-50 dark:bg-red-900/30 text-red-700 dark:text-red-400')}>
                    {proc.is_signed ? 'Signed' : 'Unsigned'}
                  </span>
                )}
                {proc.is_trusted !== undefined && proc.is_signed && (
                  <span className={'rounded px-1.5 py-0.5 text-[10px] font-medium ' + (proc.is_trusted ? 'bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400' : 'bg-amber-50 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400')}>
                    {proc.is_trusted ? 'Trusted' : 'Untrusted'}
                  </span>
                )}
                {proc.cert_subject && <span className="text-[10px] text-gray-500 dark:text-slate-400">{proc.cert_subject}</span>}
              </div>
              {(proc.sha256 || proc.md5) && (
                <div className="space-y-0.5">
                  {proc.sha256 && <div><span className="text-[10px] text-gray-400 dark:text-slate-500">SHA256</span><p className="text-[10px] text-gray-700 dark:text-slate-300 font-mono break-all">{proc.sha256}</p></div>}
                  {proc.md5 && <div><span className="text-[10px] text-gray-400 dark:text-slate-500">MD5</span><p className="text-[10px] text-gray-700 dark:text-slate-300 font-mono break-all">{proc.md5}</p></div>}
                </div>
              )}
            </div>

            {/* ── Parent Process Card ── */}
            {proc.parent_name && (
              <div className="rounded-lg border border-amber-200 dark:border-amber-800/50 bg-amber-50/30 dark:bg-amber-950/30 p-3 space-y-2">
                <div className="flex items-center gap-2">
                  <span className="rounded bg-amber-100 dark:bg-amber-900/50 text-amber-700 dark:text-amber-300 px-1.5 py-0.5 text-[10px] font-bold uppercase">Parent</span>
                  <span className="font-medium text-sm text-gray-900 dark:text-slate-100">{proc.parent_name}</span>
                  {proc.ppid != null && <span className="text-xs text-gray-400 dark:text-slate-500 font-mono">PID {proc.ppid}</span>}
                </div>
                {proc.parent_cmdline && (
                  <div className="rounded bg-gray-900 dark:bg-black px-2 py-1.5 text-[11px] text-gray-100 font-mono break-all whitespace-pre-wrap">{proc.parent_cmdline}</div>
                )}
              </div>
            )}

            {/* ── Ancestry Chain ── */}
            {proc.ancestors && proc.ancestors.length > 0 && (
              <div>
                <span className="text-[10px] text-gray-400 dark:text-slate-500 uppercase font-medium">Process Ancestry</span>
                <div className="mt-1 flex items-center gap-1 flex-wrap">
                  {proc.ancestors.map((a, i) => (
                    <span key={i} className="flex items-center gap-0.5">
                      <span className="rounded bg-blue-50 dark:bg-blue-900/30 border border-blue-200 dark:border-blue-700 px-1.5 py-0.5 text-[10px] font-mono text-blue-700 dark:text-blue-300">{a}</span>
                      {i < proc.ancestors!.length - 1 && <span className="text-gray-300 dark:text-slate-600 text-xs">&larr;</span>}
                    </span>
                  ))}
                </div>
              </div>
            )}
          </>
        )}

        {/* ── Event Parameters ── */}
        {paramEntries.length > 0 && (
          <div>
            <span className="text-[10px] text-gray-400 dark:text-slate-500 uppercase font-medium">Event Parameters</span>
            <div className="mt-1.5 rounded-lg border border-gray-200 dark:border-slate-700 bg-gray-50 dark:bg-slate-900/50 overflow-hidden">
              <table className="w-full text-[11px]">
                <tbody className="divide-y divide-gray-100 dark:divide-slate-700/50">
                  {paramEntries.map(([k, v]) => (
                    <tr key={k}>
                      <td className="px-2.5 py-1 text-gray-500 dark:text-slate-500 font-medium whitespace-nowrap align-top w-1/4">{k}</td>
                      <td className="px-2.5 py-1 text-gray-800 dark:text-slate-200 font-mono break-all">
                        {typeof v === 'object' && v !== null ? JSON.stringify(v) : String(v ?? '')}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {/* ── Callstack ── */}
        {callstack.length > 0 && (
          <div>
            <span className="text-[10px] text-gray-400 dark:text-slate-500 uppercase font-medium">Call Stack</span>
            <div className="mt-1.5 rounded-lg bg-gray-900 dark:bg-black border border-gray-700 dark:border-slate-700 p-2.5 max-h-48 overflow-auto">
              {callstack.map((frame, i) => (
                <div key={i} className="flex items-start gap-2 py-0.5">
                  <span className="text-[9px] text-gray-500 font-mono tabular-nums flex-shrink-0 w-4 text-right">{i}</span>
                  <span className="text-[10px] text-emerald-400 font-mono break-all">{frame}</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* ── Raw JSON toggle ── */}
        <div>
          <button
            onClick={() => setShowRaw(!showRaw)}
            className="flex items-center gap-1.5 text-[10px] text-gray-400 dark:text-slate-500 hover:text-gray-600 dark:hover:text-slate-300 uppercase font-medium"
          >
            <svg className={'w-3 h-3 transition-transform ' + (showRaw ? 'rotate-90' : '')} fill="currentColor" viewBox="0 0 20 20">
              <path fillRule="evenodd" d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z" clipRule="evenodd" />
            </svg>
            Raw JSON
          </button>
          {showRaw && (
            <pre className="mt-1.5 max-h-64 overflow-auto rounded-lg bg-gray-900 dark:bg-black border border-gray-700 dark:border-slate-700 p-3 text-[10px] text-gray-200 font-mono whitespace-pre-wrap">
              {JSON.stringify(evt, null, 2)}
            </pre>
          )}
        </div>
      </div>
    </div>
  )
}
