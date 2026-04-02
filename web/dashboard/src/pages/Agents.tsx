import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type Agent } from '../lib/api'
import StatusBadge from '../components/StatusBadge'
import SlidePanel from '../components/SlidePanel'
import ConfirmDialog from '../components/ConfirmDialog'

export default function Agents() {
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [page, setPage] = useState(1)
  const [selectedAgent, setSelectedAgent] = useState<Agent | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Agent | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['agents', page, statusFilter, search],
    queryFn: () =>
      api.getAgents({
        page: String(page),
        status: statusFilter,
        search,
      }),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteAgent(id),
    onSuccess: () => {
      setDeleteTarget(null)
      setSelectedAgent(null)
      queryClient.invalidateQueries({ queryKey: ['agents'] })
    },
  })

  const agents = (data?.data || []) as Agent[]
  const total = data?.meta?.total ?? 0
  const perPage = data?.meta?.per_page ?? 50
  const totalPages = Math.ceil(total / perPage)

  return (
    <div>
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Agents</h1>
          <p className="mt-1 text-sm text-gray-500">{total} agent(s) registered</p>
        </div>
      </div>

      {/* Filters */}
      <div className="mt-6 flex gap-4">
        <input
          type="text"
          placeholder="Search by hostname..."
          value={search}
          onChange={(e) => {
            setSearch(e.target.value)
            setPage(1)
          }}
          className="rounded-lg border border-gray-300 px-4 py-2 text-sm focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
        />
        <select
          value={statusFilter}
          onChange={(e) => {
            setStatusFilter(e.target.value)
            setPage(1)
          }}
          className="rounded-lg border border-gray-300 px-4 py-2 text-sm focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
        >
          <option value="">All statuses</option>
          <option value="online">Online</option>
          <option value="offline">Offline</option>
          <option value="stale">Stale</option>
        </select>
      </div>

      {/* Agent table */}
      <div className="mt-6 rounded-xl border border-gray-200 bg-white shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 bg-gray-50/50">
              <tr>
                <th className="px-6 py-3 font-medium text-gray-500">Hostname</th>
                <th className="px-6 py-3 font-medium text-gray-500">Status</th>
                <th className="px-6 py-3 font-medium text-gray-500">OS</th>
                <th className="px-6 py-3 font-medium text-gray-500">Engine</th>
                <th className="px-6 py-3 font-medium text-gray-500">Group</th>
                <th className="px-6 py-3 font-medium text-gray-500">Last Heartbeat</th>
                <th className="px-6 py-3 font-medium text-gray-500">Registered</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {isLoading && (
                <tr>
                  <td colSpan={7} className="px-6 py-12 text-center text-gray-400">
                    Loading...
                  </td>
                </tr>
              )}
              {!isLoading &&
                agents.map((agent) => (
                  <tr
                    key={agent.id}
                    className="cursor-pointer hover:bg-gray-50/50"
                    onClick={() => setSelectedAgent(agent)}
                  >
                    <td className="px-6 py-3">
                      <span className="font-medium text-gray-900">{agent.hostname}</span>
                      <span className="ml-2 text-xs text-gray-400">{agent.id.slice(0, 8)}</span>
                    </td>
                    <td className="px-6 py-3">
                      <StatusBadge status={agent.status} />
                    </td>
                    <td className="px-6 py-3 text-gray-600">{agent.os_version}</td>
                    <td className="px-6 py-3 text-gray-600">{agent.engine_version}</td>
                    <td className="px-6 py-3 text-gray-600">{agent.group_name || agent.group_id}</td>
                    <td className="px-6 py-3 text-gray-500">
                      {agent.last_heartbeat ? timeAgo(new Date(agent.last_heartbeat)) : 'Never'}
                    </td>
                    <td className="px-6 py-3 text-gray-500">
                      {new Date(agent.registered_at).toLocaleDateString()}
                    </td>
                  </tr>
                ))}
              {!isLoading && agents.length === 0 && (
                <tr>
                  <td colSpan={7} className="px-6 py-12 text-center text-gray-400">
                    No agents enrolled. Create an enrollment token in Settings to get started.
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

      {/* Agent detail slide-out panel */}
      <SlidePanel
        open={!!selectedAgent}
        title="Agent Detail"
        onClose={() => setSelectedAgent(null)}
      >
        {selectedAgent && (
          <div className="space-y-6">
            <div>
              <h3 className="text-xl font-bold text-gray-900">{selectedAgent.hostname}</h3>
              <div className="mt-2 flex items-center gap-3">
                <StatusBadge status={selectedAgent.status} />
                <span className="text-sm text-gray-500 font-mono">{selectedAgent.id}</span>
              </div>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="rounded-lg bg-gray-50 px-4 py-3">
                <span className="text-xs text-gray-500">OS Version</span>
                <p className="mt-0.5 text-sm font-medium text-gray-900">{selectedAgent.os_version}</p>
              </div>
              <div className="rounded-lg bg-gray-50 px-4 py-3">
                <span className="text-xs text-gray-500">Engine Version</span>
                <p className="mt-0.5 text-sm font-medium text-gray-900">{selectedAgent.engine_version}</p>
              </div>
              <div className="rounded-lg bg-gray-50 px-4 py-3">
                <span className="text-xs text-gray-500">Group</span>
                <p className="mt-0.5 text-sm font-medium text-gray-900">
                  {selectedAgent.group_name || selectedAgent.group_id || 'None'}
                </p>
              </div>
              <div className="rounded-lg bg-gray-50 px-4 py-3">
                <span className="text-xs text-gray-500">Last Heartbeat</span>
                <p className="mt-0.5 text-sm font-medium text-gray-900">
                  {selectedAgent.last_heartbeat
                    ? timeAgo(new Date(selectedAgent.last_heartbeat))
                    : 'Never'}
                </p>
              </div>
              <div className="rounded-lg bg-gray-50 px-4 py-3">
                <span className="text-xs text-gray-500">Registered</span>
                <p className="mt-0.5 text-sm font-medium text-gray-900">
                  {new Date(selectedAgent.registered_at).toLocaleString()}
                </p>
              </div>
              <div className="rounded-lg bg-gray-50 px-4 py-3">
                <span className="text-xs text-gray-500">Organization</span>
                <p className="mt-0.5 text-sm font-medium text-gray-900 font-mono text-xs">
                  {selectedAgent.org_id}
                </p>
              </div>
            </div>

            {selectedAgent.tags && Object.keys(selectedAgent.tags).length > 0 && (
              <div>
                <h4 className="text-sm font-medium text-gray-500">Tags</h4>
                <div className="mt-2 flex flex-wrap gap-2">
                  {Object.entries(selectedAgent.tags).map(([k, v]) => (
                    <span
                      key={k}
                      className="inline-flex items-center rounded-full bg-gray-100 px-2.5 py-0.5 text-xs font-medium text-gray-700"
                    >
                      {k}: {v}
                    </span>
                  ))}
                </div>
              </div>
            )}

            <div className="border-t border-gray-200 pt-6">
              <button
                onClick={() => setDeleteTarget(selectedAgent)}
                className="rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700"
              >
                Remove Agent
              </button>
            </div>
          </div>
        )}
      </SlidePanel>

      {/* Delete confirmation dialog */}
      <ConfirmDialog
        open={!!deleteTarget}
        title="Remove Agent"
        message={
          'Are you sure you want to remove agent ' +
          (deleteTarget?.hostname || '') +
          '? This action cannot be undone.'
        }
        confirmLabel="Remove"
        onConfirm={() => {
          if (deleteTarget) deleteMutation.mutate(deleteTarget.id)
        }}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  )
}

function timeAgo(date: Date): string {
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
  if (seconds < 60) return seconds + 's ago'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return minutes + 'm ago'
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return hours + 'h ago'
  const days = Math.floor(hours / 24)
  return days + 'd ago'
}
