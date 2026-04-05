import { useEffect, useRef } from 'react'

export default function CursorGlow() {
  const canvasRef = useRef<HTMLCanvasElement>(null)

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return

    const ctx = canvas.getContext('2d')
    if (!ctx) return

    let mx = -1000
    let my = -1000
    let raf = 0
    const GRID = 40
    const RADIUS = 160
    const RING_WIDTH = 40
    const isDark = () => document.documentElement.classList.contains('dark')

    const resize = () => {
      canvas.width = window.innerWidth
      canvas.height = window.innerHeight
    }
    resize()
    window.addEventListener('resize', resize)

    const draw = () => {
      const w = canvas.width
      const h = canvas.height
      ctx.clearRect(0, 0, w, h)

      if (mx < -500) { raf = requestAnimationFrame(draw); return }

      const dark = isDark()
      const baseColor = dark ? [96, 206, 253] : [76, 110, 245]

      // Only draw grid in a tight area around cursor
      const extent = RADIUS + GRID
      const x0 = Math.floor((mx - extent) / GRID) * GRID
      const x1 = mx + extent
      const y0 = Math.floor((my - extent) / GRID) * GRID
      const y1 = my + extent

      for (let gx = x0; gx <= x1; gx += GRID) {
        for (let gy = y0; gy <= y1; gy += GRID) {
          const dx = gx - mx
          const dy = gy - my
          const dist = Math.sqrt(dx * dx + dy * dy)
          if (dist > RADIUS) continue

          // Hollow ring: fade in from center, peak at ring, fade out
          const ringCenter = RADIUS - RING_WIDTH
          let alpha: number
          if (dist < ringCenter - 20) {
            alpha = 0 // invisible inside the ring
          } else if (dist < ringCenter) {
            alpha = ((dist - (ringCenter - 20)) / 20) * 0.12 // fade in
          } else if (dist < RADIUS - 10) {
            alpha = 0.12 // peak brightness in ring band
          } else {
            alpha = ((RADIUS - dist) / 10) * 0.12 // fade out at edge
          }

          if (alpha <= 0) continue

          const [r, g, b] = baseColor
          ctx.strokeStyle = `rgba(${r},${g},${b},${alpha.toFixed(3)})`
          ctx.lineWidth = 0.5

          // Draw grid cell edges
          ctx.beginPath()
          ctx.moveTo(gx, gy)
          ctx.lineTo(Math.min(gx + GRID, x1), gy)
          ctx.stroke()

          ctx.beginPath()
          ctx.moveTo(gx, gy)
          ctx.lineTo(gx, Math.min(gy + GRID, y1))
          ctx.stroke()
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
