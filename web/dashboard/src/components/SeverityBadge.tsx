import { clsx } from 'clsx'

const severityStyles: Record<string, string> = {
  critical: 'bg-red-100 text-red-800 border-red-200',
  high: 'bg-orange-100 text-orange-800 border-orange-200',
  medium: 'bg-amber-100 text-amber-800 border-amber-200',
  low: 'bg-blue-100 text-blue-800 border-blue-200',
}

export default function SeverityBadge({ severity }: { severity: string }) {
  return (
    <span
      className={clsx(
        'inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold capitalize',
        severityStyles[severity] || 'bg-gray-100 text-gray-800 border-gray-200'
      )}
    >
      {severity}
    </span>
  )
}
