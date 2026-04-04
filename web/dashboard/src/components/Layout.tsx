import { useEffect, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { clsx } from 'clsx'
import { api, clearSession, getCurrentOrgId, setCurrentOrgId, type Account, type Organization, type User } from '../lib/api'

const navigation = [
  { name: 'Overview', href: '/' },
  { name: 'Agents', href: '/agents' },
  { name: 'Detections', href: '/detections' },
  { name: 'Events', href: '/events' },
  { name: 'Rules', href: '/rules' },
  { name: 'Macros', href: '/macros' },
  { name: 'Audit Log', href: '/audit-log' },
  { name: 'Users', href: '/users' },
  { name: 'Settings', href: '/settings' },
]

export default function Layout({ children }: { children: React.ReactNode }) {
  const location = useLocation()
  const navigate = useNavigate()
  const [orgs, setOrgs] = useState<Organization[]>([])
  const [currentOrg, setCurrentOrg] = useState(getCurrentOrgId())
  const [user, setUser] = useState<User | null>(null)
  const [accounts, setAccounts] = useState<Account[]>([])
  const [selectedAccountId, setSelectedAccountId] = useState<string>('')

  const isRoot = user?.role === 'root'

  useEffect(() => {
    api.getOrganizations().then(res => {
      const data = res.data as Organization[] | undefined
      if (data) setOrgs(data)
    })
    api.getCurrentUser().then(res => {
      const data = res.data as User | undefined
      if (data) {
        setUser(data)
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
    if (orgId === '') {
      setCurrentOrgId('')
      setCurrentOrg('')
      window.location.reload()
      return
    }
    setCurrentOrgId(orgId)
    setCurrentOrg(orgId)
    window.location.reload()
  }

  const handleAccountChange = (accountId: string) => {
    setSelectedAccountId(accountId)
    api.adminSwitchAccount(accountId).then(() => {
      api.adminGetAccountOrgs(accountId).then(res => {
        const accountOrgs = res.data as Organization[] | undefined
        if (accountOrgs && accountOrgs.length > 0) {
          setOrgs(accountOrgs)
          setCurrentOrgId(accountOrgs[0].id)
          setCurrentOrg(accountOrgs[0].id)
        } else {
          setOrgs([])
          setCurrentOrgId('')
          setCurrentOrg('')
        }
        window.location.reload()
      })
    })
  }

  const handleLogout = () => {
    clearSession()
    navigate('/login')
  }

  const currentOrgName = orgs.find(o => o.id === currentOrg)?.name || 'Select org'

  return (
    <div className="min-h-screen bg-gray-50">
      {/* Sidebar */}
      <aside className="fixed inset-y-0 left-0 w-64 bg-gray-900 text-white flex flex-col">
        <div className="flex h-16 items-center px-6 border-b border-gray-800">
          <h1 className="text-xl font-bold tracking-tight">
            <span className="text-fibratus-400">Fibratus</span> Fleet
          </h1>
        </div>

        {/* Account switcher (root only) */}
        {isRoot && accounts.length > 0 && (
          <div className="px-3 pt-3 pb-1">
            <label className="block text-[10px] font-medium uppercase tracking-wider text-gray-500 mb-1 px-1">Account</label>
            <select
              value={selectedAccountId}
              onChange={(e) => handleAccountChange(e.target.value)}
              className="w-full rounded-lg bg-gray-800 border border-purple-300/30 px-3 py-2 text-sm text-white focus:border-purple-400 focus:outline-none"
            >
              {accounts.map(acct => (
                <option key={acct.id} value={acct.id}>{acct.name}</option>
              ))}
            </select>
          </div>
        )}

        {/* Org switcher */}
        <div className="px-3 py-3 border-b border-gray-800">
          <label className="block text-[10px] font-medium uppercase tracking-wider text-gray-500 mb-1 px-1">Organization</label>
          <select
            value={currentOrg}
            onChange={(e) => handleOrgChange(e.target.value)}
            className="w-full rounded-lg bg-gray-800 border border-gray-700 px-3 py-2 text-sm text-white focus:border-fibratus-500 focus:outline-none"
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
          {navigation.map((item) => (
            <Link
              key={item.name}
              to={item.href}
              className={clsx(
                'flex items-center rounded-lg px-3 py-2.5 text-sm font-medium transition-colors mb-1',
                location.pathname === item.href
                  ? 'bg-gray-800 text-white'
                  : 'text-gray-400 hover:bg-gray-800 hover:text-white'
              )}
            >
              {item.name}
            </Link>
          ))}
          {isRoot && (
            <Link
              to="/admin"
              className={clsx(
                'flex items-center rounded-lg px-3 py-2.5 text-sm font-medium transition-colors mb-1',
                location.pathname === '/admin'
                  ? 'bg-purple-900/50 text-purple-200'
                  : 'text-purple-400 hover:bg-purple-900/30 hover:text-purple-200'
              )}
            >
              Admin
            </Link>
          )}
        </nav>

        {/* User info + Logout */}
        <div className="px-3 py-4 border-t border-gray-800">
          {user && (
            <div className="px-3 mb-3">
              <p className="text-sm font-medium text-gray-200 truncate">{user.name || user.email}</p>
              <p className="text-xs text-gray-500 capitalize">{user.role}</p>
            </div>
          )}
          <button
            onClick={handleLogout}
            className="flex w-full items-center rounded-lg px-3 py-2.5 text-sm font-medium text-gray-400 hover:bg-gray-800 hover:text-white transition-colors"
          >
            Sign out
          </button>
        </div>
      </aside>

      {/* Main content */}
      <main className="pl-64">
        <div className="px-8 py-8">
          {children}
        </div>
      </main>
    </div>
  )
}
