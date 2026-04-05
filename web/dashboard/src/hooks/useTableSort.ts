import { useState, useMemo } from 'react'

export type SortDir = 'asc' | 'desc'

export interface SortState {
  key: string
  dir: SortDir
}

export function useTableSort<T>(data: T[], defaultKey = '', defaultDir: SortDir = 'desc') {
  const [sort, setSort] = useState<SortState>({ key: defaultKey, dir: defaultDir })

  const toggleSort = (key: string) => {
    setSort(prev => {
      if (prev.key === key) {
        return { key, dir: prev.dir === 'asc' ? 'desc' : 'asc' }
      }
      return { key, dir: 'desc' }
    })
  }

  const sorted = useMemo(() => {
    if (!sort.key) return data
    return [...data].sort((a, b) => {
      const av = (a as Record<string, unknown>)[sort.key]
      const bv = (b as Record<string, unknown>)[sort.key]

      // Handle nulls
      if (av == null && bv == null) return 0
      if (av == null) return 1
      if (bv == null) return -1

      // Numbers
      if (typeof av === 'number' && typeof bv === 'number') {
        return sort.dir === 'asc' ? av - bv : bv - av
      }

      // Booleans
      if (typeof av === 'boolean' && typeof bv === 'boolean') {
        return sort.dir === 'asc' ? (av ? 1 : -1) : (av ? -1 : 1)
      }

      // Strings (case-insensitive)
      const as = String(av).toLowerCase()
      const bs = String(bv).toLowerCase()
      const cmp = as.localeCompare(bs)
      return sort.dir === 'asc' ? cmp : -cmp
    })
  }, [data, sort])

  return { sorted, sort, toggleSort }
}
