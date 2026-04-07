import { useState } from 'react'
import RemoteShell from '../RemoteShell'
import { Terminal } from 'lucide-react'

export default function AgentTerminal({ agentId, hostname }: { agentId: string; hostname: string }) {
  const [shellType, setShellType] = useState<'cmd' | 'powershell'>('powershell')

  return (
    <div className="space-y-3">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Terminal className="w-4 h-4 text-gray-500 dark:text-slate-400" />
          <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Remote Terminal</h3>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-gray-500 dark:text-slate-400">Shell:</span>
          <button
            onClick={() => setShellType('powershell')}
            className={`px-3 py-1 text-xs rounded-full font-medium transition-colors ${
              shellType === 'powershell'
                ? 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400'
                : 'bg-gray-100 dark:bg-slate-700 text-gray-600 dark:text-slate-400 hover:bg-gray-200 dark:hover:bg-slate-600'
            }`}
          >
            PowerShell
          </button>
          <button
            onClick={() => setShellType('cmd')}
            className={`px-3 py-1 text-xs rounded-full font-medium transition-colors ${
              shellType === 'cmd'
                ? 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400'
                : 'bg-gray-100 dark:bg-slate-700 text-gray-600 dark:text-slate-400 hover:bg-gray-200 dark:hover:bg-slate-600'
            }`}
          >
            CMD
          </button>
        </div>
      </div>

      {/* Shell */}
      <RemoteShell agentId={agentId} hostname={hostname} shellType={shellType} />
    </div>
  )
}
