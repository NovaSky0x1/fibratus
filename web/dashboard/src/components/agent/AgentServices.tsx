import { useState } from 'react'
import { clsx } from 'clsx'
import { useAgentCommand } from '../../hooks/useAgentCommand'
import { useTableSort } from '../../hooks/useTableSort'
import SortableHeader from '../SortableHeader'

interface Service {
  Name: string
  DisplayName: string
  State: string
  StartMode: string
  account: string
  PathName: string
  ProcessId: number
}

interface ServicesResponse {
  services: Service[]
}

function serviceStateBadge(state: string): string {
  const s = state.toLowerCase()
  if (s === 'running') {
    return 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400'
  }
  if (s === 'stopped') {
    return 'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400'
  }
  return 'bg-gray-100 dark:bg-slate-700 text-gray-700 dark:text-slate-300'
}

export default function AgentServices({ agentId }: { agentId: string }) {
  const [search, setSearch] = useState('')

  const { data, isLoading, error, execute, lastUpdated } = useAgentCommand<ServicesResponse>(
    agentId,
    'get_services',
  )

  const services = data?.services || []
  const filtered = search
    ? services.filter(
        (s) =>
          s.Name.toLowerCase().includes(search.toLowerCase()) ||
          s.DisplayName.toLowerCase().includes(search.toLowerCase()) ||
          (s.account || '').toLowerCase().includes(search.toLowerCase()),
      )
    : services

  const { sorted, sort, toggleSort } = useTableSort(filtered, 'Name', 'asc')

  return (
    <div className="flex flex-col gap-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold text-gray-900 dark:text-slate-100">Services</h3>
        <div className="flex items-center gap-3">
          {lastUpdated && (
            <span className="text-xs text-gray-400 dark:text-slate-500">
              Updated {lastUpdated.toLocaleTimeString()}
            </span>
          )}
          <button
            onClick={execute}
            disabled={isLoading}
            className="rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-800 p-1.5 text-gray-500 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700 disabled:opacity-50"
            title="Refresh"
          >
            <svg className={clsx('h-4 w-4', isLoading && 'animate-spin')} fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
          </button>
        </div>
      </div>

      {/* Search */}
      <input
        type="text"
        placeholder="Filter by name, display name, or account..."
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-800 px-4 py-2 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
      />

      {/* Error */}
      {error && (
        <div className="rounded-lg border border-red-300 dark:border-red-700 bg-red-50 dark:bg-red-900/20 p-4">
          <p className="text-sm text-red-700 dark:text-red-400">{error}</p>
          <button
            onClick={execute}
            className="mt-2 rounded-lg bg-red-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-red-700"
          >
            Retry
          </button>
        </div>
      )}

      {/* Loading skeleton */}
      {isLoading && !data && (
        <div className="space-y-2">
          {Array.from({ length: 8 }).map((_, i) => (
            <div key={i} className="h-10 rounded bg-gray-200 dark:bg-slate-700 animate-pulse" />
          ))}
        </div>
      )}

      {/* Table */}
      {data && (
        <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50 overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 dark:border-slate-700 bg-gray-50/50 dark:bg-slate-800/50">
              <tr>
                <SortableHeader label="Name" sortKey="Name" sort={sort} onSort={toggleSort} />
                <SortableHeader label="Display Name" sortKey="DisplayName" sort={sort} onSort={toggleSort} />
                <SortableHeader label="Status" sortKey="State" sort={sort} onSort={toggleSort} />
                <SortableHeader label="Start Type" sortKey="StartMode" sort={sort} onSort={toggleSort} />
                <SortableHeader label="Account" sortKey="account" sort={sort} onSort={toggleSort} />
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Path</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {sorted.map((svc) => (
                <tr key={svc.Name} className="even:bg-gray-50 dark:even:bg-slate-800/50">
                  <td className="px-6 py-2 font-mono text-xs font-medium text-gray-900 dark:text-slate-100">
                    {svc.Name}
                  </td>
                  <td className="px-6 py-2 text-gray-700 dark:text-slate-300 text-xs">{svc.DisplayName}</td>
                  <td className="px-6 py-2">
                    <span
                      className={clsx(
                        'inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium',
                        serviceStateBadge(svc.State),
                      )}
                    >
                      {svc.State}
                    </span>
                  </td>
                  <td className="px-6 py-2 text-gray-600 dark:text-slate-400 text-xs">{svc.StartMode}</td>
                  <td className="px-6 py-2 text-gray-600 dark:text-slate-400 text-xs">{svc.account || '-'}</td>
                  <td className="px-6 py-2 font-mono text-xs text-gray-500 dark:text-slate-400 max-w-sm truncate" title={svc.PathName}>
                    {svc.PathName || '-'}
                  </td>
                </tr>
              ))}
              {sorted.length === 0 && (
                <tr>
                  <td colSpan={6} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">
                    {search ? 'No services match the filter' : 'No services returned'}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
          <div className="border-t border-gray-100 dark:border-slate-700 px-6 py-2">
            <span className="text-xs text-gray-400 dark:text-slate-500">
              {sorted.length} of {services.length} services
            </span>
          </div>
        </div>
      )}
    </div>
  )
}
