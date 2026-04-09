interface SlidePanelProps {
  open: boolean
  title: string
  onClose: () => void
  children: React.ReactNode
  wide?: boolean
}

export default function SlidePanel({ open, title, onClose, children, wide }: SlidePanelProps) {
  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/20 dark:bg-black/50" onClick={onClose}>
      <div
        className={`h-full overflow-y-auto bg-white dark:bg-slate-800 shadow-xl dark:shadow-slate-900/50 ${wide ? 'w-full max-w-[calc(100vw-16rem)]' : 'w-full max-w-2xl'}`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="sticky top-0 z-10 flex items-center justify-between border-b border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 px-6 py-4">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-slate-100">{title}</h2>
          <button
            onClick={onClose}
            className="rounded-lg p-1 text-gray-400 dark:text-slate-500 hover:bg-gray-100 dark:hover:bg-slate-700 hover:text-gray-600 dark:hover:text-slate-300"
          >
            <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div className="p-6">
          {children}
        </div>
      </div>
    </div>
  )
}
