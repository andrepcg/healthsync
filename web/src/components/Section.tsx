import type { ReactNode } from 'react'

export function Section({ title, hint, right, children, className }: { title: ReactNode; hint?: ReactNode; right?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={`card ${className ?? ''}`}>
      <div className="card-head">
        <h2>
          {title}
          {hint && <span className="hint">{hint}</span>}
        </h2>
        {right}
      </div>
      {children}
    </section>
  )
}

export function EmptyState({ icon = '📭', title, children }: { icon?: string; title: string; children?: ReactNode }) {
  return (
    <div className="empty">
      <div className="big">{icon}</div>
      <div style={{ fontWeight: 600, color: 'var(--text)' }}>{title}</div>
      {children && <div style={{ marginTop: 6 }}>{children}</div>}
    </div>
  )
}

export function Loading({ lines = 3 }: { lines?: number }) {
  return (
    <div className="stack" style={{ gap: 8 }}>
      {Array.from({ length: lines }).map((_, i) => (
        <div key={i} className="skeleton" style={{ width: `${90 - i * 15}%` }} />
      ))}
    </div>
  )
}

export function ErrorNote({ error }: { error: unknown }) {
  const msg = error instanceof Error ? error.message : String(error)
  return <div className="err small">Could not load: {msg}</div>
}
