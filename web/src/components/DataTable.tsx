import { fmtNum } from '../format'

function cell(v: unknown): { text: string; cls: string } {
  if (v === null || v === undefined) return { text: '', cls: 'faint' }
  if (typeof v === 'number') return { text: Number.isInteger(v) ? String(v) : fmtNum(v, 3), cls: 'num' }
  if (typeof v === 'string' && v.startsWith('{')) return { text: v, cls: 'cell-json' }
  if (typeof v === 'object') return { text: JSON.stringify(v), cls: 'cell-json' }
  return { text: String(v), cls: '' }
}

export function DataTable({ columns, rows, onRow, hide = [] }: { columns: string[]; rows: Record<string, unknown>[]; onRow?: (r: Record<string, unknown>) => void; hide?: string[] }) {
  const cols = columns.filter((c) => !hide.includes(c))
  return (
    <div className="table-wrap">
      <table className="data">
        <thead>
          <tr>
            {cols.map((c) => (
              <th key={c} className={rows[0] && typeof rows[0][c] === 'number' ? 'num' : ''}>
                {c}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i} className={onRow ? 'clickable' : ''} onClick={onRow ? () => onRow(r) : undefined}>
              {cols.map((c) => {
                const { text, cls } = cell(r[c])
                return (
                  <td key={c} className={cls} title={cls === 'cell-json' ? text : undefined}>
                    {text}
                  </td>
                )
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

export function Pagination({ total, limit, offset, onChange }: { total: number; limit: number; offset: number; onChange: (offset: number) => void }) {
  const page = Math.floor(offset / limit) + 1
  const pages = Math.max(1, Math.ceil(total / limit))
  return (
    <div className="row spread" style={{ marginTop: 10 }}>
      <span className="muted small">
        {total === 0 ? 'No rows' : `${offset + 1}–${Math.min(offset + limit, total)} of ${total.toLocaleString()}`}
      </span>
      <div className="row">
        <button className="btn sm" disabled={page <= 1} onClick={() => onChange(0)}>
          «
        </button>
        <button className="btn sm" disabled={page <= 1} onClick={() => onChange(Math.max(0, offset - limit))}>
          ‹
        </button>
        <span className="small muted">
          {page} / {pages}
        </span>
        <button className="btn sm" disabled={page >= pages} onClick={() => onChange(offset + limit)}>
          ›
        </button>
        <button className="btn sm" disabled={page >= pages} onClick={() => onChange((pages - 1) * limit)}>
          »
        </button>
      </div>
    </div>
  )
}
