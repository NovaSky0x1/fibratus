import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type Agent } from '../lib/api'
import StatusBadge from '../components/StatusBadge'

export default function Agents() {
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [page, setPage] = useState(1)

  const { data, isLoading } = useQuery({
    queryKey: ['agents', page, statusFilter, search],
    queryFn: () => api.getAgents({ page, status: statusFilter, search }),
  })

  const agents = (data?.data || []) as Agent[]
  const total = data?.meta?.total ?? 0
  const perPage = data?.meta?.per_page ?? 50

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
          onChange={(e) => { setSearch(e.target.value); setPage(1) }}
          className="rounded-lg border border-gray-300 px-4 py-2 text-sm focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
        />
        <select
          value={statusFilter}
          onChange={(e) => { setStatusFilter(e.target.value); setPage(1) }}
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
              {!isLoading && agents.map((agent) => (
                <tr key={agent.id} className="hover:bg-gray-50/50">
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
                    No agents found. Deploy Fibratus agents with fleet mode enabled.
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
              Page {page} of {Math.ceil(total / perPage)}
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
                disabled={page >= Math.ceil(total / perPage)}
                className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm disabled:opacity-50"
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

function timeAgo(date: Date): string {
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}
