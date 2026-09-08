import { useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { personPath, withParams } from '../api/client'
import { useTable } from '../api/hooks'
import { DataTable, Pagination } from '../components/DataTable'
import { ErrorNote, Loading, Section } from '../components/Section'
import { fmtInt, metricLabel } from '../format'
import { NoData, PageHead } from './PageHead'
import { usePage } from './usePage'

const PAGE = 100
const EXTRA = [
  { table: 'activity_summary', name: 'Activity rings (daily)', group: 'records' },
  { table: 'workouts', name: 'Workouts', group: 'records' },
  { table: 'workout_statistics', name: 'Workout statistics', group: 'records' },
  { table: 'workout_events', name: 'Workout events', group: 'records' },
  { table: 'workout_zones', name: 'Workout HR zones', group: 'records' },
  { table: 'workout_routes', name: 'Workout routes', group: 'records' },
  { table: 'ecg', name: 'ECG recordings', group: 'records' },
  { table: 'hrv_beats', name: 'HRV beat-to-beat', group: 'records' },
  { table: 'devices', name: 'Devices', group: 'meta' },
  { table: 'profile', name: 'Profile (Me)', group: 'meta' },
  { table: 'imports', name: 'Imports', group: 'meta' },
]

export function ExplorePage() {
  const page = usePage()
  const { personId, period, person, availability } = page
  const [params, setParams] = useSearchParams()
  const table = params.get('table') ?? ''
  const type = params.get('type') ?? ''
  const [offset, setOffset] = useState(0)
  const [order, setOrder] = useState<'asc' | 'desc'>('desc')
  const q = useTable(personId, table || undefined, { ...period.range, type, limit: PAGE, offset, order })

  const groups = useMemo(() => {
    const out = new Map<string, { table: string; name: string; rows: number; type?: string }[]>()
    for (const tb of availability?.tables ?? []) {
      if (['workouts', 'activity_summary', 'ecg'].includes(tb.table)) continue
      const g = tb.group
      const list = out.get(g) ?? []
      list.push({ table: tb.table, name: tb.name, rows: tb.rows })
      out.set(g, list)
    }
    for (const o of availability?.other_types ?? []) {
      const list = out.get('other types') ?? []
      list.push({ table: o.table, name: o.hk_type.replace(/^HK(Quantity|Category|Data)Type(Identifier)?/, ''), rows: o.rows, type: o.hk_type })
      out.set('other types', list)
    }
    return out
  }, [availability])

  const select = (tb: string, ty?: string) => {
    setOffset(0)
    setParams((p) => {
      const n = new URLSearchParams(p)
      n.set('table', tb)
      if (ty) n.set('type', ty)
      else n.delete('type')
      return n
    })
  }

  if (page.loading) return <Loading />
  if (person && !person.has_data) return <><PageHead title="Explore data" page={page} /><NoData page={page} /></>

  const csvHref = withParams(personPath(personId, `/tables/${table}`), { ...period.range, type, format: 'csv' })

  return (
    <>
      <PageHead title="Explore data" sub="Every table, every row. Filters follow the period picker." page={page} compare={false} right={<a className="btn sm" href={personPath(personId, '/export.db')}>Download SQLite</a>} />
      <div className="grid" style={{ gridTemplateColumns: 'minmax(220px, 260px) 1fr', alignItems: 'start' }}>
        <div className="card pad-0" style={{ maxHeight: '75vh', overflowY: 'auto', position: 'sticky', top: 72 }}>
          {[...groups.entries()].map(([g, list]) => (
            <div key={g}>
              <div className="nav-section" style={{ marginTop: 12 }}>{g}</div>
              {list.map((it) => (
                <button key={it.table + (it.type ?? '')} className={`menu-item ${table === it.table && (it.type ?? '') === type ? 'active' : ''}`} onClick={() => select(it.table, it.type)} style={{ padding: '6px 12px' }}>
                  <span style={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis' }}>{it.name}</span>
                  <span className="faint small">{fmtInt(it.rows)}</span>
                </button>
              ))}
            </div>
          ))}
          <div className="nav-section" style={{ marginTop: 12 }}>records & meta</div>
          {EXTRA.map((it) => (
            <button key={it.table} className={`menu-item ${table === it.table ? 'active' : ''}`} onClick={() => select(it.table)} style={{ padding: '6px 12px' }}>
              <span style={{ flex: 1 }}>{it.name}</span>
            </button>
          ))}
        </div>
        <div>
          {!table && <div className="card empty"><div className="big">🗂️</div>Pick a table on the left.</div>}
          {table && (
            <Section
              title={type ? type : metricLabel(table)}
              hint={q.data ? `${fmtInt(q.data.total)} rows in range` : undefined}
              right={
                <div className="row">
                  <div className="seg">
                    <button className={order === 'desc' ? 'active' : ''} onClick={() => setOrder('desc')}>newest</button>
                    <button className={order === 'asc' ? 'active' : ''} onClick={() => setOrder('asc')}>oldest</button>
                  </div>
                  <a className="btn sm" href={csvHref}>Download CSV</a>
                </div>
              }
            >
              {q.isLoading && !q.data && <Loading />}
              {q.error && <ErrorNote error={q.error} />}
              {q.data && (
                <>
                  <div style={{ maxHeight: '60vh', overflow: 'auto' }}>
                    <DataTable columns={q.data.columns} rows={q.data.rows} />
                  </div>
                  <Pagination total={q.data.total} limit={PAGE} offset={offset} onChange={setOffset} />
                </>
              )}
            </Section>
          )}
        </div>
      </div>
    </>
  )
}
