import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type Detection, type Rule } from '../lib/api'
import SeverityBadge from '../components/SeverityBadge'
import SlidePanel from '../components/SlidePanel'
import DetectionProcessGraph from '../components/DetectionProcessGraph'

interface DetectionEvent {
  name?: string; category?: string; timestamp?: string
  params?: Record<string, unknown>; callstack?: string[]
  proc?: {
    pid?: number; tid?: number; ppid?: number; name?: string; exe?: string
    cmdline?: string; parent_name?: string; parent_cmdline?: string
    cwd?: string; sid?: string; username?: string; domain?: string
    session_id?: number; integrity_level?: string; ancestors?: string[]
  }
}

function parseEvents(events: unknown): DetectionEvent[] {
  if (!events) return []
  if (Array.isArray(events)) return events as DetectionEvent[]
  if (typeof events === 'string') { try { return JSON.parse(events) } catch { return [] } }
  return []
}

type DetailView = 'detail' | 'tree'

export default function Detections() {
  const [page, setPage] = useState(1)
  const [severityFilter, setSeverityFilter] = useState('')
  const [selectedDet, setSelectedDet] = useState<Detection | null>(null)
  const [detailView, setDetailView] = useState<DetailView>('detail')

  const { data, isLoading } = useQuery({
    queryKey: ['detections', page, severityFilter],
    queryFn: () => api.getDetections({ page: String(page), severity: severityFilter }),
    refetchInterval: 30000,
  })

  // Fetch rules to resolve rule links
  const { data: rulesData } = useQuery({
    queryKey: ['rules-for-lookup'],
    queryFn: () => api.getRules(),
    staleTime: 60000,
  })
  const rulesMap = new Map<string, Rule>()
  if (rulesData?.data) {
    for (const r of rulesData.data as Rule[]) { rulesMap.set(r.id, r) }
  }

  const detections = (data?.data || []) as Detection[]
  const total = data?.meta?.total ?? 0
  const perPage = data?.meta?.per_page ?? 50
  const totalPages = Math.ceil(total / perPage)

  const selectedEvents = selectedDet ? parseEvents(selectedDet.events) : []
  const focusProc = selectedEvents[0]?.proc
  const matchedRule = selectedDet?.rule_id ? rulesMap.get(selectedDet.rule_id) : null

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900">Detections</h1>
      <p className="mt-1 text-sm text-gray-500">{total} detection(s)</p>

      <div className="mt-6 flex gap-4">
        <select value={severityFilter} onChange={e => { setSeverityFilter(e.target.value); setPage(1) }}
          className="rounded-lg border border-gray-300 px-4 py-2 text-sm focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500">
          <option value="">All severities</option>
          <option value="critical">Critical</option>
          <option value="high">High</option>
          <option value="medium">Medium</option>
          <option value="low">Low</option>
        </select>
      </div>

      {/* Detection table */}
      <div className="mt-4 rounded-xl border border-gray-200 bg-white shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 bg-gray-50/50">
              <tr>
                <th className="px-6 py-3 font-medium text-gray-500">Rule</th>
                <th className="px-6 py-3 font-medium text-gray-500">Agent</th>
                <th className="px-6 py-3 font-medium text-gray-500">Severity</th>
                <th className="px-6 py-3 font-medium text-gray-500">MITRE</th>
                <th className="px-6 py-3 font-medium text-gray-500">Time</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {isLoading && <tr><td colSpan={5} className="px-6 py-12 text-center text-gray-400">Loading...</td></tr>}
              {!isLoading && detections.map(det => (
                <tr key={det.id} className="cursor-pointer hover:bg-gray-50/50"
                  onClick={() => { setSelectedDet(det); setDetailView('detail') }}>
                  <td className="px-6 py-3">
                    <div className="font-medium text-gray-900">{det.title || det.rule_name}</div>
                    {det.text && <div className="mt-0.5 text-xs text-gray-500 line-clamp-1">{det.text}</div>}
                  </td>
                  <td className="px-6 py-3 text-gray-600">{det.agent_hostname || det.agent_id?.slice(0, 8)}</td>
                  <td className="px-6 py-3"><SeverityBadge severity={det.severity} /></td>
                  <td className="px-6 py-3 text-gray-600">
                    {det.labels?.['technique.id'] && <span className="rounded bg-gray-100 px-1.5 py-0.5 text-xs font-mono">{det.labels['technique.id']}</span>}
                  </td>
                  <td className="px-6 py-3 text-gray-500 whitespace-nowrap">{new Date(det.timestamp).toLocaleString()}</td>
                </tr>
              ))}
              {!isLoading && detections.length === 0 && (
                <tr><td colSpan={5} className="px-6 py-12 text-center text-gray-400">No detections yet.</td></tr>
              )}
            </tbody>
          </table>
        </div>
        {total > perPage && (
          <div className="flex items-center justify-between border-t border-gray-100 px-6 py-3">
            <span className="text-sm text-gray-500">Page {page} of {totalPages}</span>
            <div className="flex gap-2">
              <button onClick={() => setPage(Math.max(1, page - 1))} disabled={page <= 1} className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm disabled:opacity-50">Previous</button>
              <button onClick={() => setPage(page + 1)} disabled={page >= totalPages} className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm disabled:opacity-50">Next</button>
            </div>
          </div>
        )}
      </div>

      {/* Detection detail panel */}
      <SlidePanel open={!!selectedDet} title="Detection" onClose={() => setSelectedDet(null)} wide>
        {selectedDet && (
          <div>
            {/* View tabs */}
            <div className="flex gap-1 border-b border-gray-200 mb-4">
              {(['detail', 'tree'] as const).map(tab => (
                <button key={tab} onClick={() => setDetailView(tab)}
                  className={'px-4 py-2 text-sm font-medium border-b-2 -mb-px ' +
                    (detailView === tab ? 'border-fibratus-600 text-fibratus-600' : 'border-transparent text-gray-500 hover:text-gray-700')}>
                  {tab === 'detail' ? 'Details' : 'Process Tree'}
                </button>
              ))}
            </div>

            {detailView === 'detail' && (
              <div className="space-y-5">
                {/* Header */}
                <div>
                  <h3 className="text-lg font-bold text-gray-900">{selectedDet.title}</h3>
                  <div className="mt-2 flex items-center gap-3 flex-wrap">
                    <SeverityBadge severity={selectedDet.severity} />
                    <span className="text-sm text-gray-500">{selectedDet.agent_hostname}</span>
                    <span className="text-sm text-gray-400">{new Date(selectedDet.timestamp).toLocaleString()}</span>
                  </div>
                </div>

                {/* Alert text */}
                {selectedDet.text && (
                  <div>
                    <h4 className="text-xs font-medium text-gray-500 uppercase tracking-wider">Alert</h4>
                    <p className="mt-1 text-sm text-gray-700">{selectedDet.text}</p>
                  </div>
                )}

                {selectedDet.description && (
                  <div>
                    <h4 className="text-xs font-medium text-gray-500 uppercase tracking-wider">Description</h4>
                    <p className="mt-1 text-sm text-gray-700">{selectedDet.description}</p>
                  </div>
                )}

                {/* MITRE ATT&CK */}
                {selectedDet.labels && Object.keys(selectedDet.labels).length > 0 && (
                  <div>
                    <h4 className="text-xs font-medium text-gray-500 uppercase tracking-wider">MITRE ATT&CK</h4>
                    <div className="mt-2 flex flex-wrap gap-2">
                      {Object.entries(selectedDet.labels).map(([k, v]) => (
                        <div key={k} className="rounded bg-gray-50 px-2.5 py-1">
                          <span className="text-[10px] text-gray-400 uppercase">{k.replace('tactic.', '').replace('technique.', '')}</span>
                          <p className="text-xs font-medium text-gray-800">{v}</p>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* Rule actions */}
                {matchedRule && (
                  <div>
                    <h4 className="text-xs font-medium text-gray-500 uppercase tracking-wider">Rule Actions</h4>
                    <div className="mt-2 space-y-2">
                      <a href={`/rules?rule=${matchedRule.id}`}
                        className="flex items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 hover:bg-gray-50 text-sm">
                        <span className="text-fibratus-600 font-medium">{matchedRule.name}</span>
                        <span className="text-xs text-gray-400">v{matchedRule.version}</span>
                        <span className={'ml-auto rounded-full px-2 py-0.5 text-[10px] font-medium ' +
                          (matchedRule.enabled ? 'bg-emerald-50 text-emerald-700' : 'bg-gray-100 text-gray-500')}>
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
                        <div className="rounded-lg bg-gray-50 border border-gray-200 px-3 py-2">
                          <span className="text-xs text-gray-500">No response actions — alert only</span>
                        </div>
                      )}
                    </div>
                  </div>
                )}
                {!matchedRule && selectedDet.rule_id && (
                  <div>
                    <h4 className="text-xs font-medium text-gray-500 uppercase tracking-wider">Rule</h4>
                    <p className="text-xs text-gray-400 mt-1">Rule {selectedDet.rule_id.slice(0, 12)}... (not found in current ruleset)</p>
                  </div>
                )}

                {/* Triggering events */}
                {selectedEvents.length > 0 && (
                  <div>
                    <h4 className="text-xs font-medium text-gray-500 uppercase tracking-wider">
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
                    <h4 className="text-xs font-medium text-gray-500 uppercase tracking-wider">Event Data</h4>
                    <pre className="mt-2 max-h-64 overflow-auto rounded bg-gray-900 p-3 text-xs text-gray-100 font-mono">
                      {JSON.stringify(selectedDet.events, null, 2)}
                    </pre>
                  </div>
                )}
              </div>
            )}

            {detailView === 'tree' && (
              <DetectionProcessGraph
                detection={selectedDet}
                focusPid={focusProc?.pid}
                focusProcessName={focusProc?.name}
              />
            )}
          </div>
        )}
      </SlidePanel>
    </div>
  )
}

function EventCard({ evt }: { evt: DetectionEvent }) {
  const proc = evt.proc
  return (
    <div className="rounded-lg border border-gray-200 bg-white text-sm">
      <div className="flex items-center gap-2 border-b border-gray-100 px-3 py-2 bg-gray-50/50">
        <span className="rounded bg-blue-100 text-blue-700 px-1.5 py-0.5 text-xs font-medium">{evt.name || 'Unknown'}</span>
        {evt.category && <span className="text-xs text-gray-400">{evt.category}</span>}
        {evt.timestamp && <span className="ml-auto text-xs text-gray-400">{new Date(evt.timestamp).toLocaleString()}</span>}
      </div>
      {proc && (
        <div className="px-3 py-2.5 space-y-2">
          <div className="grid grid-cols-2 gap-x-4 gap-y-1">
            {proc.name && <div><span className="text-xs text-gray-400">Process</span> <span className="text-xs font-mono text-gray-700">{proc.name}</span></div>}
            {proc.pid != null && <div><span className="text-xs text-gray-400">PID</span> <span className="text-xs font-mono text-gray-700">{proc.pid}</span></div>}
            {proc.parent_name && <div><span className="text-xs text-gray-400">Parent</span> <span className="text-xs font-mono text-gray-700">{proc.parent_name}</span></div>}
            {proc.username && <div><span className="text-xs text-gray-400">User</span> <span className="text-xs text-gray-700">{proc.domain}\\{proc.username}</span></div>}
            {proc.integrity_level && <div><span className="text-xs text-gray-400">Integrity</span> <span className="text-xs text-gray-700">{proc.integrity_level}</span></div>}
          </div>
          {proc.cmdline && (
            <div className="rounded bg-gray-900 px-2 py-1.5 text-[11px] text-gray-100 font-mono break-all whitespace-pre-wrap">{proc.cmdline}</div>
          )}
          {proc.ancestors && proc.ancestors.length > 0 && (
            <div className="flex items-center gap-1 flex-wrap">
              <span className="text-[10px] text-gray-400">Ancestry:</span>
              {proc.ancestors.map((a, i) => (
                <span key={i} className="flex items-center gap-0.5">
                  <span className="rounded bg-gray-100 px-1 py-0.5 text-[10px] font-mono text-gray-600">{a}</span>
                  {i < proc.ancestors!.length - 1 && <span className="text-gray-300 text-[10px]">&larr;</span>}
                </span>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
