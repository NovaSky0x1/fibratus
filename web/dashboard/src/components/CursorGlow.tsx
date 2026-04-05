import { useEffect, useRef } from 'react'

export default function CursorGlow() {
  const canvasRef = useRef<HTMLCanvasElement>(null)

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    let mx = -1000, my = -1000, raf = 0
    const GRID = 32
    const RADIUS = 100     // much tighter
    const RING_R = 75      // ring at this radius
    const RING_W = 12      // thin ring
    const isDark = () => document.documentElement.classList.contains('dark')

    const resize = () => { canvas.width = window.innerWidth; canvas.height = window.innerHeight }
    resize()
    window.addEventListener('resize', resize)

    const draw = () => {
      ctx.clearRect(0, 0, canvas.width, canvas.height)
      if (mx < -500) { raf = requestAnimationFrame(draw); return }

      const dark = isDark()
      const [cr, cg, cb] = dark ? [96, 206, 253] : [76, 160, 245]

      // Draw grid near cursor
      const ext = RADIUS + GRID
      const gx0 = Math.floor((mx - ext) / GRID) * GRID
      const gy0 = Math.floor((my - ext) / GRID) * GRID

      for (let gx = gx0; gx <= mx + ext; gx += GRID) {
        for (let gy = gy0; gy <= my + ext; gy += GRID) {
          const dx = gx - mx, dy = gy - my
          const dist = Math.sqrt(dx * dx + dy * dy)
          if (dist > RADIUS) continue

          // Ring-shaped falloff
          const distFromRing = Math.abs(dist - RING_R)
          let alpha: number
          if (distFromRing < RING_W) {
            alpha = (1 - distFromRing / RING_W) * (dark ? 0.18 : 0.1)
          } else {
            alpha = Math.max(0, (1 - dist / RADIUS)) * (dark ? 0.04 : 0.02)
          }
          if (alpha <= 0.001) continue

          ctx.strokeStyle = `rgba(${cr},${cg},${cb},${alpha.toFixed(3)})`
          ctx.lineWidth = 0.5

          ctx.beginPath()
          ctx.moveTo(gx, gy); ctx.lineTo(gx + GRID, gy)
          ctx.stroke()
          ctx.beginPath()
          ctx.moveTo(gx, gy); ctx.lineTo(gx, gy + GRID)
          ctx.stroke()
        }
      }

      // Soft cyan ring glow
      ctx.beginPath()
      ctx.arc(mx, my, RING_R, 0, Math.PI * 2)
      ctx.strokeStyle = `rgba(${cr},${cg},${cb},${dark ? 0.15 : 0.08})`
      ctx.lineWidth = RING_W * 0.6
      ctx.stroke()

      // Inner softer glow
      ctx.beginPath()
      ctx.arc(mx, my, RING_R, 0, Math.PI * 2)
      ctx.strokeStyle = `rgba(${cr},${cg},${cb},${dark ? 0.06 : 0.03})`
      ctx.lineWidth = RING_W * 2
      ctx.stroke()

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
