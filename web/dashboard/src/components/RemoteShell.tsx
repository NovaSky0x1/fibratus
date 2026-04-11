import { useState, useRef, useEffect, useCallback } from 'react'
import { useMutation } from '@tanstack/react-query'
import { api, type Command } from '../lib/api'
import { Copy, Check, Trash2, Maximize2, Minimize2, Search, X } from 'lucide-react'

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
  shellType?: 'cmd' | 'powershell'
}

// Common IR commands for autocomplete
const IR_COMMANDS = [
  { cmd: 'whoami /all', desc: 'Current user + groups + privileges' },
  { cmd: 'net user', desc: 'List local users' },
  { cmd: 'net localgroup administrators', desc: 'Local admin group members' },
  { cmd: 'netstat -ano', desc: 'All network connections with PIDs' },
  { cmd: 'netstat -ano | findstr ESTABLISHED', desc: 'Active connections' },
  { cmd: 'netstat -ano | findstr LISTENING', desc: 'Listening ports' },
  { cmd: 'tasklist /V', desc: 'Running processes (verbose)' },
  { cmd: 'tasklist /SVC', desc: 'Processes with services' },
  { cmd: 'wmic process list brief', desc: 'Processes (WMI)' },
  { cmd: 'sc query type= service state= all', desc: 'All services' },
  { cmd: 'schtasks /query /fo LIST /v', desc: 'Scheduled tasks' },
  { cmd: 'reg query HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Run', desc: 'Run keys (HKLM)' },
  { cmd: 'reg query HKCU\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Run', desc: 'Run keys (HKCU)' },
  { cmd: 'wmic startup list full', desc: 'Startup items' },
  { cmd: 'ipconfig /all', desc: 'Network configuration' },
  { cmd: 'arp -a', desc: 'ARP cache' },
  { cmd: 'route print', desc: 'Routing table' },
  { cmd: 'systeminfo', desc: 'System information' },
  { cmd: 'wmic qfe list', desc: 'Installed patches' },
  { cmd: 'dir /s /b C:\\Users\\Public', desc: 'Public folder contents' },
  { cmd: 'dir /s /b C:\\Windows\\Temp', desc: 'Temp folder contents' },
  { cmd: 'type C:\\Windows\\System32\\drivers\\etc\\hosts', desc: 'Hosts file' },
  { cmd: 'wevtutil qe Security /c:20 /f:text /rd:true', desc: 'Last 20 security events' },
  { cmd: 'wevtutil qe System /c:20 /f:text /rd:true', desc: 'Last 20 system events' },
  { cmd: 'Get-Process | Sort CPU -Descending | Select -First 20', desc: 'Top 20 by CPU (PS)' },
  { cmd: 'Get-ChildItem Env:', desc: 'Environment variables (PS)' },
  { cmd: 'Get-WinEvent -LogName Security -MaxEvents 20', desc: 'Security events (PS)' },
  { cmd: 'Get-LocalUser | Format-Table', desc: 'Local users (PS)' },
  { cmd: 'Get-NetTCPConnection | Where State -eq Established', desc: 'Active connections (PS)' },
]

export default function RemoteShell({ agentId, hostname, shellType: initialShellType }: Props) {
  const [input, setInput] = useState('')
  const [history, setHistory] = useState<ShellEntry[]>([])
  const [cmdHistory, setCmdHistory] = useState<string[]>([])
  const [historyIdx, setHistoryIdx] = useState(-1)
  const [shellType, setShellType] = useState<'cmd' | 'powershell'>(initialShellType || 'powershell')
  const [fullscreen, setFullscreen] = useState(false)
  const [searchTerm, setSearchTerm] = useState('')
  const [showSearch, setShowSearch] = useState(false)
  const [showSuggestions, setShowSuggestions] = useState(false)
  const [copiedId, setCopiedId] = useState<string | null>(null)
  const terminalRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  const prompt = shellType === 'powershell' ? `PS ${hostname}> ` : `${hostname}> `

  useEffect(() => {
    if (terminalRef.current) terminalRef.current.scrollTop = terminalRef.current.scrollHeight
  }, [history])

  useEffect(() => { inputRef.current?.focus() }, [])

  // Keyboard shortcuts
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.ctrlKey && e.key === 'l') { e.preventDefault(); setHistory([]) }
      if (e.ctrlKey && e.key === 'f') { e.preventDefault(); setShowSearch(s => !s) }
      if (e.key === 'Escape') { setShowSearch(false); setShowSuggestions(false) }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [])

  // Poll for command results
  useEffect(() => {
    const pending = history.filter(e => e.status === 'pending' || e.status === 'running')
    if (pending.length === 0) return
    const interval = setInterval(async () => {
      const pendingIds = history.filter(e => e.status === 'pending' || e.status === 'running').map(e => e.id)
      const commands: Command[] = []
      for (const id of pendingIds) {
        const r = await api.getCommand(id)
        if (r.data) commands.push(r.data as Command)
      }
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
    }, 1500)
    return () => clearInterval(interval)
  }, [history, agentId])

  const execMutation = useMutation({
    mutationFn: async (command: string) => {
      const prefix = shellType === 'powershell' ? 'powershell -NoProfile -Command ' : ''
      return api.createCommand(agentId, 'run_command', { command: prefix + command, timeout: 120 })
    },
    onSuccess: (res, command) => {
      const cmd = res.data as Command | undefined
      if (cmd) {
        setHistory(prev => prev.map(e =>
          e.command === command && e.status === 'pending' && !e.id ? { ...e, id: cmd.id } : e
        ))
      }
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const cmd = input.trim()
    if (!cmd) return
    setShowSuggestions(false)

    if (cmd === 'clear' || cmd === 'cls') { setHistory([]); setInput(''); return }
    if (cmd === 'help') {
      setHistory(prev => [...prev, {
        id: 'help-' + Date.now(), command: cmd, exitCode: 0, status: 'completed', timestamp: new Date(),
        output: `Remote Shell — ${hostname}\n\nBuilt-in:\n  clear/cls    Clear terminal\n  help         Show this help\n  Ctrl+L       Clear terminal\n  Ctrl+F       Search output\n  Tab          Show command suggestions\n\nShell: ${shellType === 'powershell' ? 'PowerShell' : 'CMD'}\nTimeout: 120 seconds\nAll commands execute on the remote endpoint via Fibratus Fleet.`,
      }])
      setInput(''); return
    }

    setCmdHistory(prev => [cmd, ...prev.filter(c => c !== cmd).slice(0, 99)])
    setHistoryIdx(-1)
    setHistory(prev => [...prev, { id: '', command: cmd, output: '', exitCode: null, status: 'pending', timestamp: new Date() }])
    setInput('')
    execMutation.mutate(cmd)
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      if (cmdHistory.length > 0) {
        const idx = Math.min(historyIdx + 1, cmdHistory.length - 1)
        setHistoryIdx(idx); setInput(cmdHistory[idx])
      }
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      if (historyIdx > 0) { const idx = historyIdx - 1; setHistoryIdx(idx); setInput(cmdHistory[idx]) }
      else { setHistoryIdx(-1); setInput('') }
    }
    if (e.key === 'Tab') {
      e.preventDefault()
      setShowSuggestions(s => !s)
    }
  }

  const copyOutput = useCallback((id: string, text: string) => {
    navigator.clipboard.writeText(text)
    setCopiedId(id)
    setTimeout(() => setCopiedId(null), 2000)
  }, [])

  const suggestions = input.length > 0
    ? IR_COMMANDS.filter(c => c.cmd.toLowerCase().includes(input.toLowerCase()) || c.desc.toLowerCase().includes(input.toLowerCase()))
    : IR_COMMANDS

  const filteredHistory = searchTerm
    ? history.filter(e => e.command.toLowerCase().includes(searchTerm.toLowerCase()) || e.output.toLowerCase().includes(searchTerm.toLowerCase()))
    : history

  const containerClass = fullscreen
    ? 'fixed inset-0 z-50 flex flex-col bg-gray-950'
    : 'flex flex-col bg-gray-950 rounded-xl overflow-hidden border border-gray-800'

  const terminalHeight = fullscreen ? 'flex-1' : 'min-h-[500px]'

  return (
    <div className={containerClass}>
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-2.5 bg-gray-900 border-b border-gray-800 shrink-0">
        <div className="flex items-center gap-3">
          <div className="flex gap-1.5">
            <div className="w-3 h-3 rounded-full bg-red-500 cursor-pointer hover:brightness-125" onClick={() => setHistory([])} title="Clear" />
            <div className="w-3 h-3 rounded-full bg-yellow-500" />
            <div className="w-3 h-3 rounded-full bg-green-500" />
          </div>
          <span className="text-gray-400 text-xs font-mono">{hostname}</span>

          {/* Shell toggle */}
          <div className="flex rounded-md overflow-hidden border border-gray-700">
            <button onClick={() => setShellType('powershell')}
              className={'px-2.5 py-1 text-[10px] font-medium transition-colors ' + (shellType === 'powershell' ? 'bg-blue-600 text-white' : 'bg-gray-800 text-gray-400 hover:text-gray-300')}>
              PowerShell
            </button>
            <button onClick={() => setShellType('cmd')}
              className={'px-2.5 py-1 text-[10px] font-medium transition-colors ' + (shellType === 'cmd' ? 'bg-blue-600 text-white' : 'bg-gray-800 text-gray-400 hover:text-gray-300')}>
              CMD
            </button>
          </div>
        </div>

        <div className="flex items-center gap-1.5">
          <button onClick={() => setShowSearch(s => !s)} className="p-1.5 rounded text-gray-500 hover:text-gray-300 hover:bg-gray-800" title="Search (Ctrl+F)">
            <Search className="w-3.5 h-3.5" />
          </button>
          <button onClick={() => setHistory([])} className="p-1.5 rounded text-gray-500 hover:text-gray-300 hover:bg-gray-800" title="Clear (Ctrl+L)">
            <Trash2 className="w-3.5 h-3.5" />
          </button>
          <button onClick={() => setFullscreen(f => !f)} className="p-1.5 rounded text-gray-500 hover:text-gray-300 hover:bg-gray-800" title={fullscreen ? 'Exit fullscreen' : 'Fullscreen'}>
            {fullscreen ? <Minimize2 className="w-3.5 h-3.5" /> : <Maximize2 className="w-3.5 h-3.5" />}
          </button>
        </div>
      </div>

      {/* Search bar */}
      {showSearch && (
        <div className="flex items-center gap-2 px-4 py-2 bg-gray-900/80 border-b border-gray-800">
          <Search className="w-3.5 h-3.5 text-gray-500" />
          <input
            type="text" value={searchTerm} onChange={e => setSearchTerm(e.target.value)}
            placeholder="Search output..." autoFocus
            className="flex-1 bg-transparent text-sm text-gray-200 placeholder-gray-600 outline-none font-mono"
          />
          <span className="text-[10px] text-gray-500">{searchTerm ? `${filteredHistory.length} matches` : ''}</span>
          <button onClick={() => { setShowSearch(false); setSearchTerm('') }} className="p-1 rounded text-gray-500 hover:text-gray-300">
            <X className="w-3.5 h-3.5" />
          </button>
        </div>
      )}

      {/* Terminal body */}
      <div ref={terminalRef} className={`${terminalHeight} overflow-y-auto p-4 text-gray-100 font-mono text-[13px] leading-relaxed`}
        onClick={() => inputRef.current?.focus()}>

        {filteredHistory.length === 0 && !searchTerm && (
          <div className="text-gray-600 mb-4">
            <p className="text-gray-400">Fibratus Fleet Remote Shell</p>
            <p className="mt-1">Connected to <span className="text-emerald-400">{hostname}</span></p>
            <p className="text-gray-600 mt-1">Type <span className="text-gray-400">help</span> for commands, <span className="text-gray-400">Tab</span> for suggestions</p>
          </div>
        )}

        {filteredHistory.map((entry, i) => (
          <div key={entry.id || i} className="mb-3 group">
            <div className="flex items-start justify-between">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <span className="text-emerald-400 select-none shrink-0">{prompt}</span>
                  <span className="text-gray-100">{entry.command}</span>
                  <span className="text-gray-700 text-[10px] shrink-0">{entry.timestamp.toLocaleTimeString()}</span>
                </div>
              </div>
              {entry.status === 'completed' && entry.output && (
                <button
                  onClick={() => copyOutput(entry.id, entry.output)}
                  className="ml-2 p-1 rounded text-gray-700 hover:text-gray-400 opacity-0 group-hover:opacity-100 transition-opacity shrink-0"
                  title="Copy output"
                >
                  {copiedId === entry.id ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                </button>
              )}
            </div>

            {entry.status === 'pending' && <div className="text-yellow-500/80 ml-[2ch] mt-0.5 animate-pulse text-xs">Executing...</div>}
            {entry.status === 'running' && <div className="text-blue-400/80 ml-[2ch] mt-0.5 animate-pulse text-xs">Running...</div>}
            {entry.status === 'failed' && <div className="text-red-400 ml-[2ch] mt-0.5">Error: {entry.output}</div>}
            {entry.status === 'completed' && entry.output && (
              <pre className="text-gray-400 ml-[2ch] mt-0.5 whitespace-pre-wrap break-words text-[12px]">{entry.output}</pre>
            )}
            {entry.status === 'completed' && entry.exitCode !== null && entry.exitCode !== 0 && (
              <div className="text-amber-500/70 ml-[2ch] text-[10px]">Exit code: {entry.exitCode}</div>
            )}
          </div>
        ))}

        {/* Input line */}
        <form onSubmit={handleSubmit} className="flex items-center">
          <span className="text-emerald-400 select-none shrink-0">{prompt}</span>
          <input
            ref={inputRef} type="text" value={input}
            onChange={(e) => { setInput(e.target.value); if (e.target.value) setShowSuggestions(false) }}
            onKeyDown={handleKeyDown}
            className="flex-1 bg-transparent text-gray-100 outline-none caret-emerald-400"
            spellCheck={false} autoComplete="off"
          />
        </form>

        {/* Command suggestions */}
        {showSuggestions && (
          <div className="mt-2 rounded-lg border border-gray-800 bg-gray-900/95 max-h-64 overflow-auto">
            {suggestions.map((s, i) => (
              <button key={i}
                onClick={() => { setInput(s.cmd); setShowSuggestions(false); inputRef.current?.focus() }}
                className="w-full flex items-center justify-between px-3 py-1.5 hover:bg-gray-800 text-left"
              >
                <span className="text-[12px] font-mono text-emerald-400">{s.cmd}</span>
                <span className="text-[10px] text-gray-600 ml-4 shrink-0">{s.desc}</span>
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
