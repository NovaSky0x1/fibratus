import { createContext, useContext, useState, useEffect } from 'react'
import { api, isAuthenticated } from '../lib/api'

interface PermissionContextValue {
  permissions: Set<string>
  loading: boolean
  hasPermission: (perm: string) => boolean
  hasAnyPermission: (...perms: string[]) => boolean
  isRoot: boolean
}

const PermissionContext = createContext<PermissionContextValue>({
  permissions: new Set(),
  loading: true,
  hasPermission: () => false,
  hasAnyPermission: () => false,
  isRoot: false,
})

export function PermissionProvider({ children }: { children: React.ReactNode }) {
  const [permissions, setPermissions] = useState<Set<string>>(new Set())
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!isAuthenticated()) {
      setLoading(false)
      return
    }
    api.getMyPermissions().then(res => {
      const perms = (res.data || []) as string[]
      setPermissions(new Set(perms))
      setLoading(false)
    }).catch(() => setLoading(false))
  }, [])

  const hasPermission = (perm: string) => permissions.has(perm)
  const hasAnyPermission = (...perms: string[]) => perms.some(p => permissions.has(p))
  const isRoot = permissions.has('admin:panel') && permissions.has('accounts:manage')

  return (
    <PermissionContext.Provider value={{ permissions, loading, hasPermission, hasAnyPermission, isRoot }}>
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
