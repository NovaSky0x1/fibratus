import { useState, useCallback, useRef, useEffect } from 'react'
import { api, Command } from '../lib/api'

interface UseAgentCommandOptions {
  autoExecute?: boolean  // fire on mount
  pollInterval?: number  // ms, default 2000
  timeout?: number       // ms, default 60000
}

interface UseAgentCommandResult<T> {
  data: T | null
  isLoading: boolean
  error: string | null
  execute: () => void
  lastUpdated: Date | null
}

export function useAgentCommand<T>(
  agentId: string,
  commandType: string,
  payload?: Record<string, unknown>,
  options: UseAgentCommandOptions = {}
): UseAgentCommandResult<T> {
  const { autoExecute = true, pollInterval = 2000, timeout = 60000 } = options
  const [data, setData] = useState<T | null>(null)
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const cmdIdRef = useRef<string | null>(null)

  const cleanup = useCallback(() => {
    if (pollRef.current) clearInterval(pollRef.current)
    if (timeoutRef.current) clearTimeout(timeoutRef.current)
    pollRef.current = null
    timeoutRef.current = null
  }, [])

  const execute = useCallback(async () => {
    cleanup()
    setIsLoading(true)
    setError(null)

    try {
      const resp = await api.createCommand(agentId, commandType, payload)
      if (resp.error) {
        setError(resp.error.message)
        setIsLoading(false)
        return
      }
      const cmdId = resp.data?.id
      if (!cmdId) {
        setError('No command ID returned')
        setIsLoading(false)
        return
      }
      cmdIdRef.current = cmdId

      // Set timeout
      timeoutRef.current = setTimeout(() => {
        cleanup()
        setError('Command timed out')
        setIsLoading(false)
      }, timeout)

      // Poll for result
      pollRef.current = setInterval(async () => {
        try {
          const cmdsResp = await api.getAgentCommands(agentId)
          const cmd = (cmdsResp.data || []).find((c: Command) => c.id === cmdId)
          if (!cmd) return

          if (cmd.status === 'completed') {
            cleanup()
            try {
              const result = typeof cmd.result === 'string' ? JSON.parse(cmd.result) : cmd.result
              setData(result as T)
            } catch {
              setData(cmd.result as T)
            }
            setLastUpdated(new Date())
            setIsLoading(false)
          } else if (cmd.status === 'failed') {
            cleanup()
            setError(cmd.error_message || 'Command failed')
            setIsLoading(false)
          }
        } catch {
          // ignore poll errors, keep polling
        }
      }, pollInterval)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Unknown error')
      setIsLoading(false)
    }
  }, [agentId, commandType, payload, pollInterval, timeout, cleanup])

  useEffect(() => {
    if (autoExecute && agentId) {
      execute()
    }
    return cleanup
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return { data, isLoading, error, execute, lastUpdated }
}
