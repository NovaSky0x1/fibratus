import { useEffect, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { clsx } from 'clsx'
import { useQueryClient } from '@tanstack/react-query'
import { api, clearSession, getCurrentOrgId, setCurrentOrgId, type Account, type Organization, type User } from '../lib/api'
import { usePermissions } from '../contexts/PermissionContext'
import CursorGlow from './CursorGlow'

const navigation = [
  { name: 'Overview', href: '/', perm: 'page:overview' },
  { name: 'Agents', href: '/agents', perm: 'page:agents' },
  { name: 'Detections', href: '/detections', perm: 'page:detections' },
  { name: 'Events', href: '/events', perm: 'page:events' },
  { name: 'Rules', href: '/rules', perm: 'page:rules' },
  { name: 'Macros', href: '/macros', perm: 'page:macros' },
  { name: 'Audit Log', href: '/audit-log', perm: 'page:audit' },
]

function getInitialTheme(): 'dark' | 'light' {
  const stored = localStorage.getItem('theme')
  if (stored === 'dark' || stored === 'light') return stored
  return 'dark'
}

function SunIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="5" />
      <line x1="12" y1="1" x2="12" y2="3" />
      <line x1="12" y1="21" x2="12" y2="23" />
      <line x1="4.22" y1="4.22" x2="5.64" y2="5.64" />
      <line x1="18.36" y1="18.36" x2="19.78" y2="19.78" />
      <line x1="1" y1="12" x2="3" y2="12" />
      <line x1="21" y1="12" x2="23" y2="12" />
      <line x1="4.22" y1="19.78" x2="5.64" y2="18.36" />
      <line x1="18.36" y1="5.64" x2="19.78" y2="4.22" />
    </svg>
  )
}

function MoonIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
    </svg>
  )
}

export default function Layout({ children }: { children: React.ReactNode }) {
  const location = useLocation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [orgs, setOrgs] = useState<Organization[]>([])
  const [currentOrg, setCurrentOrg] = useState(getCurrentOrgId())
  const [user, setUser] = useState<User | null>(null)
  const [mfaRequired, setMfaRequired] = useState(false)
  const isAdminOrRoot = user?.role === 'admin' || user?.role === 'root'
  const [accounts, setAccounts] = useState<Account[]>([])
  const [selectedAccountId, setSelectedAccountId] = useState<string>('')
  const [theme, setTheme] = useState<'dark' | 'light'>(getInitialTheme)

  const isRoot = user?.role === 'root'

  // Permission-based access control (augments role checks)
  const { hasPermission, loading: permsLoading } = usePermissions()
  const canSuperAdmin = hasPermission('admin:panel')
  const showManagement = permsLoading ? isAdminOrRoot : hasPermission('page:management')
  const showSuperAdmin = permsLoading ? isRoot : canSuperAdmin

  // MFA enforcement is now handled in the login flow — no post-login blocking needed

  // Apply theme on mount and when it changes
  useEffect(() => {
    if (theme === 'dark') {
      document.documentElement.classList.add('dark')
    } else {
      document.documentElement.classList.remove('dark')
    }
    localStorage.setItem('theme', theme)
  }, [theme])

  useEffect(() => {
    api.getOrganizations().then(res => {
      const data = res.data as Organization[] | undefined
      if (data) setOrgs(data)
    })
    api.getCurrentUser().then(res => {
      const data = res.data as (User & { require_2fa?: boolean }) | undefined
      if (data) {
        setUser(data)
        if (data.require_2fa && !data.totp_enabled) {
          setMfaRequired(true)
        }
        if (data.role === 'root') {
          api.adminGetAccounts().then(acctRes => {
            const accts = acctRes.data as Account[] | undefined
            if (accts) {
              setAccounts(accts)
              if (data.account_id) setSelectedAccountId(data.account_id)
            }
          })
        }
      }
    })
  }, [])

  const handleOrgChange = (orgId: string) => {
    setCurrentOrgId(orgId)
    setCurrentOrg(orgId)
    queryClient.invalidateQueries()
  }

  const handleAccountChange = async (accountId: string) => {
    setSelectedAccountId(accountId)
    try {
      const res = await api.adminGetAccountOrgs(accountId)
      const accountOrgs = res.data as Organization[] | undefined
      if (accountOrgs && accountOrgs.length > 0) {
        setOrgs(accountOrgs)
        setCurrentOrgId(accountOrgs[0].id)
        setCurrentOrg(accountOrgs[0].id)
      } else {
        setOrgs([])
        // Keep the current org if no orgs in new account
      }
      // Soft reload — navigate to current page to refresh data
      navigate(location.pathname)
    } catch {
      // Don't sign out on error — just log
    }
  }

  const handleLogout = () => {
    clearSession()
    navigate('/login')
  }

  const toggleTheme = () => {
    setTheme(prev => prev === 'dark' ? 'light' : 'dark')
  }

  const currentOrgName = orgs.find(o => o.id === currentOrg)?.name || 'Select org'

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-slate-900">
      <CursorGlow />
      {/* Sidebar */}
      <aside className="fixed inset-y-0 left-0 z-20 w-64 sidebar-gradient dark:sidebar-gradient-dark flex flex-col">
        {/* Logo */}
        <div className="flex h-16 items-center gap-3 px-6 border-b border-white/10">
          <img src="/fibratus.svg" alt="Fibratus" className="h-8 w-8" />
          <h1 className="text-xl font-bold tracking-tight text-white">
            Fibratus <span className="font-normal text-white/70">Fleet</span>
          </h1>
        </div>

        {/* Account switcher (root only) */}
        {isRoot && accounts.length > 0 && (
          <div className="px-3 pt-3 pb-1">
            <label className="block text-[10px] font-medium uppercase tracking-wider text-white/50 mb-1 px-1">Account</label>
            <select
              value={selectedAccountId}
              onChange={(e) => handleAccountChange(e.target.value)}
              className="w-full rounded-lg bg-white/10 border border-white/20 px-3 py-2 text-sm text-white focus:border-white/40 focus:outline-none [&>option]:bg-slate-800 [&>option]:text-white"
            >
              {accounts.map(acct => (
                <option key={acct.id} value={acct.id}>{acct.name}</option>
              ))}
            </select>
          </div>
        )}

        {/* Org switcher */}
        <div className="px-3 py-3 border-b border-white/10">
          <label className="block text-[10px] font-medium uppercase tracking-wider text-white/50 mb-1 px-1">Organization</label>
          <select
            value={currentOrg}
            onChange={(e) => handleOrgChange(e.target.value)}
            className="w-full rounded-lg bg-white/10 border border-white/20 px-3 py-2 text-sm text-white focus:border-white/40 focus:outline-none [&>option]:bg-slate-800 [&>option]:text-white"
          >
            <option value="">All Organizations</option>
            {orgs.map(org => (
              <option key={org.id} value={org.id}>{org.name}</option>
            ))}
            {orgs.length === 0 && currentOrgName !== 'Select org' && (
              <option value="">{currentOrgName}</option>
            )}
          </select>
        </div>

        <nav className="mt-4 px-3 flex-1">
          {navigation.filter(item => permsLoading || !item.perm || hasPermission(item.perm)).map((item) => (
            <Link
              key={item.name}
              to={item.href}
              className={clsx(
                'flex items-center rounded-lg px-3 py-2.5 text-sm font-medium transition-colors mb-1',
                location.pathname === item.href
                  ? 'bg-white/10 text-white'
                  : 'text-white/70 hover:bg-white/5 hover:text-white'
              )}
            >
              {item.name}
            </Link>
          ))}

          {/* Divider before admin links */}
          {(showManagement || showSuperAdmin) && (
            <div className="my-2 border-t border-white/10" />
          )}

          {showManagement && (
            <Link
              to="/management"
              className={clsx(
                'flex items-center rounded-lg px-3 py-2.5 text-sm font-medium transition-colors mb-1',
                location.pathname === '/management'
                  ? 'bg-white/10 text-white'
                  : 'text-white/70 hover:bg-white/5 hover:text-white'
              )}
            >
              <svg className="mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
              </svg>
              Management
            </Link>
          )}
          {showSuperAdmin && (
            <Link
              to="/admin"
              className={clsx(
                'flex items-center rounded-lg px-3 py-2.5 text-sm font-medium transition-colors mb-1',
                location.pathname === '/admin'
                  ? 'bg-purple-500/20 text-purple-200'
                  : 'text-purple-300/80 hover:bg-purple-500/10 hover:text-purple-200'
              )}
            >
              Super Admin
            </Link>
          )}
        </nav>

        {/* Footer: user info, profile, theme toggle, sign out */}
        <div className="px-3 py-4 border-t border-white/10">
          {user && (
            <div className="px-3 mb-3">
              <p className="text-sm font-medium text-white truncate">{user.name || user.email}</p>
              <Link to="/profile" className="text-[11px] text-white/50 hover:text-white/80 transition-colors">
                My Profile
              </Link>
            </div>
          )}
          <div className="flex items-center gap-2">
            <button
              onClick={toggleTheme}
              className="flex items-center justify-center rounded-lg p-2 text-white/50 hover:bg-white/5 hover:text-white transition-colors"
              title={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
            >
              {theme === 'dark' ? (
                <SunIcon className="h-4 w-4" />
              ) : (
                <MoonIcon className="h-4 w-4" />
              )}
            </button>
            <button
              onClick={handleLogout}
              className="flex flex-1 items-center rounded-lg px-3 py-2 text-sm font-medium text-white/50 hover:bg-white/5 hover:text-white transition-colors"
            >
              Sign out
            </button>
          </div>
        </div>
      </aside>

      {/* Main content — z-10 so it sits above the cursor glow canvas */}
      <main className="relative z-10 pl-64">
        <div className="px-8 py-8">
          {children}
        </div>
      </main>
    </div>
  )
}
