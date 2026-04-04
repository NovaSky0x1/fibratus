import { clsx } from 'clsx'

interface StatCardProps {
  title: string
  value: number | string
  subtitle?: string
  variant?: 'default' | 'success' | 'danger' | 'warning'
}

export default function StatCard({ title, value, subtitle, variant = 'default' }: StatCardProps) {
  return (
    <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50">
      <p className="text-sm font-medium text-gray-500 dark:text-slate-400">{title}</p>
      <p
        className={clsx(
          'mt-2 text-3xl font-bold tracking-tight',
          variant === 'success' && 'text-emerald-600',
          variant === 'danger' && 'text-red-600',
          variant === 'warning' && 'text-amber-600',
          variant === 'default' && 'text-gray-900 dark:text-slate-100'
        )}
      >
        {value}
      </p>
      {subtitle && (
        <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">{subtitle}</p>
      )}
    </div>
  )
}
