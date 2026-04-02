import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts'
import { api, type Detection, type TimelineBucket } from '../lib/api'
import SeverityBadge from '../components/SeverityBadge'
import SlidePanel from '../components/SlidePanel'

export default function Detections() {
  const [page, setPage] = useState(1)
  const [severityFilter, setSeverityFilter] = useState('')
  const [selectedDet, setSelectedDet] = useState<Detection | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['detections', page, severityFilter],
    queryFn: () =>
      api.getDetections({
        page: String(page),
        severity: severityFilter,
      }),
  })

  const { data: timeline } = useQuery({
    queryKey: ['detection-timeline-page'],
    queryFn: () => api.getDetectionTimeline(),
  })

  const detections = (data?.data || []) as Detection[]
  const total = data?.meta?.total ?? 0
  const perPage = data?.meta?.per_page ?? 50
  const totalPages = Math.ceil(total / perPage)
  const timelineData = (timeline?.data || []) as TimelineBucket[]

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900">Detections</h1>
      <p className="mt-1 text-sm text-gray-500">{total} detection(s) total</p>

      {/* Timeline chart */}
      <div className="mt-6 rounded-xl border border-gray-200 bg-white p-6 shadow-sm">
        <h3 className="text-sm font-medium text-gray-500">Detection Timeline</h3>
        {timelineData.length > 0 ? (
          <div className="mt-4 h-48">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={timelineData}>
                <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
                <XAxis
                  dataKey="timestamp"
                  tickFormatter={(t) =>
                    new Date(t).toLocaleTimeString([], {
                      hour: '2-digit',
                      minute: '2-digit',
                    })
                  }
                  fontSize={12}
                />
                <YAxis fontSize={12} />
                <Tooltip
                  labelFormatter={(t) => new Date(t as string).toLocaleString()}
                />
                <Bar dataKey="count" fill="#4c6ef5" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        ) : (
          <p className="mt-4 text-sm text-gray-400">No timeline data available.</p>
        )}
      </div>

      {/* Filters */}
      <div className="mt-6 flex gap-4">
        <select
          value={severityFilter}
          onChange={(e) => {
            setSeverityFilter(e.target.value)
            setPage(1)
          }}
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
                <tr>
                  <td colSpan={5} className="px-6 py-12 text-center text-gray-400">
                    Loading...
                  </td>
                </tr>
              )}
              {!isLoading &&
                detections.map((det) => (
                  <tr
                    key={det.id}
                    className="cursor-pointer hover:bg-gray-50/50"
                    onClick={() => setSelectedDet(det)}
                  >
                    <td className="px-6 py-3">
                      <div className="font-medium text-gray-900">
                        {det.title || det.rule_name}
                      </div>
                      {det.text && (
                        <div className="mt-0.5 text-xs text-gray-500 line-clamp-1">
                          {det.text}
                        </div>
                      )}
                    </td>
                    <td className="px-6 py-3 text-gray-600">
                      {det.agent_hostname || det.agent_id?.slice(0, 8)}
                    </td>
                    <td className="px-6 py-3">
                      <SeverityBadge severity={det.severity} />
                    </td>
                    <td className="px-6 py-3 text-gray-600">
                      {det.labels?.['technique.id'] && (
                        <span className="rounded bg-gray-100 px-1.5 py-0.5 text-xs font-mono">
                          {det.labels['technique.id']}
                        </span>
                      )}
                    </td>
                    <td className="px-6 py-3 text-gray-500 whitespace-nowrap">
                      {new Date(det.timestamp).toLocaleString()}
                    </td>
                  </tr>
                ))}
              {!isLoading && detections.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-6 py-12 text-center text-gray-400">
                    No detections yet.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        {/* Pagination */}
        {total > perPage && (
          <div className="flex items-center justify-between border-t border-gray-100 px-6 py-3">
            <span className="text-sm text-gray-500">
              Page {page} of {totalPages}
            </span>
            <div className="flex gap-2">
              <button
                onClick={() => setPage(Math.max(1, page - 1))}
                disabled={page <= 1}
                className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm disabled:opacity-50"
              >
                Previous
              </button>
              <button
                onClick={() => setPage(page + 1)}
                disabled={page >= totalPages}
                className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm disabled:opacity-50"
              >
                Next
              </button>
            </div>
          </div>
        )}
      </div>

      {/* Detection detail slide-out panel */}
      <SlidePanel
        open={!!selectedDet}
        title="Detection Detail"
        onClose={() => setSelectedDet(null)}
      >
        {selectedDet && (
          <div className="space-y-6">
            <div>
              <h3 className="text-xl font-bold text-gray-900">{selectedDet.title}</h3>
              <div className="mt-2 flex items-center gap-3">
                <SeverityBadge severity={selectedDet.severity} />
                <span className="text-sm text-gray-500">
                  {selectedDet.agent_hostname}
                </span>
                <span className="text-sm text-gray-500">
                  {new Date(selectedDet.timestamp).toLocaleString()}
                </span>
              </div>
            </div>

            {selectedDet.text && (
              <div>
                <h4 className="text-sm font-medium text-gray-500">Alert Text</h4>
                <p className="mt-1 text-sm text-gray-700">{selectedDet.text}</p>
              </div>
            )}

            {selectedDet.description && (
              <div>
                <h4 className="text-sm font-medium text-gray-500">Description</h4>
                <p className="mt-1 text-sm text-gray-700">{selectedDet.description}</p>
              </div>
            )}

            {selectedDet.labels &&
              Object.keys(selectedDet.labels).length > 0 && (
                <div>
                  <h4 className="text-sm font-medium text-gray-500">MITRE ATT&CK</h4>
                  <div className="mt-2 grid grid-cols-2 gap-2">
                    {Object.entries(selectedDet.labels).map(([k, v]) => (
                      <div key={k} className="rounded-lg bg-gray-50 px-3 py-2">
                        <span className="text-xs text-gray-500">{k}</span>
                        <p className="text-sm font-medium text-gray-900">{v}</p>
                      </div>
                    ))}
                  </div>
                </div>
              )}

            {selectedDet.events && (
              <div>
                <h4 className="text-sm font-medium text-gray-500">Event Data</h4>
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
