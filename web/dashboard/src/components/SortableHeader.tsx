import type { SortState } from '../hooks/useTableSort'

interface Props {
  label: string
  sortKey: string
  sort: SortState
  onSort: (key: string) => void
  className?: string
}

export default function SortableHeader({ label, sortKey, sort, onSort, className = '' }: Props) {
  const isActive = sort.key === sortKey
  return (
    <th
      className={`px-6 py-3 font-medium text-gray-500 dark:text-slate-400 th-sort ${isActive ? 'th-sort-active' : ''} ${className}`}
      onClick={() => onSort(sortKey)}
    >
      <div className="flex items-center gap-1">
        {label}
        <span className="inline-flex flex-col text-[8px] leading-none">
          <span className={isActive && sort.dir === 'asc' ? 'text-fibratus-500' : 'opacity-30'}>&#9650;</span>
          <span className={isActive && sort.dir === 'desc' ? 'text-fibratus-500' : 'opacity-30'}>&#9660;</span>
        </span>
      </div>
    </th>
  )
}
