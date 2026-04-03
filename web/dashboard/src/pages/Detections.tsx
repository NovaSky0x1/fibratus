import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type Detection } from '../lib/api'
import SeverityBadge from '../components/SeverityBadge'
import SlidePanel from '../components/SlidePanel'

interface DetectionEvent {
  name?: string
  category?: string
  timestamp?: string
  params?: Record<string, unknown>
  callstack?: string[]
  proc?: {
    pid?: number
    tid?: number
    ppid?: number
    name?: string
    exe?: string
    cmdline?: string
    parent_name?: string
    parent_cmdline?: string
    cwd?: string
    sid?: string
    username?: string
    domain?: string
    session_id?: number
    integrity_level?: string
    is_wow64?: boolean
    is_packaged?: boolean
    is_protected?: boolean
    ancestors?: string[]
  }
}

function parseEvents(events: unknown): DetectionEvent[] {
  if (!events) return []
  if (Array.isArray(events)) return events as DetectionEvent[]
  if (typeof events === 'string') {
    try { return JSON.parse(events) } catch { return [] }
  }
  return []
}

function EventDetail({ evt }: { evt: DetectionEvent }) {
  const proc = evt.proc
  return (
    <div className="rounded-lg border border-gray-200 bg-white">
      {/* Event header */}
      <div className="flex items-center gap-2 border-b border-gray-100 px-4 py-2.5 bg-gray-50/50">
        <span className="rounded bg-blue-100 text-blue-700 px-1.5 py-0.5 text-xs font-medium">{evt.name || 'Unknown'}</span>
        {evt.category && <span className="text-xs text-gray-400">{evt.category}</span>}
        {evt.timestamp && <span className="ml-auto text-xs text-gray-400 tabular-nums">{new Date(evt.timestamp).toLocaleString()}</span>}
      </div>

      {/* Process info */}
      {proc && (
        <div className="px-4 py-3 space-y-2">
          <div className="grid grid-cols-2 gap-x-6 gap-y-1.5 text-sm">
            {proc.name && <Field label="Process" value={proc.name} mono />}
            {proc.pid != null && <Field label="PID" value={String(proc.pid)} mono />}
            {proc.exe && <Field label="Executable" value={proc.exe} mono />}
            {proc.ppid != null && <Field label="Parent PID" value={String(proc.ppid)} mono />}
            {proc.parent_name && <Field label="Parent" value={proc.parent_name} mono />}
            {proc.username && <Field label="User" value={`${proc.domain || ''}\\${proc.username}`} />}
            {proc.integrity_level && <Field label="Integrity" value={proc.integrity_level} />}
            {proc.session_id != null && <Field label="Session" value={String(proc.session_id)} mono />}
          </div>

          {proc.cmdline && (
            <div className="mt-2">
              <span className="text-xs text-gray-500">Command Line</span>
              <div className="mt-0.5 rounded bg-gray-900 px-3 py-2 text-xs text-gray-100 font-mono break-all whitespace-pre-wrap">
                {proc.cmdline}
              </div>
            </div>
          )}

          {proc.parent_cmdline && (
            <div className="mt-2">
              <span className="text-xs text-gray-500">Parent Command Line</span>
              <div className="mt-0.5 rounded bg-gray-900 px-3 py-2 text-xs text-gray-100 font-mono break-all whitespace-pre-wrap">
                {proc.parent_cmdline}
              </div>
            </div>
          )}

          {proc.ancestors && proc.ancestors.length > 0 && (
            <div className="mt-2">
              <span className="text-xs text-gray-500">Process Ancestry</span>
              <div className="mt-1 flex items-center gap-1 flex-wrap">
                {proc.ancestors.map((a, i) => (
                  <span key={i} className="flex items-center gap-1">
                    <span className="rounded bg-gray-100 px-1.5 py-0.5 text-xs font-mono text-gray-700">{a}</span>
                    {i < proc.ancestors!.length - 1 && <span className="text-gray-300 text-xs">&larr;</span>}
                  </span>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Event params */}
      {evt.params && Object.keys(evt.params).length > 0 && (
        <div className="border-t border-gray-100 px-4 py-3">
          <span className="text-xs text-gray-500 font-medium">Parameters</span>
          <div className="mt-1.5 grid grid-cols-2 gap-x-6 gap-y-1 text-sm">
            {Object.entries(evt.params).map(([k, v]) => (
              <Field key={k} label={k} value={typeof v === 'object' ? JSON.stringify(v) : String(v)} mono />
            ))}
          </div>
        </div>
      )}

      {/* Callstack */}
      {evt.callstack && evt.callstack.length > 0 && (
        <details className="border-t border-gray-100 px-4 py-3">
          <summary className="text-xs text-fibratus-600 cursor-pointer hover:text-fibratus-800 font-medium">
            Call Stack ({evt.callstack.length} frames)
          </summary>
          <div className="mt-2 rounded bg-gray-900 px-3 py-2 text-xs text-gray-100 font-mono max-h-48 overflow-auto">
            {evt.callstack.map((frame, i) => (
              <div key={i} className="py-0.5">{frame}</div>
            ))}
          </div>
        </details>
      )}
    </div>
  )
}

function Field({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="min-w-0">
      <span className="text-xs text-gray-400">{label}</span>
      <p className={'text-sm text-gray-800 truncate ' + (mono ? 'font-mono' : '')} title={value}>{value}</p>
    </div>
  )
}

export default function Detections() {
  const [page, setPage] = useState(1)
  const [severityFilter, setSeverityFilter] = useState('')
  const [selectedDet, setSelectedDet] = useState<Detection | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['detections', page, severityFilter],
    queryFn: () => api.getDetections({ page: String(page), severity: severityFilter }),
    refetchInterval: 30000,
  })

  const detections = (data?.data || []) as Detection[]
  const total = data?.meta?.total ?? 0
  const perPage = data?.meta?.per_page ?? 50
  const totalPages = Math.ceil(total / perPage)

  const selectedEvents = selectedDet ? parseEvents(selectedDet.events) : []

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900">Detections</h1>
      <p className="mt-1 text-sm text-gray-500">{total} detection(s)</p>

      {/* Filters */}
      <div className="mt-6 flex gap-4">
        <select
          value={severityFilter}
          onChange={(e) => { setSeverityFilter(e.target.value); setPage(1) }}
          className="rounded-lg border border-gray-300 px-4 py-2 text-sm focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
        >
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
              {isLoading && (
                <tr><td colSpan={5} className="px-6 py-12 text-center text-gray-400">Loading...</td></tr>
              )}
              {!isLoading && detections.map((det) => (
                <tr key={det.id} className="cursor-pointer hover:bg-gray-50/50" onClick={() => setSelectedDet(det)}>
                  <td className="px-6 py-3">
                    <div className="font-medium text-gray-900">{det.title || det.rule_name}</div>
                    {det.text && <div className="mt-0.5 text-xs text-gray-500 line-clamp-1">{det.text}</div>}
                  </td>
                  <td className="px-6 py-3 text-gray-600">{det.agent_hostname || det.agent_id?.slice(0, 8)}</td>
                  <td className="px-6 py-3"><SeverityBadge severity={det.severity} /></td>
                  <td className="px-6 py-3 text-gray-600">
                    {det.labels?.['technique.id'] && (
                      <span className="rounded bg-gray-100 px-1.5 py-0.5 text-xs font-mono">{det.labels['technique.id']}</span>
                    )}
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

      {/* Detection detail */}
      <SlidePanel open={!!selectedDet} title="Detection Detail" onClose={() => setSelectedDet(null)} wide>
        {selectedDet && (
          <div className="space-y-6">
            {/* Header */}
            <div>
              <h3 className="text-xl font-bold text-gray-900">{selectedDet.title}</h3>
              <div className="mt-2 flex items-center gap-3 flex-wrap">
                <SeverityBadge severity={selectedDet.severity} />
                <span className="text-sm text-gray-500">{selectedDet.agent_hostname}</span>
                <span className="text-sm text-gray-400">{new Date(selectedDet.timestamp).toLocaleString()}</span>
                {selectedDet.rule_id && (
                  <a
                    href={`/rules?highlight=${selectedDet.rule_id}`}
                    className="text-xs text-fibratus-600 hover:text-fibratus-800 hover:underline"
                  >
                    View Rule
                  </a>
                )}
              </div>
            </div>

            {/* Alert text */}
            {selectedDet.text && (
              <div>
                <h4 className="text-sm font-medium text-gray-500">Alert</h4>
                <p className="mt-1 text-sm text-gray-700">{selectedDet.text}</p>
              </div>
            )}

            {selectedDet.description && (
              <div>
                <h4 className="text-sm font-medium text-gray-500">Description</h4>
                <p className="mt-1 text-sm text-gray-700">{selectedDet.description}</p>
              </div>
            )}

            {/* MITRE ATT&CK */}
            {selectedDet.labels && Object.keys(selectedDet.labels).length > 0 && (
              <div>
                <h4 className="text-sm font-medium text-gray-500">MITRE ATT&CK</h4>
                <div className="mt-2 flex flex-wrap gap-2">
                  {Object.entries(selectedDet.labels).map(([k, v]) => (
                    <div key={k} className="rounded-lg bg-gray-50 px-3 py-1.5">
                      <span className="text-[10px] text-gray-400 uppercase">{k.replace('tactic.', '').replace('technique.', '')}</span>
                      <p className="text-xs font-medium text-gray-800">{v}</p>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Parsed events */}
            {selectedEvents.length > 0 && (
              <div>
                <h4 className="text-sm font-medium text-gray-500">
                  Triggering Event{selectedEvents.length > 1 ? 's' : ''} ({selectedEvents.length})
                </h4>
                <div className="mt-2 space-y-3">
                  {selectedEvents.map((evt, i) => (
                    <EventDetail key={i} evt={evt} />
                  ))}
                </div>
              </div>
            )}

            {/* Raw JSON fallback */}
            {selectedEvents.length === 0 && selectedDet.events != null && (
              <div>
                <h4 className="text-sm font-medium text-gray-500">Event Data (raw)</h4>
                <pre className="mt-2 max-h-96 overflow-auto rounded-lg bg-gray-900 p-4 text-xs text-gray-100 font-mono">
                  {JSON.stringify(selectedDet.events, null, 2)}
                </pre>
              </div>
            )}
          </div>
        )}
      </SlidePanel>
    </div>
  )
}
