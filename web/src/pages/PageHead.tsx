import type { ReactNode } from 'react'
import { PeriodPicker } from '../components/PeriodPicker'
import type { usePage } from './usePage'

type Page = ReturnType<typeof usePage>

export function PageHead({ title, sub, page, right, compare = true }: { title: string; sub?: ReactNode; page: Page; right?: ReactNode; compare?: boolean }) {
  return (
    <div className="page-head">
      <div>
        <h1>{title}</h1>
        {sub && <div className="sub">{sub}</div>}
      </div>
      <div className="row">
        {right}
        <PeriodPicker state={page.period} set={page.period.set} first={page.person?.first_date} last={page.person?.last_date} showCompare={compare} />
      </div>
    </div>
  )
}

export function NoData({ page }: { page: Page }) {
  return (
    <div className="card empty">
      <div className="big">📥</div>
      <div style={{ fontWeight: 600, color: 'var(--text)' }}>No data yet for {page.person?.name ?? 'this person'}</div>
      <div style={{ marginTop: 6 }}>
        Upload an Apple Health export on the <a href="/people">People & imports</a> page.
      </div>
    </div>
  )
}
