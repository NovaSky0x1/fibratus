import { useEffect, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { clsx } from 'clsx'
import { api, clearSession, getCurrentOrgId, setCurrentOrgId, type Organization } from '../lib/api'

const navigation = [
  { name: 'Overview', href: '/' },
  { name: 'Agents', href: '/agents' },
  { name: 'Detections', href: '/detections' },
]

export default function Layout({ children }: { children: React.ReactNode }) {
  const location = useLocation()
  const navigate = useNavigate()
  const [orgs, setOrgs] = useState<Organization[]>([])
  const [currentOrg, setCurrentOrg] = useState(getCurrentOrgId())

  useEffect(() => {
    api.getOrganizations().then(res => {
      const data = res.data as Organization[] | undefined
      if (data) setOrgs(data)
    })
  }, [])

  const handleOrgChange = (orgId: string) => {
    setCurrentOrgId(orgId)
    setCurrentOrg(orgId)
    window.location.reload() // Refresh data for new org
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

        {/* Org switcher */}
        <div className="px-3 py-3 border-b border-gray-800">
          <select
            value={currentOrg}
            onChange={(e) => handleOrgChange(e.target.value)}
            className="w-full rounded-lg bg-gray-800 border border-gray-700 px-3 py-2 text-sm text-white focus:border-fibratus-500 focus:outline-none"
          >
            {orgs.map(org => (
              <option key={org.id} value={org.id}>{org.name}</option>
            ))}
            {orgs.length === 0 && (
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
        </nav>

        {/* Logout */}
        <div className="px-3 py-4 border-t border-gray-800">
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
