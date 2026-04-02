import { clsx } from 'clsx'

interface StatCardProps {
  title: string
  value: number | string
  subtitle?: string
  variant?: 'default' | 'success' | 'danger' | 'warning'
}

export default function StatCard({ title, value, subtitle, variant = 'default' }: StatCardProps) {
  return (
    <div className="rounded-xl border border-gray-200 bg-white p-6 shadow-sm">
      <p className="text-sm font-medium text-gray-500">{title}</p>
      <p
        className={clsx(
          'mt-2 text-3xl font-bold tracking-tight',
          variant === 'success' && 'text-emerald-600',
          variant === 'danger' && 'text-red-600',
          variant === 'warning' && 'text-amber-600',
          variant === 'default' && 'text-gray-900'
        )}
      >
        {value}
      </p>
      {subtitle && (
        <p className="mt-1 text-sm text-gray-500">{subtitle}</p>
      )}
    </div>
  )
}
