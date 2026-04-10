import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type AuditEntry } from '../../lib/api'
import { useTableSort } from '../../hooks/useTableSort'
import SortableHeader from '../SortableHeader'

const actionColors: Record<string, string> = {
  create: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400',
  update: 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
  delete: 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-400',
  execute: 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400',
  login: 'bg-purple-50 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400',
  toggle: 'bg-indigo-50 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-400',
}

const resourceIcons: Record<string, string> = {
  rule: 'R',
  macro: 'M',
  command: 'C',
  agent: 'A',
  enrollment_token: 'T',
  user: 'U',
}

export default function AuditTab() {
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['audit-log', page],
    queryFn: () => api.getAuditLog({ page: String(page), per_page: '50' }),
  })

  const allEntries = (data?.data || []) as AuditEntry[]
  const entries = search
    ? allEntries.filter(e =>
        (e.user_email || '').toLowerCase().includes(search.toLowerCase()) ||
        e.action.toLowerCase().includes(search.toLowerCase()) ||
        e.resource_type.toLowerCase().includes(search.toLowerCase()) ||
        (e.resource_name || '').toLowerCase().includes(search.toLowerCase()))
    : allEntries
  const { sorted: sortedEntries, sort, toggleSort } = useTableSort(entries, 'timestamp', 'desc')
  const total = data?.meta?.total ?? 0
  const perPage = data?.meta?.per_page ?? 50
  const totalPages = Math.ceil(total / perPage)

  return (
    <div className="mt-6">
      <div className="flex items-center justify-between mb-4">
        <p className="text-sm text-gray-500 dark:text-slate-400">{total} action(s) recorded</p>
        <input
          type="text"
          placeholder="Search by user, action, or resource..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="w-80 rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-800 px-4 py-2 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
        />
      </div>

      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 dark:border-slate-700 bg-gray-50/50 dark:bg-slate-800/50">
              <tr>
                <SortableHeader label="Timestamp" sortKey="timestamp" sort={sort} onSort={toggleSort} />
                <SortableHeader label="User" sortKey="user_email" sort={sort} onSort={toggleSort} />
                <SortableHeader label="Action" sortKey="action" sort={sort} onSort={toggleSort} />
                <SortableHeader label="Resource" sortKey="resource_type" sort={sort} onSort={toggleSort} />
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Details</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">IP</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {isLoading && (
                <tr><td colSpan={6} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">Loading...</td></tr>
              )}
              {!isLoading && sortedEntries.map((entry) => (
                <tr key={entry.id} className="hover:bg-gray-50/50 dark:hover:bg-slate-700/50">
                  <td className="px-6 py-3 text-gray-500 dark:text-slate-400 tabular-nums text-xs whitespace-nowrap">
                    {new Date(entry.timestamp).toLocaleString()}
                  </td>
                  <td className="px-6 py-3">
                    <span className="text-sm text-gray-900 dark:text-slate-100">{entry.user_email || entry.user_id?.slice(0, 8) || 'system'}</span>
                  </td>
                  <td className="px-6 py-3">
                    <span className={'inline-flex rounded-full px-2 py-0.5 text-xs font-medium ' +
                      (actionColors[entry.action] || 'bg-gray-50 text-gray-700')}>
                      {entry.action}
                    </span>
                  </td>
                  <td className="px-6 py-3">
                    <div className="flex items-center gap-2">
                      <span className="flex h-5 w-5 items-center justify-center rounded bg-gray-100 dark:bg-slate-700 text-[10px] font-bold text-gray-500 dark:text-slate-400">
                        {resourceIcons[entry.resource_type] || '?'}
                      </span>
                      <span className="text-sm text-gray-700 dark:text-slate-300">
                        {entry.resource_name || entry.resource_type}
                      </span>
                      {entry.resource_id && (
                        <span className="text-xs text-gray-400 dark:text-slate-500 font-mono">{entry.resource_id.slice(0, 8)}</span>
                      )}
                    </div>
                  </td>
                  <td className="px-6 py-3 text-xs text-gray-500 dark:text-slate-400 break-all">
                    {entry.details && typeof entry.details === 'object' && Object.keys(entry.details as Record<string, unknown>).length > 0
                      ? Object.entries(entry.details as Record<string, unknown>).map(([k, v]) => `${k}: ${v}`).join(', ')
                      : '-'}
                  </td>
                  <td className="px-6 py-3 text-xs text-gray-400 dark:text-slate-500 font-mono">{entry.ip_address?.split(':')[0] || '-'}</td>
                </tr>
              ))}
              {!isLoading && sortedEntries.length === 0 && (
                <tr><td colSpan={6} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">
                  {search ? 'No audit log entries match your search.' : 'No audit log entries yet.'}
                </td></tr>
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
    </div>
  )
}
