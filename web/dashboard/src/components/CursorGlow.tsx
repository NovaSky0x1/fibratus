import { useEffect, useRef } from 'react'

export default function CursorGlow() {
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const el = ref.current
    if (!el) return

    let raf = 0
    let mx = -500
    let my = -500

    const move = (e: MouseEvent) => {
      mx = e.clientX
      my = e.clientY
      if (!raf) {
        raf = requestAnimationFrame(() => {
          el.style.left = `${mx}px`
          el.style.top = `${my}px`
          el.style.opacity = '1'
          raf = 0
        })
      }
    }

    const leave = () => {
      el.style.opacity = '0'
    }

    document.addEventListener('mousemove', move)
    document.addEventListener('mouseleave', leave)
    return () => {
      document.removeEventListener('mousemove', move)
      document.removeEventListener('mouseleave', leave)
      if (raf) cancelAnimationFrame(raf)
    }
  }, [])

  return <div ref={ref} className="cursor-glow" style={{ opacity: 0 }} />
}
