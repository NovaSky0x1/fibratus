import { useQuery } from '@tanstack/react-query'
import { api, Detection } from '../../lib/api'
import { useTableSort } from '../../hooks/useTableSort'
import SortableHeader from '../SortableHeader'
import SeverityBadge from '../SeverityBadge'
import { RefreshCw, ExternalLink } from 'lucide-react'

function timeAgo(date: Date): string {
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
  if (seconds < 60) return seconds + 's ago'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return minutes + 'm ago'
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return hours + 'h ago'
  return Math.floor(hours / 24) + 'd ago'
}

function Skeleton({ className = '' }: { className?: string }) {
  return <div className={`animate-pulse bg-gray-200 dark:bg-slate-700 rounded ${className}`} />
}

export default function AgentDetections({ agentId }: { agentId: string }) {
  const { data: res, isLoading, refetch } = useQuery({
    queryKey: ['agent-detections', agentId],
    queryFn: () => api.getDetections({ agent_id: agentId }),
    refetchInterval: 30000,
  })

  const detections = (res?.data || []) as Detection[]
  const { sorted, sort, toggleSort } = useTableSort(detections, 'timestamp', 'desc')

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Detections</h3>
        <div className="flex items-center gap-3">
          <span className="text-xs text-gray-400 dark:text-slate-500 tabular-nums">{detections.length} detection(s)</span>
          <button
            onClick={() => refetch()}
            className="p-1.5 rounded text-gray-400 dark:text-slate-500 hover:text-gray-600 dark:hover:text-slate-300 hover:bg-gray-100 dark:hover:bg-slate-700 transition-colors"
          >
            <RefreshCw className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50 overflow-hidden">
        <div className="overflow-auto" style={{ maxHeight: 'calc(100vh - 260px)' }}>
          <table className="w-full text-left text-sm">
            <thead className="bg-gray-50/50 dark:bg-slate-800/50 border-b border-gray-100 dark:border-slate-700 sticky top-0 z-10">
              <tr>
                <SortableHeader label="Rule Name" sortKey="rule_name" sort={sort} onSort={toggleSort} className="text-xs" />
                <SortableHeader label="Severity" sortKey="severity" sort={sort} onSort={toggleSort} className="text-xs" />
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400 text-xs">MITRE Technique</th>
                <SortableHeader label="Timestamp" sortKey="timestamp" sort={sort} onSort={toggleSort} className="text-xs" />
                <th className="px-4 py-3 w-10" />
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {isLoading && (
                <tr><td colSpan={5} className="p-6">
                  <div className="space-y-3">
                    {[...Array(5)].map((_, i) => <Skeleton key={i} className="h-8 w-full" />)}
                  </div>
                </td></tr>
              )}
              {!isLoading && sorted.map(det => (
                <tr
                  key={det.id}
                  className="cursor-pointer hover:bg-gray-50/50 dark:hover:bg-slate-700/50 transition-colors"
                  onClick={() => window.location.href = `/detections?id=${det.id}`}
                >
                  <td className="px-6 py-3">
                    <span className="font-medium text-gray-900 dark:text-slate-100">{det.rule_name}</span>
                    {det.text && (
                      <p className="mt-0.5 text-xs text-gray-500 dark:text-slate-400 truncate max-w-md">{det.text}</p>
                    )}
                  </td>
                  <td className="px-6 py-3"><SeverityBadge severity={det.severity} /></td>
                  <td className="px-6 py-3 font-mono text-xs text-gray-500 dark:text-slate-400">
                    {det.labels?.['technique.id'] || '-'}
                  </td>
                  <td className="px-6 py-3 text-gray-500 dark:text-slate-400 text-xs tabular-nums whitespace-nowrap">
                    <div>{timeAgo(new Date(det.timestamp))}</div>
                    <div className="text-[10px] text-gray-400 dark:text-slate-500 mt-0.5">
                      {new Date(det.timestamp).toLocaleString()}
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <ExternalLink className="w-3.5 h-3.5 text-gray-400 dark:text-slate-500" />
                  </td>
                </tr>
              ))}
              {!isLoading && sorted.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-6 py-12 text-center text-gray-400 dark:text-slate-500">
                    No detections for this agent
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
