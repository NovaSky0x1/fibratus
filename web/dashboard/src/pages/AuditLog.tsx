import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type AuditEntry } from '../lib/api'

const actionColors: Record<string, string> = {
  create: 'bg-emerald-50 text-emerald-700',
  update: 'bg-blue-50 text-blue-700',
  delete: 'bg-red-50 text-red-700',
  execute: 'bg-amber-50 text-amber-700',
  login: 'bg-purple-50 text-purple-700',
  toggle: 'bg-indigo-50 text-indigo-700',
}

const resourceIcons: Record<string, string> = {
  rule: 'R',
  macro: 'M',
  command: 'C',
  agent: 'A',
  enrollment_token: 'T',
  user: 'U',
}

export default function AuditLog() {
  const [page, setPage] = useState(1)

  const { data, isLoading } = useQuery({
    queryKey: ['audit-log', page],
    queryFn: () => api.getAuditLog({ page: String(page), per_page: '50' }),
  })

  const entries = (data?.data || []) as AuditEntry[]
  const total = data?.meta?.total ?? 0
  const perPage = data?.meta?.per_page ?? 50
  const totalPages = Math.ceil(total / perPage)

  return (
    <div>
      <div>
        <h1 className="text-2xl font-bold text-gray-900">Audit Log</h1>
        <p className="mt-1 text-sm text-gray-500">{total} action(s) recorded</p>
      </div>

      <div className="mt-6 rounded-xl border border-gray-200 bg-white shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 bg-gray-50/50">
              <tr>
                <th className="px-6 py-3 font-medium text-gray-500">Timestamp</th>
                <th className="px-6 py-3 font-medium text-gray-500">User</th>
                <th className="px-6 py-3 font-medium text-gray-500">Action</th>
                <th className="px-6 py-3 font-medium text-gray-500">Resource</th>
                <th className="px-6 py-3 font-medium text-gray-500">Details</th>
                <th className="px-6 py-3 font-medium text-gray-500">IP</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {isLoading && (
                <tr><td colSpan={6} className="px-6 py-12 text-center text-gray-400">Loading...</td></tr>
              )}
              {!isLoading && entries.map((entry) => (
                <tr key={entry.id} className="hover:bg-gray-50/50">
                  <td className="px-6 py-3 text-gray-500 tabular-nums text-xs whitespace-nowrap">
                    {new Date(entry.timestamp).toLocaleString()}
                  </td>
                  <td className="px-6 py-3">
                    <span className="text-sm text-gray-900">{entry.user_email || entry.user_id?.slice(0, 8) || 'system'}</span>
                  </td>
                  <td className="px-6 py-3">
                    <span className={'inline-flex rounded-full px-2 py-0.5 text-xs font-medium ' +
                      (actionColors[entry.action] || 'bg-gray-50 text-gray-700')}>
                      {entry.action}
                    </span>
                  </td>
                  <td className="px-6 py-3">
                    <div className="flex items-center gap-2">
                      <span className="flex h-5 w-5 items-center justify-center rounded bg-gray-100 text-[10px] font-bold text-gray-500">
                        {resourceIcons[entry.resource_type] || '?'}
                      </span>
                      <span className="text-sm text-gray-700">
                        {entry.resource_name || entry.resource_type}
                      </span>
                      {entry.resource_id && (
                        <span className="text-xs text-gray-400 font-mono">{entry.resource_id.slice(0, 8)}</span>
                      )}
                    </div>
                  </td>
                  <td className="px-6 py-3 text-xs text-gray-500 break-all">
                    {entry.details && typeof entry.details === 'object' && Object.keys(entry.details as Record<string, unknown>).length > 0
                      ? Object.entries(entry.details as Record<string, unknown>).map(([k, v]) => `${k}: ${v}`).join(', ')
                      : '-'}
                  </td>
                  <td className="px-6 py-3 text-xs text-gray-400 font-mono">{entry.ip_address?.split(':')[0] || '-'}</td>
                </tr>
              ))}
              {!isLoading && entries.length === 0 && (
                <tr><td colSpan={6} className="px-6 py-12 text-center text-gray-400">
                  No audit log entries yet. Actions will be recorded as users interact with the portal.
                </td></tr>
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
    </div>
  )
}
