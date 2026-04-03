import { useState, useEffect, useCallback, useRef } from 'react'
import { api, type Command } from '../lib/api'
import {
  Folder,
  File,
  ChevronRight,
  ArrowUp,
  Download,
  Loader2,
  RefreshCw,
  AlertCircle,
} from 'lucide-react'

interface FileEntry {
  name: string
  is_dir: boolean
  size: number
  mod_time: string
}

interface ListDirectoryResult {
  path: string
  count: number
  files: FileEntry[]
}

interface GetFileResult {
  path: string
  size: number
  mod_time: string
  content: string
}

interface Props {
  agentId: string
}

type BrowseState =
  | { kind: 'idle' }
  | { kind: 'loading' }
  | { kind: 'success'; result: ListDirectoryResult }
  | { kind: 'error'; message: string }

type FilePreviewState =
  | { kind: 'idle' }
  | { kind: 'loading'; path: string }
  | { kind: 'success'; result: GetFileResult }
  | { kind: 'error'; message: string }

function formatSize(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  const value = bytes / Math.pow(1024, i)
  return value.toFixed(i === 0 ? 0 : 1) + ' ' + units[i]
}

function formatDate(iso: string): string {
  if (!iso) return '-'
  const d = new Date(iso)
  return d.toLocaleDateString() + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function parsePath(path: string): string[] {
  // Normalize slashes to backslash (Windows paths)
  const normalized = path.replace(/\//g, '\\')
  const parts = normalized.split('\\').filter(Boolean)
  return parts
}

function buildPath(segments: string[]): string {
  if (segments.length === 0) return 'C:\\'
  // First segment is drive letter like "C:"
  if (segments.length === 1) return segments[0] + '\\'
  return segments.join('\\')
}

function parentPath(currentPath: string): string {
  const segments = parsePath(currentPath)
  if (segments.length <= 1) return currentPath
  return buildPath(segments.slice(0, -1))
}

export default function FileBrowser({ agentId }: Props) {
  const [currentPath, setCurrentPath] = useState('C:\\')
  const [pathInput, setPathInput] = useState('C:\\')
  const [browseState, setBrowseState] = useState<BrowseState>({ kind: 'idle' })
  const [filePreview, setFilePreview] = useState<FilePreviewState>({ kind: 'idle' })
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const stopPolling = useCallback(() => {
    if (pollRef.current) {
      clearInterval(pollRef.current)
      pollRef.current = null
    }
  }, [])

  // Poll a command until completed or failed
  const pollCommand = useCallback(
    (commandId: string, onComplete: (cmd: Command) => void, onError: (msg: string) => void) => {
      stopPolling()
      const poll = async () => {
        try {
          const res = await api.getAgentCommands(agentId)
          const commands = (res.data || []) as Command[]
          const cmd = commands.find((c) => c.id === commandId)
          if (!cmd) return
          if (cmd.status === 'completed') {
            stopPolling()
            onComplete(cmd)
          } else if (cmd.status === 'failed') {
            stopPolling()
            onError(cmd.error_message || 'Command failed')
          }
        } catch {
          stopPolling()
          onError('Failed to poll command status')
        }
      }
      pollRef.current = setInterval(poll, 500)
      // Also run immediately
      poll()
    },
    [agentId, stopPolling],
  )

  // Cleanup polling on unmount
  useEffect(() => {
    return () => stopPolling()
  }, [stopPolling])

  // Browse a directory
  const browse = useCallback(
    async (path: string) => {
      setCurrentPath(path)
      setPathInput(path)
      setBrowseState({ kind: 'loading' })
      setFilePreview({ kind: 'idle' })
      stopPolling()

      try {
        const res = await api.createCommand(agentId, 'list_directory', { path })
        const cmd = res.data as Command | undefined
        if (!cmd) {
          setBrowseState({ kind: 'error', message: 'Failed to create command' })
          return
        }
        pollCommand(
          cmd.id,
          (completed) => {
            const result = completed.result as ListDirectoryResult
            if (result?.files) {
              // Sort: directories first, then alphabetical
              result.files.sort((a, b) => {
                if (a.is_dir && !b.is_dir) return -1
                if (!a.is_dir && b.is_dir) return 1
                return a.name.localeCompare(b.name, undefined, { sensitivity: 'base' })
              })
            }
            setBrowseState({ kind: 'success', result })
          },
          (message) => setBrowseState({ kind: 'error', message }),
        )
      } catch {
        setBrowseState({ kind: 'error', message: 'Failed to send list_directory command' })
      }
    },
    [agentId, pollCommand, stopPolling],
  )

  // Fetch a file
  const fetchFile = useCallback(
    async (filePath: string) => {
      setFilePreview({ kind: 'loading', path: filePath })

      try {
        const res = await api.createCommand(agentId, 'get_file', { path: filePath })
        const cmd = res.data as Command | undefined
        if (!cmd) {
          setFilePreview({ kind: 'error', message: 'Failed to create command' })
          return
        }

        // Use a separate poll interval for file fetches so we don't conflict with directory polling
        const filePoll = setInterval(async () => {
          try {
            const pollRes = await api.getAgentCommands(agentId)
            const commands = (pollRes.data || []) as Command[]
            const updated = commands.find((c) => c.id === cmd.id)
            if (!updated) return
            if (updated.status === 'completed') {
              clearInterval(filePoll)
              setFilePreview({ kind: 'success', result: updated.result as GetFileResult })
            } else if (updated.status === 'failed') {
              clearInterval(filePoll)
              setFilePreview({ kind: 'error', message: updated.error_message || 'Failed to retrieve file' })
            }
          } catch {
            clearInterval(filePoll)
            setFilePreview({ kind: 'error', message: 'Failed to poll file command' })
          }
        }, 2000)
      } catch {
        setFilePreview({ kind: 'error', message: 'Failed to send get_file command' })
      }
    },
    [agentId],
  )

  // Browse initial directory on mount
  useEffect(() => {
    browse('C:\\')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agentId])

  const handlePathSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const trimmed = pathInput.trim()
    if (trimmed) browse(trimmed)
  }

  const handleNavigate = (entry: FileEntry) => {
    if (entry.is_dir) {
      const target = currentPath.endsWith('\\')
        ? currentPath + entry.name
        : currentPath + '\\' + entry.name
      browse(target)
    } else {
      const target = currentPath.endsWith('\\')
        ? currentPath + entry.name
        : currentPath + '\\' + entry.name
      fetchFile(target)
    }
  }

  const handleGoUp = () => {
    const parent = parentPath(currentPath)
    if (parent !== currentPath) browse(parent)
  }

  const breadcrumbSegments = parsePath(currentPath)

  // Determine if file content looks like displayable text (not binary)
  const isTextContent = (base64: string): boolean => {
    try {
      const decoded = atob(base64)
      // Check first 512 bytes for non-text characters
      const check = decoded.slice(0, 512)
      for (let i = 0; i < check.length; i++) {
        const c = check.charCodeAt(i)
        if (c === 0) return false
        if (c < 32 && c !== 9 && c !== 10 && c !== 13) return false
      }
      return true
    } catch {
      return false
    }
  }

  const downloadFile = (result: GetFileResult) => {
    const binary = atob(result.content)
    const bytes = new Uint8Array(binary.length)
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
    const blob = new Blob([bytes])
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = result.path.split('\\').pop() || 'file'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }

  return (
    <div className="flex flex-col gap-3">
      {/* Path input bar */}
      <form onSubmit={handlePathSubmit} className="flex gap-2">
        <input
          type="text"
          value={pathInput}
          onChange={(e) => setPathInput(e.target.value)}
          placeholder="Enter path (e.g., C:\Users)"
          className="flex-1 rounded-lg border border-gray-300 px-3 py-2 text-sm font-mono focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
          onKeyDown={(e) => {
            if (e.key === 'Enter') handlePathSubmit(e)
          }}
        />
        <button
          type="button"
          onClick={() => browse(currentPath)}
          disabled={browseState.kind === 'loading'}
          className="rounded-lg border border-gray-300 px-3 py-2 text-gray-600 hover:bg-gray-50 disabled:opacity-50"
          title="Refresh"
        >
          <RefreshCw className="h-4 w-4" />
        </button>
      </form>

      {/* Breadcrumb bar */}
      <div className="flex items-center gap-1 rounded-lg bg-gray-50 px-3 py-2 text-sm overflow-x-auto">
        {breadcrumbSegments.map((segment, idx) => (
          <span key={idx} className="flex items-center gap-1 whitespace-nowrap">
            {idx > 0 && <ChevronRight className="h-3 w-3 text-gray-400 flex-shrink-0" />}
            <button
              onClick={() => browse(buildPath(breadcrumbSegments.slice(0, idx + 1)))}
              className="text-fibratus-600 hover:text-fibratus-800 hover:underline font-medium"
            >
              {segment}
            </button>
          </span>
        ))}
      </div>

      {/* Loading state */}
      {browseState.kind === 'loading' && (
        <div className="flex items-center justify-center py-12 text-gray-400">
          <Loader2 className="h-5 w-5 animate-spin mr-2" />
          <span className="text-sm">Loading directory...</span>
        </div>
      )}

      {/* Error state */}
      {browseState.kind === 'error' && (
        <div className="flex items-center gap-2 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          <AlertCircle className="h-4 w-4 flex-shrink-0" />
          <span>{browseState.message}</span>
        </div>
      )}

      {/* Directory listing */}
      {browseState.kind === 'success' && (
        <div className="rounded-xl border border-gray-200 bg-white shadow-sm overflow-hidden">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-100 bg-gray-50/50">
              <tr>
                <th className="px-4 py-2.5 font-medium text-gray-500 w-8"></th>
                <th className="px-4 py-2.5 font-medium text-gray-500">Name</th>
                <th className="px-4 py-2.5 font-medium text-gray-500 text-right w-24">Size</th>
                <th className="px-4 py-2.5 font-medium text-gray-500 text-right w-44">Modified</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {/* Go up row */}
              {parsePath(currentPath).length > 1 && (
                <tr
                  className="cursor-pointer hover:bg-gray-50/50"
                  onClick={handleGoUp}
                  onDoubleClick={handleGoUp}
                >
                  <td className="px-4 py-2">
                    <ArrowUp className="h-4 w-4 text-gray-400" />
                  </td>
                  <td className="px-4 py-2 text-gray-500 font-medium">..</td>
                  <td className="px-4 py-2"></td>
                  <td className="px-4 py-2"></td>
                </tr>
              )}
              {browseState.result.files?.length === 0 && (
                <tr>
                  <td colSpan={4} className="px-4 py-8 text-center text-gray-400">
                    Directory is empty.
                  </td>
                </tr>
              )}
              {browseState.result.files?.map((entry) => (
                <tr
                  key={entry.name}
                  className="cursor-pointer hover:bg-gray-50/50"
                  onClick={() => handleNavigate(entry)}
                  onDoubleClick={() => handleNavigate(entry)}
                >
                  <td className="px-4 py-2">
                    {entry.is_dir ? (
                      <Folder className="h-4 w-4 text-amber-500" />
                    ) : (
                      <File className="h-4 w-4 text-gray-400" />
                    )}
                  </td>
                  <td className="px-4 py-2">
                    <span
                      className={
                        entry.is_dir
                          ? 'font-medium text-gray-900 hover:text-fibratus-600'
                          : 'text-gray-700'
                      }
                    >
                      {entry.name}
                    </span>
                  </td>
                  <td className="px-4 py-2 text-right text-gray-500 tabular-nums">
                    {entry.is_dir ? '-' : formatSize(entry.size)}
                  </td>
                  <td className="px-4 py-2 text-right text-gray-500 tabular-nums">
                    {formatDate(entry.mod_time)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {browseState.result.count > 0 && (
            <div className="border-t border-gray-100 px-4 py-2 text-xs text-gray-400">
              {browseState.result.count} item(s)
            </div>
          )}
        </div>
      )}

      {/* File preview panel */}
      {filePreview.kind !== 'idle' && (
        <div className="rounded-xl border border-gray-200 bg-white shadow-sm overflow-hidden">
          <div className="flex items-center justify-between border-b border-gray-100 bg-gray-50/50 px-4 py-2.5">
            <span className="text-sm font-medium text-gray-700">File Preview</span>
            <button
              onClick={() => setFilePreview({ kind: 'idle' })}
              className="rounded p-0.5 text-gray-400 hover:bg-gray-200 hover:text-gray-600"
            >
              <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>

          <div className="p-4">
            {filePreview.kind === 'loading' && (
              <div className="flex items-center justify-center py-6 text-gray-400">
                <Loader2 className="h-5 w-5 animate-spin mr-2" />
                <span className="text-sm">Retrieving {filePreview.path.split('\\').pop()}...</span>
              </div>
            )}

            {filePreview.kind === 'error' && (
              <div className="flex items-center gap-2 text-sm text-red-600">
                <AlertCircle className="h-4 w-4 flex-shrink-0" />
                <span>{filePreview.message}</span>
              </div>
            )}

            {filePreview.kind === 'success' && (
              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <div className="text-sm text-gray-500">
                    <span className="font-mono text-xs">{filePreview.result.path}</span>
                    <span className="mx-2">-</span>
                    <span>{formatSize(filePreview.result.size)}</span>
                    <span className="mx-2">-</span>
                    <span>{formatDate(filePreview.result.mod_time)}</span>
                  </div>
                  <button
                    onClick={() => downloadFile(filePreview.result)}
                    className="flex items-center gap-1.5 rounded-lg border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50"
                  >
                    <Download className="h-3.5 w-3.5" />
                    Download
                  </button>
                </div>

                {/* Text preview for small text files, download prompt for binary/large */}
                {filePreview.result.content && isTextContent(filePreview.result.content) && filePreview.result.size <= 256 * 1024 ? (
                  <pre className="max-h-80 overflow-auto rounded-lg bg-gray-900 p-3 text-xs text-gray-100 font-mono whitespace-pre-wrap break-words">
                    {atob(filePreview.result.content)}
                  </pre>
                ) : (
                  <div className="rounded-lg bg-gray-50 px-4 py-6 text-center text-sm text-gray-500">
                    {filePreview.result.size > 256 * 1024
                      ? 'File is too large to preview. Use the download button.'
                      : 'Binary file detected. Use the download button.'}
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
