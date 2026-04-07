import { useState, useCallback, useMemo, useEffect, useRef } from 'react'
import { clsx } from 'clsx'
import { useAgentCommand } from '../../hooks/useAgentCommand'

interface RegistryKey {
  PSChildName: string
  subkey_count: number
}

interface RegistryValue {
  name: string
  value: string
  type: string
}

interface RegistryResponse {
  path: string
  keys: RegistryKey[]
  values: RegistryValue[]
}

const INITIAL_PATH = 'HKLM:\\SOFTWARE'

export default function AgentRegistry({ agentId }: { agentId: string }) {
  const [currentPath, setCurrentPath] = useState(INITIAL_PATH)
  const isInitialMount = useRef(true)

  const payload = useMemo(() => ({ path: currentPath }), [currentPath])

  const { data, isLoading, error, execute, lastUpdated } = useAgentCommand<RegistryResponse>(
    agentId,
    'get_registry',
    payload,
  )

  const keys = data?.keys || []
  const values = data?.values || []

  // Re-execute when path changes after initial mount (hook handles first execution)
  useEffect(() => {
    if (isInitialMount.current) {
      isInitialMount.current = false
      return
    }
    execute()
  }, [currentPath]) // eslint-disable-line react-hooks/exhaustive-deps

  const navigateToSubkey = useCallback(
    (subkey: string) => {
      const sep = currentPath.endsWith('\\') ? '' : '\\'
      setCurrentPath(`${currentPath}${sep}${subkey}`)
    },
    [currentPath],
  )

  const navigateUp = useCallback(() => {
    const parts = currentPath.split('\\')
    if (parts.length > 1) {
      parts.pop()
      setCurrentPath(parts.join('\\'))
    }
  }, [currentPath])

  const breadcrumbs = useMemo(() => {
    const parts = currentPath.split('\\')
    const segments: { label: string; path: string }[] = []
    for (let i = 0; i < parts.length; i++) {
      segments.push({
        label: parts[i].replace(':', ''),
        path: parts.slice(0, i + 1).join('\\'),
      })
    }
    return segments
  }, [currentPath])

  return (
    <div className="flex flex-col gap-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold text-gray-900 dark:text-slate-100">Registry Browser</h3>
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

      {/* Breadcrumb */}
      <div className="flex items-center gap-1 rounded-lg bg-gray-50 dark:bg-slate-900 px-4 py-2 overflow-x-auto">
        {breadcrumbs.map((bc, idx) => (
          <span key={bc.path} className="flex items-center gap-1 whitespace-nowrap">
            {idx > 0 && <span className="text-gray-400 dark:text-slate-600 mx-0.5">\</span>}
            <button
              onClick={() => setCurrentPath(bc.path)}
              className={clsx(
                'text-xs font-mono hover:underline',
                idx === breadcrumbs.length - 1
                  ? 'text-fibratus-600 font-semibold'
                  : 'text-gray-500 dark:text-slate-400',
              )}
            >
              {bc.label}
            </button>
          </span>
        ))}
        {currentPath !== INITIAL_PATH && (
          <button
            onClick={navigateUp}
            className="ml-auto text-xs text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-200 flex items-center gap-1"
          >
            <svg className="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
            </svg>
            Up
          </button>
        )}
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
        <div className="grid grid-cols-3 gap-4">
          <div className="col-span-1 h-64 rounded-xl bg-gray-200 dark:bg-slate-700 animate-pulse" />
          <div className="col-span-2 h-64 rounded-xl bg-gray-200 dark:bg-slate-700 animate-pulse" />
        </div>
      )}

      {/* Loading overlay for navigation */}
      {isLoading && data && (
        <div className="text-xs text-gray-400 dark:text-slate-500 flex items-center gap-2">
          <div className="h-3 w-3 rounded-full border-2 border-fibratus-500 border-t-transparent animate-spin" />
          Loading...
        </div>
      )}

      {/* Content: subkeys panel + values panel */}
      {data && (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
          {/* Left: Subkeys */}
          <div className="lg:col-span-1 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50 overflow-hidden">
            <div className="px-4 py-3 border-b border-gray-100 dark:border-slate-700 bg-gray-50/50 dark:bg-slate-800/50">
              <h4 className="text-xs font-semibold text-gray-500 dark:text-slate-400 uppercase tracking-wider">
                Subkeys
                <span className="ml-2 text-gray-400 dark:text-slate-500 normal-case">({keys.length})</span>
              </h4>
            </div>
            <div className="max-h-96 overflow-y-auto divide-y divide-gray-100 dark:divide-slate-700">
              {keys.length === 0 ? (
                <p className="px-4 py-6 text-center text-sm text-gray-400 dark:text-slate-500">No subkeys</p>
              ) : (
                keys.map((k) => (
                  <button
                    key={k.PSChildName}
                    onClick={() => navigateToSubkey(k.PSChildName)}
                    className="flex w-full items-center justify-between px-4 py-2 text-left hover:bg-gray-50 dark:hover:bg-slate-700/50 transition-colors"
                  >
                    <div className="flex items-center gap-2 min-w-0">
                      <svg className="h-4 w-4 flex-shrink-0 text-amber-500 dark:text-amber-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
                      </svg>
                      <span className="font-mono text-xs text-gray-900 dark:text-slate-100 truncate">{k.PSChildName}</span>
                    </div>
                    {k.subkey_count > 0 && (
                      <span className="ml-2 flex-shrink-0 rounded bg-gray-100 dark:bg-slate-700 px-1.5 py-0.5 text-[10px] text-gray-500 dark:text-slate-400 tabular-nums">
                        {k.subkey_count}
                      </span>
                    )}
                  </button>
                ))
              )}
            </div>
          </div>

          {/* Right: Values */}
          <div className="lg:col-span-2 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50 overflow-hidden">
            <div className="px-4 py-3 border-b border-gray-100 dark:border-slate-700 bg-gray-50/50 dark:bg-slate-800/50">
              <h4 className="text-xs font-semibold text-gray-500 dark:text-slate-400 uppercase tracking-wider">
                Values
                <span className="ml-2 text-gray-400 dark:text-slate-500 normal-case">({values.length})</span>
              </h4>
            </div>
            <div className="max-h-96 overflow-auto">
              {values.length === 0 ? (
                <p className="px-4 py-6 text-center text-sm text-gray-400 dark:text-slate-500">No values</p>
              ) : (
                <table className="w-full text-left text-sm">
                  <thead className="bg-gray-50/50 dark:bg-slate-800/50 sticky top-0">
                    <tr>
                      <th className="px-4 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Name</th>
                      <th className="px-4 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Type</th>
                      <th className="px-4 py-2 font-medium text-gray-500 dark:text-slate-400 text-xs">Value</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
                    {values.map((v, idx) => (
                      <tr key={idx} className="even:bg-gray-50 dark:even:bg-slate-800/50">
                        <td className="px-4 py-2 font-mono text-xs font-medium text-gray-900 dark:text-slate-100">
                          {v.name || '(Default)'}
                        </td>
                        <td className="px-4 py-2">
                          <span className="inline-flex rounded bg-gray-100 dark:bg-slate-700 px-1.5 py-0.5 text-[10px] font-medium text-gray-600 dark:text-slate-400">
                            {v.type}
                          </span>
                        </td>
                        <td className="px-4 py-2 font-mono text-xs text-gray-600 dark:text-slate-400 max-w-md break-all">
                          {v.value || '-'}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
