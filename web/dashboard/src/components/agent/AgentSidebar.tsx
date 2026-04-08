import { Link } from 'react-router-dom'
import { clsx } from 'clsx'
import {
  ArrowLeft,
  Activity,
  Radio,
  Shield,
  Cpu,
  Globe,
  Server,
  HardDrive,
  Key,
  Package,
  Users,
  FolderOpen,
  Database,
  Terminal,
  Zap,
  Clock,
} from 'lucide-react'
import type { Agent } from '../../lib/api'

export interface SectionItem {
  id: string
  label: string
  icon: React.ComponentType<{ className?: string }>
}

export interface SectionGroup {
  label: string
  items: SectionItem[]
}

export const sectionGroups: SectionGroup[] = [
  {
    label: '',
    items: [
      { id: 'overview', label: 'Overview', icon: Activity },
    ],
  },
  {
    label: 'RESPOND',
    items: [
      { id: 'response', label: 'Response', icon: Zap },
      { id: 'terminal', label: 'Terminal', icon: Terminal },
      { id: 'history', label: 'History', icon: Clock },
    ],
  },
  {
    label: 'MONITOR',
    items: [
      { id: 'events', label: 'Events', icon: Radio },
      { id: 'detections', label: 'Detections', icon: Shield },
    ],
  },
  {
    label: 'ENUMERATE',
    items: [
      { id: 'processes', label: 'Processes', icon: Cpu },
      { id: 'network', label: 'Network', icon: Globe },
      { id: 'services', label: 'Services', icon: Server },
      { id: 'drivers', label: 'Drivers', icon: HardDrive },
      { id: 'autoruns', label: 'Autoruns', icon: Key },
      { id: 'software', label: 'Software', icon: Package },
      { id: 'users', label: 'Users', icon: Users },
    ],
  },
  {
    label: 'INSPECT',
    items: [
      { id: 'files', label: 'Files', icon: FolderOpen },
      { id: 'registry', label: 'Registry', icon: Database },
    ],
  },
]

function timeAgo(date: Date): string {
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
  if (seconds < 5) return 'just now'
  if (seconds < 60) return seconds + 's ago'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return minutes + 'm ago'
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return hours + 'h ago'
  const days = Math.floor(hours / 24)
  return days + 'd ago'
}

interface AgentSidebarProps {
  agent: Agent
  activeSection: string
  onSectionChange: (section: string) => void
}

export default function AgentSidebar({ agent, activeSection, onSectionChange }: AgentSidebarProps) {
  const isOnline = agent.status === 'online'

  return (
    <div className="w-64 flex-shrink-0 border-r border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 flex flex-col h-full overflow-y-auto">
      {/* Back link */}
      <Link
        to="/agents"
        className="flex items-center gap-2 px-4 py-3 text-sm text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-200 border-b border-gray-100 dark:border-slate-700 transition-colors"
      >
        <ArrowLeft className="h-4 w-4" />
        All Agents
      </Link>

      {/* Agent summary card */}
      <div className="px-4 py-4 border-b border-gray-100 dark:border-slate-700">
        <h2 className="text-xl font-bold text-gray-900 dark:text-slate-100 truncate" title={agent.hostname}>
          {agent.hostname}
        </h2>

        {/* Status badge */}
        <div className="mt-2 flex items-center gap-2">
          <span
            className={clsx(
              'inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium',
              isOnline
                ? 'bg-emerald-50 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-400'
                : agent.status === 'stale'
                  ? 'bg-amber-50 dark:bg-amber-900/20 text-amber-700 dark:text-amber-400'
                  : 'bg-gray-100 dark:bg-slate-700 text-gray-600 dark:text-slate-400'
            )}
          >
            <span className="relative flex h-2 w-2">
              {isOnline && (
                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
              )}
              <span
                className={clsx(
                  'relative inline-flex h-2 w-2 rounded-full',
                  isOnline
                    ? 'bg-emerald-500'
                    : agent.status === 'stale'
                      ? 'bg-amber-500'
                      : 'bg-gray-400'
                )}
              />
            </span>
            {agent.status}
          </span>
        </div>

        {/* Agent metadata */}
        <div className="mt-3 space-y-1.5">
          <div className="flex items-center justify-between text-xs">
            <span className="text-gray-500 dark:text-slate-500">OS</span>
            <span className="text-gray-700 dark:text-slate-300 truncate ml-2 max-w-[140px] text-right" title={agent.os_version}>
              {agent.os_version || 'Unknown'}
            </span>
          </div>
          <div className="flex items-center justify-between text-xs">
            <span className="text-gray-500 dark:text-slate-500">Engine</span>
            <span className="text-gray-700 dark:text-slate-300 font-mono">
              {agent.engine_version || 'Unknown'}
            </span>
          </div>
          <div className="flex items-center justify-between text-xs">
            <span className="text-gray-500 dark:text-slate-500">Heartbeat</span>
            <span className="text-gray-700 dark:text-slate-300">
              {agent.last_heartbeat ? timeAgo(new Date(agent.last_heartbeat)) : 'Never'}
            </span>
          </div>
        </div>
      </div>

      {/* Navigation sections */}
      <nav className="flex-1 px-3 py-3 space-y-4">
        {sectionGroups.map((group) => (
          <div key={group.label}>
            <p className="px-2 mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-gray-400 dark:text-slate-500">
              {group.label}
            </p>
            <div className="space-y-0.5">
              {group.items.map((item) => {
                const Icon = item.icon
                const isActive = activeSection === item.id
                return (
                  <button
                    key={item.id}
                    onClick={() => onSectionChange(item.id)}
                    className={clsx(
                      'flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm font-medium transition-colors',
                      isActive
                        ? 'bg-fibratus-50 dark:bg-fibratus-900/20 text-fibratus-700 dark:text-fibratus-400'
                        : 'text-gray-600 dark:text-slate-400 hover:bg-gray-50 dark:hover:bg-slate-700/50 hover:text-gray-900 dark:hover:text-slate-200'
                    )}
                  >
                    <Icon className={clsx('h-4 w-4 flex-shrink-0', isActive ? 'text-fibratus-600 dark:text-fibratus-400' : '')} />
                    {item.label}
                  </button>
                )
              })}
            </div>
          </div>
        ))}
      </nav>
    </div>
  )
}
