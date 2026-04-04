import { useQuery } from '@tanstack/react-query'
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
} from 'recharts'
import {
  api,
  type FleetOverview,
  type Detection,
  type TimelineBucket,
} from '../lib/api'
import StatCard from '../components/StatCard'
import SeverityBadge from '../components/SeverityBadge'

const SEVERITY_COLORS: Record<string, string> = {
  critical: '#dc2626',
  high: '#ea580c',
  medium: '#d97706',
  low: '#2563eb',
}

export default function Overview() {
  const { data: overview } = useQuery({
    queryKey: ['overview'],
    queryFn: () => api.getDashboardOverview(),
  })

  const { data: detections } = useQuery({
    queryKey: ['recent-detections'],
    queryFn: () => api.getDetections({ page: '1' }),
  })

  const { data: timeline } = useQuery({
    queryKey: ['detection-timeline'],
    queryFn: () => api.getDetectionTimeline(),
  })

  const stats = overview?.data as FleetOverview | undefined
  const recentDets = (detections?.data || []) as Detection[]
  const timelineData = (timeline?.data || []) as TimelineBucket[]

  const severityData = stats?.severity_breakdown
    ? Object.entries(stats.severity_breakdown).map(([name, value]) => ({
        name,
        value,
      }))
    : []

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 dark:text-slate-100">Fleet Overview</h1>
      <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
        Real-time visibility into your endpoint fleet
      </p>

      {/* Stat cards */}
      <div className="mt-6 grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard title="Total Agents" value={stats?.total_agents ?? 0} />
        <StatCard
          title="Online"
          value={stats?.online_agents ?? 0}
          variant="success"
        />
        <StatCard
          title="Offline"
          value={stats?.offline_agents ?? 0}
          variant="danger"
        />
        <StatCard
          title="Detections (24h)"
          value={stats?.total_detections_24h ?? 0}
          variant="warning"
        />
      </div>

      {/* Charts row */}
      <div className="mt-8 grid grid-cols-1 gap-6 lg:grid-cols-3">
        {/* Detection timeline */}
        <div className="col-span-2 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50">
          <h3 className="text-sm font-medium text-gray-500 dark:text-slate-400">
            Detection Timeline (24h)
          </h3>
          {timelineData.length > 0 ? (
            <div className="mt-4 h-64">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={timelineData}>
                  <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
                  <XAxis
                    dataKey="timestamp"
                    tickFormatter={(t) =>
                      new Date(t).toLocaleTimeString([], {
                        hour: '2-digit',
                        minute: '2-digit',
                      })
                    }
                    fontSize={12}
                  />
                  <YAxis fontSize={12} />
                  <Tooltip
                    labelFormatter={(t) =>
                      new Date(t as string).toLocaleString()
                    }
                  />
                  <Bar dataKey="count" fill="#4c6ef5" radius={[4, 4, 0, 0]} />
                </BarChart>
              </ResponsiveContainer>
            </div>
          ) : (
            <p className="mt-4 text-sm text-gray-400 dark:text-slate-500">
              No detection data in the last 24 hours.
            </p>
          )}
        </div>

        {/* Severity breakdown */}
        <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50">
          <h3 className="text-sm font-medium text-gray-500 dark:text-slate-400">
            Severity Breakdown
          </h3>
          {severityData.length > 0 ? (
            <>
              <div className="mt-4 h-64">
                <ResponsiveContainer width="100%" height="100%">
                  <PieChart>
                    <Pie
                      data={severityData}
                      cx="50%"
                      cy="50%"
                      innerRadius={60}
                      outerRadius={90}
                      paddingAngle={2}
                      dataKey="value"
                    >
                      {severityData.map((entry) => (
                        <Cell
                          key={entry.name}
                          fill={SEVERITY_COLORS[entry.name] || '#6b7280'}
                        />
                      ))}
                    </Pie>
                    <Tooltip />
                  </PieChart>
                </ResponsiveContainer>
              </div>
              <div className="mt-2 flex flex-wrap justify-center gap-3">
                {severityData.map((entry) => (
                  <div
                    key={entry.name}
                    className="flex items-center gap-1.5 text-xs"
                  >
                    <span
                      className="h-2.5 w-2.5 rounded-full"
                      style={{
                        backgroundColor:
                          SEVERITY_COLORS[entry.name] || '#6b7280',
                      }}
                    />
                    <span className="capitalize text-gray-600 dark:text-slate-400">
                      {entry.name}: {entry.value}
                    </span>
                  </div>
                ))}
              </div>
            </>
          ) : (
            <p className="mt-4 text-sm text-gray-400 dark:text-slate-500">
              No severity data available.
            </p>
          )}
        </div>
      </div>

      {/* Recent detections table */}
      <div className="mt-8 rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/50">
        <div className="border-b border-gray-200 dark:border-slate-700 px-6 py-4">
          <h3 className="text-sm font-medium text-gray-900 dark:text-slate-100">
            Recent Detections
          </h3>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 dark:border-slate-700 bg-gray-50/50 dark:bg-slate-800/50">
              <tr>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Rule</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Agent</th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">
                  Severity
                </th>
                <th className="px-6 py-3 font-medium text-gray-500 dark:text-slate-400">Time</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-slate-700">
              {recentDets.slice(0, 10).map((det) => (
                <tr key={det.id} className="hover:bg-gray-50/50 dark:hover:bg-slate-700/30">
                  <td className="px-6 py-3 font-medium text-gray-900 dark:text-slate-100">
                    {det.title || det.rule_name}
                  </td>
                  <td className="px-6 py-3 text-gray-600 dark:text-slate-400">
                    {det.agent_hostname || det.agent_id}
                  </td>
                  <td className="px-6 py-3">
                    <SeverityBadge severity={det.severity} />
                  </td>
                  <td className="px-6 py-3 text-gray-500 dark:text-slate-400">
                    {new Date(det.timestamp).toLocaleString()}
                  </td>
                </tr>
              ))}
              {recentDets.length === 0 && (
                <tr>
                  <td
                    colSpan={4}
                    className="px-6 py-12 text-center text-gray-400 dark:text-slate-500"
                  >
                    No detections yet. Connect agents to start monitoring.
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
