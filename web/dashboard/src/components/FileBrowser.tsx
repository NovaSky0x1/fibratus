import { useState, useEffect, useRef, useCallback } from 'react'
import { api } from '../lib/api'
import { Folder, File, ChevronRight, ArrowUp, Download, Loader2, RefreshCw, AlertCircle } from 'lucide-react'

interface FileEntry { name: string; is_dir: boolean; size: number; mod_time: string }

function fmtSize(b: number) {
  if (!b) return '-'
  const u = ['B','KB','MB','GB']
  const i = Math.floor(Math.log(b)/Math.log(1024))
  return (b/Math.pow(1024,i)).toFixed(i?1:0)+' '+u[i]
}

function fmtDate(s: string) {
  if (!s) return '-'
  try { return new Date(s).toLocaleString() } catch { return s }
}

async function waitForCommand(agentId: string, cmdId: string, signal: AbortSignal): Promise<Record<string, unknown>> {
  for (let i = 0; i < 30; i++) {
    if (signal.aborted) throw new Error('cancelled')
    await new Promise(r => setTimeout(r, 400))
    const res = await api.getAgentCommands(agentId)
    const cmds = (res?.data || []) as Array<{ id: string; status: string; result: unknown; error_message: string }>
    const cmd = cmds.find(c => c.id === cmdId)
    if (!cmd) continue
    if (cmd.status === 'completed') {
      const r = cmd.result
      if (typeof r === 'string') { try { return JSON.parse(r) } catch { return { raw: r } } }
      return (r || {}) as Record<string, unknown>
    }
    if (cmd.status === 'failed') throw new Error(cmd.error_message || 'Command failed')
  }
  throw new Error('Timed out waiting for response')
}

export default function FileBrowser({ agentId }: { agentId: string }) {
  const [path, setPath] = useState('C:\\')
  const [pathInput, setPathInput] = useState('C:\\')
  const [entries, setEntries] = useState<FileEntry[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [preview, setPreview] = useState<{ path: string; size: number; content: string; mod_time: string } | null>(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const abortRef = useRef<AbortController | null>(null)

  const browse = useCallback(async (dir: string) => {
    abortRef.current?.abort()
    const ctrl = new AbortController()
    abortRef.current = ctrl
    setPath(dir)
    setPathInput(dir)
    setLoading(true)
    setError('')
    setPreview(null)
    try {
      const res = await api.createCommand(agentId, 'list_directory', { path: dir })
      const cmdId = (res?.data as { id?: string })?.id
      if (!cmdId) throw new Error('No command ID returned')
      const result = await waitForCommand(agentId, cmdId, ctrl.signal)
      const files = (result.files || []) as FileEntry[]
      files.sort((a, b) => a.is_dir === b.is_dir ? a.name.localeCompare(b.name) : a.is_dir ? -1 : 1)
      setEntries(files)
    } catch (e: unknown) {
      if ((e as Error).message !== 'cancelled') setError((e as Error).message)
    } finally {
      setLoading(false)
    }
  }, [agentId])

  useEffect(() => { browse('C:\\'); return () => abortRef.current?.abort() }, [browse])

  const openFile = async (filePath: string) => {
    setPreviewLoading(true)
    setPreview(null)
    try {
      const res = await api.createCommand(agentId, 'get_file', { path: filePath })
      const cmdId = (res?.data as { id?: string })?.id
      if (!cmdId) throw new Error('No command ID')
      const ctrl = new AbortController()
      const result = await waitForCommand(agentId, cmdId, ctrl.signal)
      setPreview({ path: result.path as string, size: result.size as number, content: result.content as string, mod_time: result.mod_time as string })
    } catch (e: unknown) {
      setError((e as Error).message)
    } finally {
      setPreviewLoading(false)
    }
  }

  const downloadPreview = () => {
    if (!preview) return
    try {
      const bytes = atob(preview.content)
      const arr = new Uint8Array(bytes.length)
      for (let i = 0; i < bytes.length; i++) arr[i] = bytes.charCodeAt(i)
      const a = document.createElement('a')
      a.href = URL.createObjectURL(new Blob([arr]))
      a.download = preview.path.split('\\').pop() || 'file'
      a.click()
    } catch { setError('Download failed') }
  }

  const segments = path.replace(/\//g,'\\').split('\\').filter(Boolean)
  const goUp = () => { if (segments.length > 1) browse(segments.slice(0,-1).join('\\') + (segments.length === 2 && segments[0].includes(':') ? '\\' : '')) }
  const nav = (e: FileEntry) => {
    const sep = path.endsWith('\\') ? '' : '\\'
    if (e.is_dir) browse(path + sep + e.name)
    else openFile(path + sep + e.name)
  }

  return (
    <div className="flex flex-col gap-3 h-full">
      <form onSubmit={e => { e.preventDefault(); browse(pathInput.trim() || 'C:\\') }} className="flex gap-2">
        <input value={pathInput} onChange={e => setPathInput(e.target.value)}
          className="flex-1 rounded border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm font-mono text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:ring-2 focus:ring-fibratus-500 focus:outline-none" placeholder="Path..." />
        <button type="button" onClick={() => browse(path)} disabled={loading} className="p-2 text-gray-500 dark:text-slate-400 hover:text-fibratus-600 disabled:opacity-50">
          <RefreshCw className="w-4 h-4" />
        </button>
      </form>

      <div className="flex items-center gap-1 text-sm flex-wrap bg-gray-50 dark:bg-slate-800 rounded px-3 py-1.5">
        {segments.map((s, i) => (
          <span key={i} className="flex items-center gap-1">
            {i > 0 && <ChevronRight className="w-3 h-3 text-gray-400 dark:text-slate-500" />}
            <button onClick={() => browse(segments.slice(0,i+1).join('\\') + (i===0?'\\':''))} className="text-fibratus-600 dark:text-fibratus-400 hover:underline">{s}</button>
          </span>
        ))}
      </div>

      {error && (
        <div className="flex items-center gap-2 text-sm text-red-700 dark:text-red-400 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800/50 rounded px-3 py-2">
          <AlertCircle className="w-4 h-4 shrink-0" />{error}
          <button onClick={() => setError('')} className="ml-auto text-red-400 hover:text-red-600 dark:hover:text-red-300">x</button>
        </div>
      )}

      {loading ? (
        <div className="flex items-center justify-center py-16 text-gray-400 dark:text-slate-500"><Loader2 className="w-5 h-5 animate-spin mr-2" />Loading...</div>
      ) : (
        <div className="border border-gray-200 dark:border-slate-700 rounded overflow-auto flex-1">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 dark:bg-slate-900 sticky top-0">
              <tr>
                <th className="text-left px-3 py-2 w-8"></th>
                <th className="text-left px-3 py-2 text-gray-500 dark:text-slate-400">Name</th>
                <th className="text-right px-3 py-2 w-28 text-gray-500 dark:text-slate-400">Size</th>
                <th className="text-right px-3 py-2 w-44 text-gray-500 dark:text-slate-400">Modified</th>
              </tr>
            </thead>
            <tbody>
              {segments.length > 1 && (
                <tr className="hover:bg-gray-50 dark:hover:bg-slate-700 cursor-pointer" onClick={goUp}>
                  <td className="px-3 py-1.5"><ArrowUp className="w-4 h-4 text-gray-400 dark:text-slate-500" /></td>
                  <td className="px-3 py-1.5 text-gray-500 dark:text-slate-400">..</td><td /><td />
                </tr>
              )}
              {entries.map(e => (
                <tr key={e.name} className="hover:bg-gray-50 dark:hover:bg-slate-700 cursor-pointer border-t border-gray-100 dark:border-slate-700" onClick={() => nav(e)}>
                  <td className="px-3 py-1.5">{e.is_dir ? <Folder className="w-4 h-4 text-amber-500" /> : <File className="w-4 h-4 text-gray-400 dark:text-slate-500" />}</td>
                  <td className="px-3 py-1.5 font-mono text-gray-900 dark:text-slate-200">{e.name}</td>
                  <td className="px-3 py-1.5 text-right text-gray-500 dark:text-slate-400 tabular-nums">{e.is_dir ? '-' : fmtSize(e.size)}</td>
                  <td className="px-3 py-1.5 text-right text-gray-500 dark:text-slate-400 text-xs">{fmtDate(e.mod_time)}</td>
                </tr>
              ))}
              {entries.length === 0 && <tr><td colSpan={4} className="text-center py-8 text-gray-400 dark:text-slate-500">Empty</td></tr>}
            </tbody>
          </table>
        </div>
      )}

      {previewLoading && <div className="flex items-center gap-2 text-sm text-gray-500 dark:text-slate-400"><Loader2 className="w-4 h-4 animate-spin" />Fetching file...</div>}
      {preview && (
        <div className="border border-gray-200 dark:border-slate-700 rounded p-3 bg-gray-50 dark:bg-slate-800 mt-2">
          <div className="flex items-center justify-between mb-2">
            <span className="font-mono text-xs text-gray-600 dark:text-slate-400">{preview.path}</span>
            <div className="flex items-center gap-3 text-xs text-gray-500 dark:text-slate-400">
              <span>{fmtSize(preview.size)}</span>
              <button onClick={downloadPreview} className="flex items-center gap-1 px-2 py-1 bg-fibratus-600 text-white rounded hover:bg-fibratus-700">
                <Download className="w-3 h-3" />Download
              </button>
              <button onClick={() => setPreview(null)} className="text-gray-400 dark:text-slate-500 hover:text-gray-600 dark:hover:text-slate-300">x</button>
            </div>
          </div>
          {preview.size < 262144 ? (
            <pre className="text-xs font-mono bg-gray-900 dark:bg-gray-950 text-gray-100 rounded p-3 max-h-64 overflow-auto whitespace-pre-wrap break-all">
              {(() => { try { return atob(preview.content) } catch { return '[binary]' } })()}
            </pre>
          ) : <p className="text-sm text-gray-500 dark:text-slate-400 text-center py-4">File too large to preview. Download instead.</p>}
        </div>
      )}
    </div>
  )
}
