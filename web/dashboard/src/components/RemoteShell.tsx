import { useState, useRef, useEffect } from 'react'
import { useMutation } from '@tanstack/react-query'
import { api, type Command } from '../lib/api'

interface ShellEntry {
  id: string
  command: string
  output: string
  exitCode: number | null
  status: 'pending' | 'running' | 'completed' | 'failed'
  timestamp: Date
}

interface Props {
  agentId: string
  hostname: string
  shellType: 'cmd' | 'powershell'
}

export default function RemoteShell({ agentId, hostname, shellType }: Props) {
  const [input, setInput] = useState('')
  const [history, setHistory] = useState<ShellEntry[]>([])
  const [cmdHistory, setCmdHistory] = useState<string[]>([])
  const [historyIdx, setHistoryIdx] = useState(-1)
  const terminalRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  const prompt = shellType === 'powershell'
    ? `PS ${hostname}> `
    : `${hostname}> `

  // Auto-scroll to bottom
  useEffect(() => {
    if (terminalRef.current) {
      terminalRef.current.scrollTop = terminalRef.current.scrollHeight
    }
  }, [history])

  // Focus input on mount
  useEffect(() => {
    inputRef.current?.focus()
  }, [])

  // Poll for command results
  useEffect(() => {
    const pending = history.filter(e => e.status === 'pending' || e.status === 'running')
    if (pending.length === 0) return

    const interval = setInterval(async () => {
      const res = await api.getAgentCommands(agentId)
      const commands = (res.data || []) as Command[]

      setHistory(prev => prev.map(entry => {
        if (entry.status === 'completed' || entry.status === 'failed') return entry
        const cmd = commands.find(c => c.id === entry.id)
        if (!cmd) return entry
        if (cmd.status === 'completed' || cmd.status === 'failed') {
          const result = cmd.result as Record<string, unknown> | null
          return {
            ...entry,
            status: cmd.status as ShellEntry['status'],
            output: result?.output as string || cmd.error_message || 'Command completed.',
            exitCode: (result?.exit_code as number) ?? null,
          }
        }
        return { ...entry, status: cmd.status as ShellEntry['status'] }
      }))
    }, 2000)

    return () => clearInterval(interval)
  }, [history, agentId])

  const execMutation = useMutation({
    mutationFn: async (command: string) => {
      const prefix = shellType === 'powershell' ? 'powershell -Command ' : ''
      return api.createCommand(agentId, 'run_command', {
        command: prefix + command,
        timeout: 60,
      })
    },
    onSuccess: (res, command) => {
      const cmd = res.data as Command | undefined
      if (cmd) {
        setHistory(prev => prev.map(e =>
          e.command === command && e.status === 'pending' && !e.id
            ? { ...e, id: cmd.id }
            : e
        ))
      }
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const cmd = input.trim()
    if (!cmd) return

    // Built-in commands
    if (cmd === 'clear' || cmd === 'cls') {
      setHistory([])
      setInput('')
      return
    }
    if (cmd === 'help') {
      setHistory(prev => [...prev, {
        id: 'help-' + Date.now(),
        command: cmd,
        output: [
          'Remote Shell Commands:',
          '  clear/cls     - Clear terminal',
          '  help          - Show this help',
          '  exit          - Close shell',
          '',
          'All other commands are executed on the remote endpoint.',
          'Commands run with a 60-second timeout.',
          shellType === 'powershell'
            ? 'Shell: PowerShell'
            : 'Shell: CMD (cmd.exe)',
        ].join('\n'),
        exitCode: 0,
        status: 'completed',
        timestamp: new Date(),
      }])
      setInput('')
      return
    }

    // Add to command history
    setCmdHistory(prev => [cmd, ...prev.slice(0, 99)])
    setHistoryIdx(-1)

    // Add pending entry
    const entry: ShellEntry = {
      id: '',
      command: cmd,
      output: '',
      exitCode: null,
      status: 'pending',
      timestamp: new Date(),
    }
    setHistory(prev => [...prev, entry])
    setInput('')

    // Fire command
    execMutation.mutate(cmd)
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      if (cmdHistory.length > 0) {
        const idx = Math.min(historyIdx + 1, cmdHistory.length - 1)
        setHistoryIdx(idx)
        setInput(cmdHistory[idx])
      }
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      if (historyIdx > 0) {
        const idx = historyIdx - 1
        setHistoryIdx(idx)
        setInput(cmdHistory[idx])
      } else {
        setHistoryIdx(-1)
        setInput('')
      }
    }
  }

  return (
    <div className="flex flex-col h-full bg-gray-950 rounded-lg overflow-hidden font-mono text-sm">
      {/* Terminal header */}
      <div className="flex items-center gap-2 px-4 py-2 bg-gray-900 border-b border-gray-800">
        <div className="flex gap-1.5">
          <div className="w-3 h-3 rounded-full bg-red-500" />
          <div className="w-3 h-3 rounded-full bg-yellow-500" />
          <div className="w-3 h-3 rounded-full bg-green-500" />
        </div>
        <span className="text-gray-400 text-xs ml-2">
          {hostname} - Remote {shellType === 'powershell' ? 'PowerShell' : 'CMD'}
        </span>
      </div>

      {/* Terminal output */}
      <div
        ref={terminalRef}
        className="flex-1 overflow-y-auto p-4 text-gray-100 min-h-[300px] max-h-[500px]"
        onClick={() => inputRef.current?.focus()}
      >
        {/* Welcome message */}
        {history.length === 0 && (
          <div className="text-gray-500">
            <p>Connected to {hostname} via Fibratus Fleet.</p>
            <p>Type &apos;help&apos; for available commands.</p>
            <p className="mt-1" />
          </div>
        )}

        {/* Command history */}
        {history.map((entry, i) => (
          <div key={entry.id || i} className="mb-2">
            {/* Prompt + command */}
            <div className="flex">
              <span className="text-emerald-400 select-none">{prompt}</span>
              <span className="text-gray-100">{entry.command}</span>
            </div>

            {/* Output */}
            {entry.status === 'pending' && (
              <div className="text-yellow-500 animate-pulse">Executing...</div>
            )}
            {entry.status === 'running' && (
              <div className="text-blue-400 animate-pulse">Running...</div>
            )}
            {entry.status === 'failed' && (
              <div className="text-red-400">Error: {entry.output}</div>
            )}
            {entry.status === 'completed' && entry.output && (
              <pre className="text-gray-300 whitespace-pre-wrap break-words">{entry.output}</pre>
            )}
            {entry.status === 'completed' && entry.exitCode !== null && entry.exitCode !== 0 && (
              <div className="text-yellow-500 text-xs">Exit code: {entry.exitCode}</div>
            )}
          </div>
        ))}

        {/* Input line */}
        <form onSubmit={handleSubmit} className="flex">
          <span className="text-emerald-400 select-none">{prompt}</span>
          <input
            ref={inputRef}
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            className="flex-1 bg-transparent text-gray-100 outline-none caret-gray-100"
            spellCheck={false}
            autoComplete="off"
          />
        </form>
      </div>
    </div>
  )
}
