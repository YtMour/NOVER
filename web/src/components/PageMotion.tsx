import { useLayoutEffect, useRef, type ReactNode } from 'react'
import { gsap } from 'gsap'

export function PageMotion({ children }: { children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    if (!reduced && ref.current) {
      const tween = gsap.fromTo(ref.current, { autoAlpha: 0, y: 7 }, { autoAlpha: 1, y: 0, duration: 0.38, ease: 'power2.out' })
      return () => { tween.kill() }
    }
  }, [])

  return <div ref={ref} className="page-motion">{children}</div>
}
