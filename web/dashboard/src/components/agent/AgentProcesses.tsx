import { useState, useCallback, useRef, useEffect } from 'react'
import { clsx } from 'clsx'
import { useAgentCommand } from '../../hooks/useAgentCommand'
import { useTableSort } from '../../hooks/useTableSort'
import SortableHeader from '../SortableHeader'
import { api } from '../../lib/api'

interface Process {
  ProcessId: number
  Name: string
  CommandLine: string
  ExecutablePath: string
  mem_mb: number
  username: string
  // Legacy field names for backward compat
  Id?: number
  ProcessName?: string
  Path?: string
  cmdline?: string
  cpu_pct?: number
}

interface ProcessesResponse {
  processes: Process[]
}

export default function AgentProcesses({ agentId }: { agentId: string }) {
  const [search, setSearch] = useState('')
  const [autoRefresh, setAutoRefresh] = useState(false)
  const [killingPid, setKillingPid] = useState<number | null>(null)
  const [killedPids, setKilledPids] = useState<Set<number>>(new Set())
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const { data, isLoading, error, execute, lastUpdated } = useAgentCommand<ProcessesResponse>(
    agentId,
    'get_processes',
  )

  useEffect(() => {
    if (autoRefresh) {
      intervalRef.current = setInterval(() => execute(), 15000)
    }
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current)
    }
  }, [autoRefresh, execute])

  const processes = data?.processes || []
  const filtered = search
    ? processes.filter(
        (p) =>
          (p.Name || p.ProcessName || '').toLowerCase().includes(search.toLowerCase()) ||
          (p.username || '').toLowerCase().includes(search.toLowerCase()) ||
          String(p.ProcessId || p.Id).includes(search),
      )
    : processes

  const { sorted, sort, toggleSort } = useTableSort(filtered, 'mem_mb', 'desc')

  const handleKill = useCallback(
    async (pid: number) => {
      setKillingPid(pid)
      try {
        await api.createCommand(agentId, 'kill_process', { pid })
        setKilledPids(prev => new Set(prev).add(pid))
      } finally {
        setKillingPid(null)
      }
    },
    [agentId],
  )

  return (
    <div className="flex flex-col gap-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold text-gray-900 dark:text-slate-100">Processes</h3>
        <div className="flex items-center gap-3">
          {lastUpdated && (
            <span className="text-xs text-gray-400 dark:text-slate-500">
              Updated {lastUpdated.toLocaleTimeString()}
            </span>
          )}
          <button
            onClick={() => setAutoRefresh(!autoRefresh)}
            className={clsx(
              'rounded-lg px-3 py-1.5 text-xs font-medium border',
              autoRefresh
                ? 'border-emerald-300 dark:border-emerald-700 bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400'
                : 'border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-800 text-gray-600 dark:text-slate-400',
            )}
          >
            {autoRefresh ? 'Auto-refresh ON' : 'Auto-refresh OFF'}
          </button>
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
        placeholder="Filter by name, user, or PID..."
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
                <SortableHeader label="PID" sortKey="ProcessId" sort={sort} onSort={toggleSort} />
                <SortableHeader label="Name" sortKey="Name" sort={sort} onSort={toggleSort} />
                <SortableHeader label="Memory (MB)" sortKey="mem_mb" sort={sort} onSort={toggleSort} />
                <SortableHeader label="User" sortKey="username" sort={sort} onSort={toggleSort} />
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Path</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Command Line</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400 w-20">Action</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {sorted.map((proc) => {
                const pid = proc.ProcessId || proc.Id || 0
                const killed = killedPids.has(pid)
                return (
                <tr key={pid} className={`even:bg-gray-50 dark:even:bg-slate-800/50 hover:bg-gray-100/50 dark:hover:bg-slate-700/30 ${killed ? 'opacity-50' : ''}`}>
                  <td className="px-6 py-2 font-mono text-gray-700 dark:text-slate-300 tabular-nums">{pid}</td>
                  <td className="px-6 py-2 font-medium text-gray-900 dark:text-slate-100">
                    <span className={killed ? 'line-through' : ''}>{proc.Name || proc.ProcessName}</span>
                    {killed && <span className="ml-2 inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-semibold bg-red-100 dark:bg-red-900/40 text-red-700 dark:text-red-400">KILLED</span>}
                  </td>
                  <td className="px-6 py-2 font-mono text-gray-700 dark:text-slate-300 tabular-nums">
                    {(proc.mem_mb || 0).toFixed(1)}
                  </td>
                  <td className="px-6 py-2 text-gray-600 dark:text-slate-400 text-xs">{proc.username || '-'}</td>
                  <td className="px-6 py-2 font-mono text-xs text-gray-500 dark:text-slate-400 max-w-xs truncate" title={proc.ExecutablePath || proc.Path || ''}>
                    {proc.ExecutablePath || proc.Path || '-'}
                  </td>
                  <td className="px-6 py-2 font-mono text-xs text-gray-500 dark:text-slate-400 max-w-xs truncate" title={proc.CommandLine || proc.cmdline || ''}>
                    {proc.CommandLine || proc.cmdline || '-'}
                  </td>
                  <td className="px-6 py-2">
                    {killed ? (
                      <span className="text-[10px] text-red-500 dark:text-red-400">Terminated</span>
                    ) : (
                    <button
                      onClick={() => handleKill(pid)}
                      disabled={killingPid === pid}
                      className="rounded border border-red-300 dark:border-red-700 bg-red-50 dark:bg-red-900/20 px-2 py-1 text-xs font-medium text-red-700 dark:text-red-400 hover:bg-red-100 dark:hover:bg-red-900/40 disabled:opacity-50"
                    >
                      {killingPid === pid ? '...' : 'Kill'}
                    </button>
                    )}
                  </td>
                </tr>
                )
              })}
              {sorted.length === 0 && (
                <tr>
                  <td colSpan={8} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">
                    {search ? 'No processes match the filter' : 'No processes returned'}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
          <div className="border-t border-gray-100 dark:border-slate-700 px-6 py-2">
            <span className="text-xs text-gray-400 dark:text-slate-500">
              {sorted.length} of {processes.length} processes
            </span>
          </div>
        </div>
      )}
    </div>
  )
}
