import { createContext, useCallback, useContext, useEffect, useState } from 'react'
import { api, isAuthenticated } from '../lib/api'

interface PermissionContextValue {
  permissions: Set<string>
  loading: boolean
  hasPermission: (perm: string) => boolean
  hasAnyPermission: (...perms: string[]) => boolean
  isRoot: boolean
  refresh: () => Promise<void>
  clear: () => void
}

const PermissionContext = createContext<PermissionContextValue>({
  permissions: new Set(),
  loading: true,
  hasPermission: () => false,
  hasAnyPermission: () => false,
  isRoot: false,
  refresh: async () => {},
  clear: () => {},
})

export function PermissionProvider({ children }: { children: React.ReactNode }) {
  const [permissions, setPermissions] = useState<Set<string>>(new Set())
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(async () => {
    if (!isAuthenticated()) {
      setPermissions(new Set())
      setLoading(false)
      return
    }
    setLoading(true)
    try {
      const res = await api.getMyPermissions()
      const perms = (res.data || []) as string[]
      setPermissions(new Set(perms))
    } catch {
      setPermissions(new Set())
    } finally {
      setLoading(false)
    }
  }, [])

  const clear = useCallback(() => {
    setPermissions(new Set())
    setLoading(false)
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  const hasPermission = (perm: string) => permissions.has(perm)
  const hasAnyPermission = (...perms: string[]) => perms.some(p => permissions.has(p))
  const isRoot = permissions.has('admin:panel') && permissions.has('accounts:manage')

  return (
    <PermissionContext.Provider value={{ permissions, loading, hasPermission, hasAnyPermission, isRoot, refresh, clear }}>
      {children}
    </PermissionContext.Provider>
  )
}

export function usePermissions() {
  return useContext(PermissionContext)
}

export function usePermission(perm: string): boolean {
  const { hasPermission } = useContext(PermissionContext)
  return hasPermission(perm)
}
