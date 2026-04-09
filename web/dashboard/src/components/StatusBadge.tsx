import { clsx } from 'clsx'

const statusStyles: Record<string, { dot: string; text: string; bg: string }> = {
  online: { dot: 'bg-emerald-500', text: 'text-emerald-700 dark:text-emerald-400', bg: 'bg-emerald-50 dark:bg-emerald-900/20' },
  offline: { dot: 'bg-gray-400 dark:bg-slate-500', text: 'text-gray-600 dark:text-slate-400', bg: 'bg-gray-50 dark:bg-slate-800' },
  stale: { dot: 'bg-amber-500', text: 'text-amber-700 dark:text-amber-400', bg: 'bg-amber-50 dark:bg-amber-900/20' },
}

export default function StatusBadge({ status }: { status: string }) {
  const style = statusStyles[status] || statusStyles.offline

  return (
    <span className={clsx('inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium', style.bg, style.text)}>
      <span className={clsx('h-1.5 w-1.5 rounded-full', style.dot)} />
      {status}
    </span>
  )
}
