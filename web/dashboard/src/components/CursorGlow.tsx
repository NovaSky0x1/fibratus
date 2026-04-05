import { useEffect, useRef } from 'react'

export default function CursorGlow() {
  const canvasRef = useRef<HTMLCanvasElement>(null)

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    let mx = -1000, my = -1000, raf = 0
    const GRID = 140
    const GLOW_RADIUS = 350
    const isDark = () => document.documentElement.classList.contains('dark')

    const resize = () => { canvas.width = window.innerWidth; canvas.height = window.innerHeight }
    resize()
    window.addEventListener('resize', resize)

    const draw = () => {
      const w = canvas.width, h = canvas.height
      ctx.clearRect(0, 0, w, h)

      const dark = isDark()
      const [cr, cg, cb] = dark ? [0, 180, 255] : [60, 130, 220]
      const baseAlpha = dark ? 0.06 : 0.03     // always-visible grid
      const glowAlpha = dark ? 0.35 : 0.15     // bright near cursor

      // Draw full-page grid with cursor-based brightness
      for (let x = 0; x <= w; x += GRID) {
        for (let y = 0; y <= h; y += GRID) {
          // Distance from cursor
          const dx = x - mx, dy = y - my
          const dist = Math.sqrt(dx * dx + dy * dy)

          // Alpha: base everywhere + glow falloff near cursor
          let alpha = baseAlpha
          if (dist < GLOW_RADIUS) {
            const t = 1 - (dist / GLOW_RADIUS)
            alpha += t * t * glowAlpha  // quadratic falloff for soft glow
          }

          ctx.strokeStyle = `rgba(${cr},${cg},${cb},${alpha.toFixed(4)})`
          ctx.lineWidth = 0.8

          // Horizontal line
          if (x + GRID <= w) {
            ctx.beginPath()
            ctx.moveTo(x, y)
            ctx.lineTo(x + GRID, y)
            ctx.stroke()
          }

          // Vertical line
          if (y + GRID <= h) {
            ctx.beginPath()
            ctx.moveTo(x, y)
            ctx.lineTo(x, y + GRID)
            ctx.stroke()
          }
        }
      }

      raf = requestAnimationFrame(draw)
    }

    const onMove = (e: MouseEvent) => { mx = e.clientX; my = e.clientY }
    const onLeave = () => { mx = -1000; my = -1000 }

    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseleave', onLeave)
    raf = requestAnimationFrame(draw)

    return () => {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseleave', onLeave)
      window.removeEventListener('resize', resize)
      cancelAnimationFrame(raf)
    }
  }, [])

  return (
    <canvas
      ref={canvasRef}
      className="pointer-events-none fixed inset-0"
      style={{ zIndex: 1 }}
      aria-hidden="true"
    />
  )
}
