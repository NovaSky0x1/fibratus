import RemoteShell from '../RemoteShell'

export default function AgentTerminal({ agentId, hostname }: { agentId: string; hostname: string }) {
  return (
    <div style={{ height: 'calc(100vh - 200px)' }}>
      <RemoteShell agentId={agentId} hostname={hostname} />
    </div>
  )
}
