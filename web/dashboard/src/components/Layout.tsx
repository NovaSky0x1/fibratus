import { Link, useLocation } from 'react-router-dom'
import { clsx } from 'clsx'

const navigation = [
  { name: 'Overview', href: '/' },
  { name: 'Agents', href: '/agents' },
  { name: 'Detections', href: '/detections' },
]

export default function Layout({ children }: { children: React.ReactNode }) {
  const location = useLocation()

  return (
    <div className="min-h-screen bg-gray-50">
      {/* Sidebar */}
      <aside className="fixed inset-y-0 left-0 w-64 bg-gray-900 text-white">
        <div className="flex h-16 items-center px-6">
          <h1 className="text-xl font-bold tracking-tight">
            <span className="text-fibratus-400">Fibratus</span> Fleet
          </h1>
        </div>
        <nav className="mt-6 px-3">
          {navigation.map((item) => (
            <Link
              key={item.name}
              to={item.href}
              className={clsx(
                'flex items-center rounded-lg px-3 py-2.5 text-sm font-medium transition-colors',
                location.pathname === item.href
                  ? 'bg-gray-800 text-white'
                  : 'text-gray-400 hover:bg-gray-800 hover:text-white'
              )}
            >
              {item.name}
            </Link>
          ))}
        </nav>
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
