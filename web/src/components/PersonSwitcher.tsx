import { useEffect, useRef, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { usePeople } from '../api/hooks'
import type { Person } from '../api/types'

export function Avatar({ p, size }: { p: Pick<Person, 'emoji' | 'color'>; size?: 'lg' }) {
  return (
    <span className={`avatar ${size ?? ''}`} style={{ background: p.color + '33' }}>
      {p.emoji || '🙂'}
    </span>
  )
}

export function PersonSwitcher({ current }: { current?: Person }) {
  const { data: people } = usePeople()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const nav = useNavigate()
  const loc = useLocation()

  useEffect(() => {
    if (!open) return
    const close = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [open])

  const pick = (p: Person) => {
    setOpen(false)
    try {
      localStorage.setItem('healthsync.person', p.id)
    } catch {
      /* ignore */
    }
    // Keep the current page and period when switching person.
    const rest = loc.pathname.replace(/^\/p\/[^/]+/, '') || '/overview'
    nav(`/p/${p.id}${rest}${loc.search}`)
  }

  return (
    <div className="switcher" ref={ref}>
      <button className="switcher-btn" onClick={() => setOpen((o) => !o)} aria-haspopup="menu" aria-expanded={open}>
        {current ? (
          <>
            <Avatar p={current} />
            <span>{current.name}</span>
          </>
        ) : (
          <span style={{ padding: '4px 6px' }}>Choose person</span>
        )}
        <span className="faint">▾</span>
      </button>
      {open && (
        <div className="menu" role="menu">
          {(people ?? []).map((p) => (
            <button key={p.id} className={`menu-item ${p.id === current?.id ? 'active' : ''}`} onClick={() => pick(p)}>
              <Avatar p={p} />
              <span style={{ flex: 1 }}>{p.name}</span>
              {!p.has_data && <span className="faint small">no data</span>}
            </button>
          ))}
          <div className="menu-sep" />
          <Link className="menu-item" to="/people" onClick={() => setOpen(false)}>
            <span className="avatar">⚙️</span>
            <span>Manage people…</span>
          </Link>
        </div>
      )}
    </div>
  )
}
