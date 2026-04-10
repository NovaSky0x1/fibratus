import { useState, useEffect } from 'react'
import AccountTab from '../components/management/AccountTab'
import OrganizationsTab from '../components/management/OrganizationsTab'
import UsersTab from '../components/management/UsersTab'
import GroupsTab from '../components/management/GroupsTab'
import EnrollmentTab from '../components/management/EnrollmentTab'
import TelemetryTab from '../components/management/TelemetryTab'
import AuditTab from '../components/management/AuditTab'
import { usePermissions } from '../contexts/PermissionContext'

type Tab = 'account' | 'organizations' | 'users' | 'groups' | 'enrollment' | 'telemetry' | 'audit'

const allTabs: { key: Tab; label: string; perm: string }[] = [
  { key: 'account', label: 'Account', perm: 'settings:view' },
  { key: 'organizations', label: 'Organizations', perm: 'organizations:manage' },
  { key: 'users', label: 'Users', perm: 'users:manage' },
  { key: 'groups', label: 'User Groups', perm: 'users:groups' },
  { key: 'enrollment', label: 'Enrollment', perm: 'enrollment:view' },
  { key: 'telemetry', label: 'Telemetry', perm: 'telemetry:view' },
  { key: 'audit', label: 'Audit', perm: 'audit:view' },
]

export default function Management() {
  const { hasPermission, isRoot } = usePermissions()

  const visibleTabs = allTabs.filter(t => hasPermission(t.perm) || isRoot)

  function getTabFromHash(): Tab {
    const hash = window.location.hash.replace('#', '') as Tab
    if (visibleTabs.some(t => t.key === hash)) return hash
    return visibleTabs.length > 0 ? visibleTabs[0].key : 'account'
  }

  const [tab, setTab] = useState<Tab>(getTabFromHash)

  useEffect(() => {
    const handler = () => setTab(getTabFromHash())
    window.addEventListener('hashchange', handler)
    return () => window.removeEventListener('hashchange', handler)
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const handleTabChange = (newTab: Tab) => {
    setTab(newTab)
    window.location.hash = newTab
  }

  if (!hasPermission('page:management') && !isRoot) {
    return (
      <div className="flex items-center justify-center min-h-[60vh]">
        <div className="text-center">
          <div className="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-red-50">
            <svg className="h-8 w-8 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636" />
            </svg>
          </div>
          <h2 className="text-lg font-semibold text-gray-900 dark:text-slate-100">Access Denied</h2>
          <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">You do not have permission to access management.</p>
        </div>
      </div>
    )
  }

  return (
    <div>
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-slate-100">Management</h1>
          <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">Account settings, enrollment, users, and organization management</p>
        </div>
      </div>

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
      {tab === 'audit' && <AuditTab />}
    </div>
  )
}
