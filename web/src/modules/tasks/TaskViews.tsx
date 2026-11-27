import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Link } from 'react-router'
import Count from '../../components/Count.tsx'
import type { View } from './views.ts'

type Item = { view: View; label: string; to: string; count: number }
type Props = { current: View; items: Item[] }

export default function TaskViews({ current, items }: Props) {
  const bar = useRef<HTMLElement>(null)
  const more = useRef<HTMLDetailsElement>(null)
  const focusView = useRef(false)
  const [slots, setSlots] = useState(items.length)
  const active = items.findIndex((item) => item.view === current)
  const visible = items.slice(0, slots)
  if (active >= slots) visible[slots - 1] = items[active]

  useLayoutEffect(() => {
    const nav = bar.current!
    const menu = more.current!
    function fit() {
      const width = nav.getBoundingClientRect().width
      if (!width) return
      // Measure the labels with their current counts before distributing the space equally.
      nav.classList.add('measuring')
      const widths = Array.from(nav.querySelectorAll(':scope > a'), (a) => a.getBoundingClientRect().width)
      const menuWidth = menu.getBoundingClientRect().width
      const gap = parseFloat(getComputedStyle(nav).columnGap) || 0
      nav.classList.remove('measuring')

      const priority = [widths[active], ...widths.filter((_, i) => i !== active)]
      while (priority.length > 1) {
        const n = priority.length
        const reserved = n === widths.length ? 0 : menuWidth + gap
        if (n * Math.max(...priority) + (n - 1) * gap + reserved <= width) break
        priority.pop()
      }
      if (priority.length === widths.length) menu.open = false
      setSlots(priority.length)
    }
    fit()
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(fit)
    observer?.observe(nav)
    document.fonts?.addEventListener('loadingdone', fit)
    return () => {
      observer?.disconnect()
      document.fonts?.removeEventListener('loadingdone', fit)
    }
  }, [active, items])

  useLayoutEffect(() => {
    more.current!.open = false
    if (focusView.current) bar.current?.querySelector<HTMLAnchorElement>(':scope > [aria-current]')?.focus()
    focusView.current = false
  }, [current])

  useEffect(() => {
    const menu = more.current!
    const outside = (e: PointerEvent) => {
      if (menu.open && !menu.contains(e.target as Node)) menu.open = false
    }
    document.addEventListener('pointerdown', outside)
    return () => document.removeEventListener('pointerdown', outside)
  }, [])

  const link = (item: Item, inMenu = false) => (
    <Link
      key={item.view}
      to={item.to}
      hidden={!inMenu && !visible.includes(item)}
      aria-current={item.view === current ? 'page' : undefined}
      aria-label={item.count ? `${item.label}, ${item.count} ${item.count === 1 ? 'task' : 'tasks'}` : undefined}
      onClick={inMenu ? (e) => { focusView.current = !e.ctrlKey && !e.metaKey && !e.shiftKey && !e.altKey } : undefined}
    >
      {item.label}
      <Count>{item.count}</Count>
    </Link>
  )
  return (
    <nav className="views" ref={bar} aria-label="Task views">
      {items.map((item) => link(item))}
      <details
        ref={more}
        hidden={slots >= items.length}
        onBlur={(e) => { if (e.relatedTarget && !e.currentTarget.contains(e.relatedTarget)) e.currentTarget.open = false }}
        onKeyDown={(e) => {
          if (e.key === 'Escape' && e.currentTarget.open) {
            e.currentTarget.open = false
            e.currentTarget.querySelector('summary')?.focus()
          }
        }}
      >
        <summary>More</summary>
        <div className="view-menu">{items.filter((item) => !visible.includes(item)).map((item) => link(item, true))}</div>
      </details>
    </nav>
  )
}
