import { useState } from 'react'
import { clsx } from 'clsx'
import { useAgentCommand } from '../../hooks/useAgentCommand'
import { useTableSort } from '../../hooks/useTableSort'
import SortableHeader from '../SortableHeader'

interface Connection {
  LocalAddress: string
  LocalPort: number
  RemoteAddress: string
  RemotePort: number
  State: string
  OwningProcess: number
  process_name: string
}

interface NetworkResponse {
  connections: Connection[]
}

type StateFilter = 'all' | 'Established' | 'Listen' | 'TimeWait' | 'CloseWait'

const STATE_FILTERS: StateFilter[] = ['all', 'Established', 'Listen', 'TimeWait', 'CloseWait']

function stateBadgeClasses(state: string): string {
  const s = state.toLowerCase()
  if (s === 'established' || s === 'estab') {
    return 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400'
  }
  if (s === 'listen' || s === 'listening') {
    return 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400'
  }
  if (s === 'timewait' || s === 'time_wait' || s === 'time-wait') {
    return 'bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400'
  }
  if (s === 'closewait' || s === 'close_wait' || s === 'close-wait') {
    return 'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400'
  }
  return 'bg-gray-100 dark:bg-slate-700 text-gray-700 dark:text-slate-300'
}

function matchesFilter(state: string, filter: StateFilter): boolean {
  if (filter === 'all') return true
  const s = state.toLowerCase().replace(/[_-]/g, '')
  return s === filter.toLowerCase()
}

export default function AgentNetwork({ agentId }: { agentId: string }) {
  const [stateFilter, setStateFilter] = useState<StateFilter>('all')

  const { data, isLoading, error, execute, lastUpdated } = useAgentCommand<NetworkResponse>(
    agentId,
    'get_network',
  )

  const raw = data?.connections
  const connections: Connection[] = Array.isArray(raw) ? raw : raw ? [raw as unknown as Connection] : []
  const filtered = connections.filter((c) => matchesFilter(c.State, stateFilter))

  const { sorted, sort, toggleSort } = useTableSort(filtered, 'State', 'asc')

  // Count connections by state for filter badges
  const counts: Record<string, number> = { all: connections.length }
  for (const c of connections) {
    const s = c.State.toLowerCase().replace(/[_-]/g, '')
    for (const f of STATE_FILTERS) {
      if (f !== 'all' && s === f.toLowerCase()) {
        counts[f] = (counts[f] || 0) + 1
      }
    }
  }

  return (
    <div className="flex flex-col gap-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold text-gray-900 dark:text-slate-100">Network Connections</h3>
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

      {/* State filter tabs */}
      <div className="flex gap-1 border-b border-gray-200 dark:border-slate-700">
        {STATE_FILTERS.map((f) => (
          <button
            key={f}
            onClick={() => setStateFilter(f)}
            className={clsx(
              'px-4 py-2 text-sm font-medium border-b-2 -mb-px capitalize',
              stateFilter === f
                ? 'border-fibratus-600 text-fibratus-600'
                : 'border-transparent text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-300',
            )}
          >
            {f}
            {counts[f] != null && (
              <span className="ml-1.5 text-xs text-gray-400 dark:text-slate-500">({counts[f] || 0})</span>
            )}
          </button>
        ))}
      </div>

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
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Local Address:Port</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Remote Address:Port</th>
                <SortableHeader label="State" sortKey="State" sort={sort} onSort={toggleSort} />
                <SortableHeader label="PID" sortKey="OwningProcess" sort={sort} onSort={toggleSort} />
                <SortableHeader label="Process" sortKey="process_name" sort={sort} onSort={toggleSort} />
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {sorted.map((conn, idx) => (
                <tr key={idx} className="even:bg-gray-50 dark:even:bg-slate-800/50">
                  <td className="px-6 py-2 font-mono text-xs text-gray-700 dark:text-slate-300">
                    {conn.LocalAddress}:{conn.LocalPort}
                  </td>
                  <td className="px-6 py-2 font-mono text-xs text-gray-700 dark:text-slate-300">
                    {conn.RemoteAddress}:{conn.RemotePort}
                  </td>
                  <td className="px-6 py-2">
                    <span
                      className={clsx(
                        'inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium',
                        stateBadgeClasses(conn.State),
                      )}
                    >
                      {conn.State}
                    </span>
                  </td>
                  <td className="px-6 py-2 font-mono text-xs text-gray-700 dark:text-slate-300 tabular-nums">
                    {conn.OwningProcess}
                  </td>
                  <td className="px-6 py-2 text-gray-700 dark:text-slate-300">{conn.process_name || '-'}</td>
                </tr>
              ))}
              {sorted.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">
                    {stateFilter !== 'all' ? `No ${stateFilter} connections` : 'No connections returned'}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
          <div className="border-t border-gray-100 dark:border-slate-700 px-6 py-2">
            <span className="text-xs text-gray-400 dark:text-slate-500">
              {sorted.length} of {connections.length} connections
            </span>
          </div>
        </div>
      )}
    </div>
  )
}
