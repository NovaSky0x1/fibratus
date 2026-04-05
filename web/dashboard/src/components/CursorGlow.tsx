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
    const GRID = 50
    const GLOW_RADIUS = 250
    const RING_INNER = 80
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

      const dark = isDark()
      const gridAlpha = dark ? 0.06 : 0.04
      const glowColor = dark ? 'rgba(96, 206, 253,' : 'rgba(76, 110, 245,'

      // Draw grid lines only where the cursor glow reaches
      // This creates the "spotlight on grid" effect
      const x0 = Math.max(0, mx - GLOW_RADIUS - GRID)
      const x1 = Math.min(w, mx + GLOW_RADIUS + GRID)
      const y0 = Math.max(0, my - GLOW_RADIUS - GRID)
      const y1 = Math.min(h, my + GLOW_RADIUS + GRID)

      // Draw grid near cursor with distance-based alpha
      for (let x = Math.floor(x0 / GRID) * GRID; x <= x1; x += GRID) {
        for (let y = Math.floor(y0 / GRID) * GRID; y <= y1; y += GRID) {
          const dx = x - mx
          const dy = y - my
          const dist = Math.sqrt(dx * dx + dy * dy)

          if (dist > GLOW_RADIUS) continue

          // Ring falloff: bright at RING_INNER..GLOW_RADIUS*0.6, fades at edges
          let alpha: number
          if (dist < RING_INNER) {
            alpha = gridAlpha * 0.3 // dim inside the ring
          } else {
            const t = (dist - RING_INNER) / (GLOW_RADIUS - RING_INNER)
            alpha = gridAlpha * (1 - t) * 2.5 // bright ring, fading outward
          }

          ctx.strokeStyle = `${glowColor}${Math.min(alpha, 0.2).toFixed(3)})`
          ctx.lineWidth = 0.5

          // Horizontal line segment
          ctx.beginPath()
          ctx.moveTo(x, y)
          ctx.lineTo(x + GRID, y)
          ctx.stroke()

          // Vertical line segment
          ctx.beginPath()
          ctx.moveTo(x, y)
          ctx.lineTo(x, y + GRID)
          ctx.stroke()
        }
      }

      // Draw the ring glow itself
      const ringGradient = ctx.createRadialGradient(mx, my, RING_INNER, mx, my, GLOW_RADIUS)
      ringGradient.addColorStop(0, `${glowColor}0)`)
      ringGradient.addColorStop(0.3, `${glowColor}${dark ? '0.07' : '0.04'})`)
      ringGradient.addColorStop(0.6, `${glowColor}${dark ? '0.03' : '0.02'})`)
      ringGradient.addColorStop(1, `${glowColor}0)`)

      ctx.fillStyle = ringGradient
      ctx.beginPath()
      ctx.arc(mx, my, GLOW_RADIUS, 0, Math.PI * 2)
      ctx.fill()
    }

    const loop = () => {
      draw()
      raf = requestAnimationFrame(loop)
    }

    const onMove = (e: MouseEvent) => {
      mx = e.clientX
      my = e.clientY
    }

    const onLeave = () => {
      mx = -1000
      my = -1000
    }

    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseleave', onLeave)
    raf = requestAnimationFrame(loop)

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
      style={{ zIndex: 0 }}
      aria-hidden="true"
    />
  )
}
