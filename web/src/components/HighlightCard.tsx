import { Link } from 'react-router-dom'
import type { Highlight } from '../api/types'

export function HighlightList({ items, personId }: { items: Highlight[]; personId: string }) {
  if (!items.length) return <div className="muted small">Nothing notable for this period yet.</div>
  return (
    <div>
      {items.map((h, i) => (
        <div className="highlight" key={i}>
          <span className={`dot ${h.tone}`} />
          <div style={{ minWidth: 0 }}>
            <div className="title">{h.link ? <Link to={`/p/${personId}/${h.link}`}>{h.title}</Link> : h.title}</div>
            <div className="detail">{h.detail}</div>
          </div>
        </div>
      ))}
    </div>
  )
}
