import { useState } from 'react'
import { clsx } from 'clsx'
import { useAgentCommand } from '../../hooks/useAgentCommand'
import { useTableSort } from '../../hooks/useTableSort'
import SortableHeader from '../SortableHeader'

interface Software {
  name: string
  version: string
  publisher: string
  install_date: string
  size_mb: number
}

interface SoftwareResponse {
  software: Software[]
}

export default function AgentSoftware({ agentId }: { agentId: string }) {
  const [search, setSearch] = useState('')

  const { data, isLoading, error, execute, lastUpdated } = useAgentCommand<SoftwareResponse>(
    agentId,
    'get_software',
  )

  const software = data?.software || []
  const filtered = search
    ? software.filter(
        (s) =>
          s.name.toLowerCase().includes(search.toLowerCase()) ||
          (s.publisher || '').toLowerCase().includes(search.toLowerCase()) ||
          (s.version || '').toLowerCase().includes(search.toLowerCase()),
      )
    : software

  const { sorted, sort, toggleSort } = useTableSort(filtered, 'name', 'asc')

  return (
    <div className="flex flex-col gap-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold text-gray-900 dark:text-slate-100">Installed Software</h3>
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
        placeholder="Filter by name, publisher, or version..."
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
                <SortableHeader label="Name" sortKey="name" sort={sort} onSort={toggleSort} />
                <SortableHeader label="Version" sortKey="version" sort={sort} onSort={toggleSort} />
                <SortableHeader label="Publisher" sortKey="publisher" sort={sort} onSort={toggleSort} />
                <SortableHeader label="Install Date" sortKey="install_date" sort={sort} onSort={toggleSort} />
                <SortableHeader label="Size (MB)" sortKey="size_mb" sort={sort} onSort={toggleSort} />
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {sorted.map((sw, idx) => (
                <tr key={idx} className="even:bg-gray-50 dark:even:bg-slate-800/50">
                  <td className="px-6 py-2 font-medium text-gray-900 dark:text-slate-100 text-sm">{sw.name}</td>
                  <td className="px-6 py-2 font-mono text-xs text-gray-600 dark:text-slate-400">{sw.version || '-'}</td>
                  <td className="px-6 py-2 text-gray-600 dark:text-slate-400 text-xs">{sw.publisher || '-'}</td>
                  <td className="px-6 py-2 text-gray-600 dark:text-slate-400 text-xs tabular-nums">{sw.install_date || '-'}</td>
                  <td className="px-6 py-2 font-mono text-xs text-gray-600 dark:text-slate-400 tabular-nums">
                    {sw.size_mb > 0 ? sw.size_mb.toFixed(1) : '-'}
                  </td>
                </tr>
              ))}
              {sorted.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">
                    {search ? 'No software matches the filter' : 'No software returned'}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
          <div className="border-t border-gray-100 dark:border-slate-700 px-6 py-2">
            <span className="text-xs text-gray-400 dark:text-slate-500">
              {sorted.length} of {software.length} packages
            </span>
          </div>
        </div>
      )}
    </div>
  )
}
