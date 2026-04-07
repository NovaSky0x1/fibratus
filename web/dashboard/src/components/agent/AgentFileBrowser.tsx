import FileBrowser from '../FileBrowser'
import { FolderOpen } from 'lucide-react'

export default function AgentFileBrowser({ agentId }: { agentId: string }) {
  return (
    <div className="space-y-3">
      {/* Header */}
      <div className="flex items-center gap-2">
        <FolderOpen className="w-4 h-4 text-gray-500 dark:text-slate-400" />
        <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Remote File Browser</h3>
      </div>

      {/* File browser */}
      <FileBrowser agentId={agentId} />
    </div>
  )
}
