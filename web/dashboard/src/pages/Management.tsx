import { useState, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type User } from '../lib/api'
import AccountTab from '../components/management/AccountTab'
import OrganizationsTab from '../components/management/OrganizationsTab'
import UsersTab from '../components/management/UsersTab'
import GroupsTab from '../components/management/GroupsTab'
import EnrollmentTab from '../components/management/EnrollmentTab'
import TelemetryTab from '../components/management/TelemetryTab'
import DatabaseTab from '../components/management/DatabaseTab'
import AuditTab from '../components/management/AuditTab'

type Tab = 'account' | 'organizations' | 'users' | 'groups' | 'enrollment' | 'telemetry' | 'database' | 'audit'

const allTabs: { key: Tab; label: string; rootOnly?: boolean }[] = [
  { key: 'account', label: 'Account' },
  { key: 'organizations', label: 'Organizations' },
  { key: 'users', label: 'Users' },
  { key: 'groups', label: 'User Groups' },
  { key: 'enrollment', label: 'Enrollment' },
  { key: 'telemetry', label: 'Telemetry' },
  { key: 'database', label: 'Database', rootOnly: true },
  { key: 'audit', label: 'Audit' },
]

function getTabFromHash(): Tab {
  const hash = window.location.hash.replace('#', '') as Tab
  if (allTabs.some(t => t.key === hash)) return hash
  return 'account'
}

export default function Management() {
  const [tab, setTab] = useState<Tab>(getTabFromHash)

  const { data: currentUserData } = useQuery({
    queryKey: ['current-user'],
    queryFn: () => api.getCurrentUser(),
  })
  const currentUser = currentUserData?.data as User | undefined
  const isAdmin = currentUser?.role === 'admin' || currentUser?.role === 'root'
  const isRoot = currentUser?.role === 'root'

  // Sync tab with URL hash
  useEffect(() => {
    const handler = () => setTab(getTabFromHash())
    window.addEventListener('hashchange', handler)
    return () => window.removeEventListener('hashchange', handler)
  }, [])

  const handleTabChange = (newTab: Tab) => {
    setTab(newTab)
    window.location.hash = newTab
  }

  if (currentUser && !isAdmin) {
    return (
      <div className="flex items-center justify-center min-h-[60vh]">
        <div className="text-center">
          <div className="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-red-50">
            <svg className="h-8 w-8 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636" />
            </svg>
          </div>
          <h2 className="text-lg font-semibold text-gray-900 dark:text-slate-100">Access Denied</h2>
          <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">You need an admin or root role to access management.</p>
        </div>
      </div>
    )
  }

  const visibleTabs = allTabs.filter(t => !t.rootOnly || isRoot)

  return (
    <div>
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-slate-100">Management</h1>
          <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">Account settings, enrollment, users, organizations, and system management</p>
        </div>
      </div>

      {/* Tabs */}
      <div className="mt-6 flex gap-1 border-b border-gray-200 dark:border-slate-700 overflow-x-auto">
        {visibleTabs.map(t => (
          <button
            key={t.key}
            onClick={() => handleTabChange(t.key)}
            className={
              'px-4 py-2.5 text-sm font-medium border-b-2 transition-colors -mb-px whitespace-nowrap ' +
              (tab === t.key
                ? 'border-fibratus-600 text-fibratus-600'
                : 'border-transparent text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-300 hover:border-gray-300 dark:hover:border-slate-600')
            }
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === 'account' && <AccountTab />}
      {tab === 'organizations' && <OrganizationsTab />}
      {tab === 'users' && <UsersTab />}
      {tab === 'groups' && <GroupsTab />}
      {tab === 'enrollment' && <EnrollmentTab />}
      {tab === 'telemetry' && <TelemetryTab />}
      {tab === 'database' && isRoot && <DatabaseTab />}
      {tab === 'audit' && <AuditTab />}
    </div>
  )
}
