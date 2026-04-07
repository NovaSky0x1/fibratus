import { useState } from 'react'
import { clsx } from 'clsx'
import { useAgentCommand } from '../../hooks/useAgentCommand'

interface RunKey {
  name: string
  value: string
  location: string
}

interface ScheduledTask {
  TaskName: string
  TaskPath: string
  State: string
  action: string
}

interface StartupItem {
  name: string
  path: string
  location: string
}

interface AutorunsResponse {
  run_keys: RunKey[]
  scheduled_tasks: ScheduledTask[]
  startup_folder: StartupItem[]
}

function CollapsibleSection({
  title,
  count,
  defaultOpen,
  children,
}: {
  title: string
  count: number
  defaultOpen?: boolean
  children: React.ReactNode
}) {
  const [open, setOpen] = useState(defaultOpen ?? true)

  return (
    <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
      <button
        onClick={() => setOpen(!open)}
        className="flex w-full items-center justify-between px-6 py-3 text-left"
      >
        <div className="flex items-center gap-3">
          <svg
            className={clsx('h-4 w-4 text-gray-400 dark:text-slate-500 transition-transform', open && 'rotate-90')}
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
          </svg>
          <span className="text-sm font-semibold text-gray-900 dark:text-slate-100">{title}</span>
          <span className="rounded-full bg-gray-100 dark:bg-slate-700 px-2 py-0.5 text-xs font-medium text-gray-600 dark:text-slate-400">
            {count}
          </span>
        </div>
      </button>
      {open && <div className="border-t border-gray-100 dark:border-slate-700">{children}</div>}
    </div>
  )
}

export default function AgentAutoruns({ agentId }: { agentId: string }) {
  const { data, isLoading, error, execute, lastUpdated } = useAgentCommand<AutorunsResponse>(
    agentId,
    'get_autoruns',
  )

  const runKeys = data?.run_keys || []
  const tasks = data?.scheduled_tasks || []
  const startupItems = data?.startup_folder || []

  return (
    <div className="flex flex-col gap-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold text-gray-900 dark:text-slate-100">Autoruns</h3>
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
        <div className="space-y-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="h-32 rounded-xl bg-gray-200 dark:bg-slate-700 animate-pulse" />
          ))}
        </div>
      )}

      {data && (
        <div className="space-y-4">
          {/* Registry Run Keys */}
          <CollapsibleSection title="Registry Run Keys" count={runKeys.length} defaultOpen>
            {runKeys.length === 0 ? (
              <p className="px-6 py-4 text-sm text-gray-400 dark:text-slate-500">No run keys found</p>
            ) : (
              <table className="w-full text-left text-sm">
                <thead className="bg-gray-50/50 dark:bg-slate-800/50">
                  <tr>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Name</th>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Value</th>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Location</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
                  {runKeys.map((rk, idx) => (
                    <tr key={idx} className="even:bg-gray-50 dark:even:bg-slate-800/50">
                      <td className="px-6 py-2 font-medium text-gray-900 dark:text-slate-100 text-xs">{rk.name}</td>
                      <td className="px-6 py-2 font-mono text-xs text-gray-600 dark:text-slate-400 max-w-md truncate" title={rk.value}>
                        {rk.value}
                      </td>
                      <td className="px-6 py-2 font-mono text-xs text-gray-500 dark:text-slate-400 max-w-xs truncate" title={rk.location}>
                        {rk.location}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </CollapsibleSection>

          {/* Scheduled Tasks */}
          <CollapsibleSection title="Scheduled Tasks" count={tasks.length}>
            {tasks.length === 0 ? (
              <p className="px-6 py-4 text-sm text-gray-400 dark:text-slate-500">No scheduled tasks found</p>
            ) : (
              <table className="w-full text-left text-sm">
                <thead className="bg-gray-50/50 dark:bg-slate-800/50">
                  <tr>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Task Name</th>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Task Path</th>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">State</th>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Action</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
                  {tasks.map((t, idx) => (
                    <tr key={idx} className="even:bg-gray-50 dark:even:bg-slate-800/50">
                      <td className="px-6 py-2 font-medium text-gray-900 dark:text-slate-100 text-xs">{t.TaskName}</td>
                      <td className="px-6 py-2 font-mono text-xs text-gray-600 dark:text-slate-400">{t.TaskPath}</td>
                      <td className="px-6 py-2">
                        <span
                          className={clsx(
                            'inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium',
                            t.State.toLowerCase() === 'ready'
                              ? 'bg-emerald-100 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-400'
                              : t.State.toLowerCase() === 'disabled'
                                ? 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400'
                                : t.State.toLowerCase() === 'running'
                                  ? 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400'
                                  : 'bg-gray-100 dark:bg-slate-700 text-gray-700 dark:text-slate-300',
                          )}
                        >
                          {t.State}
                        </span>
                      </td>
                      <td className="px-6 py-2 font-mono text-xs text-gray-500 dark:text-slate-400 max-w-sm truncate" title={t.action}>
                        {t.action || '-'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </CollapsibleSection>

          {/* Startup Folder */}
          <CollapsibleSection title="Startup Folder" count={startupItems.length}>
            {startupItems.length === 0 ? (
              <p className="px-6 py-4 text-sm text-gray-400 dark:text-slate-500">No startup folder items found</p>
            ) : (
              <table className="w-full text-left text-sm">
                <thead className="bg-gray-50/50 dark:bg-slate-800/50">
                  <tr>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Name</th>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Path</th>
                    <th className="px-6 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Location</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
                  {startupItems.map((item, idx) => (
                    <tr key={idx} className="even:bg-gray-50 dark:even:bg-slate-800/50">
                      <td className="px-6 py-2 font-medium text-gray-900 dark:text-slate-100 text-xs">{item.name}</td>
                      <td className="px-6 py-2 font-mono text-xs text-gray-600 dark:text-slate-400 max-w-md truncate" title={item.path}>
                        {item.path}
                      </td>
                      <td className="px-6 py-2 text-xs text-gray-500 dark:text-slate-400">{item.location}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </CollapsibleSection>
        </div>
      )}
    </div>
  )
}
