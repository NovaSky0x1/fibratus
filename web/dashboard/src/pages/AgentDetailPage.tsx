import { useState } from 'react'
import { useParams, Navigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import AgentSidebar from '../components/agent/AgentSidebar'
import AgentOverview from '../components/agent/AgentOverview'
import AgentLiveEvents from '../components/agent/AgentLiveEvents'
import AgentDetections from '../components/agent/AgentDetections'
import AgentProcesses from '../components/agent/AgentProcesses'
import AgentNetwork from '../components/agent/AgentNetwork'
import AgentServices from '../components/agent/AgentServices'
import AgentDrivers from '../components/agent/AgentDrivers'
import AgentAutoruns from '../components/agent/AgentAutoruns'
import AgentSoftware from '../components/agent/AgentSoftware'
import AgentUsers from '../components/agent/AgentUsers'
import AgentFileBrowser from '../components/agent/AgentFileBrowser'
import AgentRegistry from '../components/agent/AgentRegistry'
import AgentTerminal from '../components/agent/AgentTerminal'
import AgentResponse from '../components/agent/AgentResponse'
import AgentCommandHistory from '../components/agent/AgentCommandHistory'
import AgentCaptures from '../components/agent/AgentCaptures'

export default function AgentDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [activeSection, setActiveSection] = useState('overview')

  const { data: agentResp, isLoading, error } = useQuery({
    queryKey: ['agent', id],
    queryFn: () => api.getAgent(id!),
    enabled: !!id,
    refetchInterval: 30000,
  })

  const agent = agentResp?.data

  if (!id) return <Navigate to="/agents" replace />

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-[calc(100vh-4rem)]">
        <div className="flex flex-col items-center gap-3">
          <div className="h-8 w-8 animate-spin rounded-full border-2 border-fibratus-600 border-t-transparent" />
          <span className="text-sm text-gray-500 dark:text-slate-400">Loading agent...</span>
        </div>
      </div>
    )
  }

  if (error || !agent) {
    return (
      <div className="flex items-center justify-center h-[calc(100vh-4rem)]">
        <div className="text-center">
          <p className="text-lg font-medium text-gray-900 dark:text-slate-100">Agent not found</p>
          <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
            {error instanceof Error ? error.message : 'The requested agent could not be loaded.'}
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="flex -mx-8 -my-8" style={{ height: 'calc(100vh - 0px)' }}>
      <AgentSidebar
        agent={agent}
        activeSection={activeSection}
        onSectionChange={setActiveSection}
      />

      <div className="flex-1 overflow-y-auto bg-gray-50 dark:bg-slate-900">
        <div className="px-8 py-6">
          <div className="mb-6">
            <h2 className="text-xl font-bold text-gray-900 dark:text-slate-100 capitalize">
              {activeSection}
            </h2>
            <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
              {agent.hostname} — {sectionDescription(activeSection)}
            </p>
          </div>

          {activeSection === 'overview' && <AgentOverview agent={agent} />}
          {activeSection === 'events' && <AgentLiveEvents agentId={agent.id} />}
          {activeSection === 'detections' && <AgentDetections agentId={agent.id} />}
          {activeSection === 'processes' && <AgentProcesses agentId={agent.id} />}
          {activeSection === 'network' && <AgentNetwork agentId={agent.id} />}
          {activeSection === 'services' && <AgentServices agentId={agent.id} />}
          {activeSection === 'drivers' && <AgentDrivers agentId={agent.id} />}
          {activeSection === 'autoruns' && <AgentAutoruns agentId={agent.id} />}
          {activeSection === 'software' && <AgentSoftware agentId={agent.id} />}
          {activeSection === 'users' && <AgentUsers agentId={agent.id} />}
          {activeSection === 'files' && <AgentFileBrowser agentId={agent.id} />}
          {activeSection === 'registry' && <AgentRegistry agentId={agent.id} />}
          {activeSection === 'terminal' && <AgentTerminal agentId={agent.id} hostname={agent.hostname} />}
          {activeSection === 'captures' && <AgentCaptures agentId={agent.id} />}
          {activeSection === 'response' && <AgentResponse agentId={agent.id} agent={agent} />}
          {activeSection === 'history' && <AgentCommandHistory agentId={agent.id} />}
        </div>
      </div>
    </div>
  )
}

function sectionDescription(section: string): string {
  const descriptions: Record<string, string> = {
    overview: 'System health, resource usage, and agent status',
    events: 'Real-time kernel event stream from this agent',
    detections: 'Security detections triggered on this agent',
    processes: 'Running processes and process tree',
    network: 'Active network connections and listeners',
    services: 'Windows services and their status',
    drivers: 'Loaded kernel drivers and modules',
    autoruns: 'Persistence mechanisms and autostart entries',
    software: 'Installed software and applications',
    users: 'Local user accounts and sessions',
    files: 'Remote file system browser',
    registry: 'Windows registry browser',
    terminal: 'Interactive remote shell',
    captures: 'Kernel event captures (.kcap) for deep investigations',
    response: 'Active response actions',
    history: 'Command execution history',
  }
  return descriptions[section] || ''
}
