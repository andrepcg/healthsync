import { PRESETS, type CompareMode, type PeriodState, type Preset } from '../state/period'
import { fmtDate } from '../format'

interface Props {
  state: PeriodState
  set: (next: Partial<PeriodState>) => void
  first?: string
  last?: string
  showCompare?: boolean
}

export function PeriodPicker({ state, set, first, last, showCompare = true }: Props) {
  return (
    <div className="row">
      <div className="seg" role="tablist">
        {PRESETS.map((p) => (
          <button key={p.key} className={state.preset === p.key ? 'active' : ''} onClick={() => set({ preset: p.key as Preset })}>
            {p.label}
          </button>
        ))}
      </div>
      <input
        type="date"
        value={state.from}
        min={first}
        max={state.to}
        onChange={(e) => e.target.value && set({ preset: 'custom', from: e.target.value, to: state.to })}
        aria-label="from"
      />
      <span className="faint">→</span>
      <input
        type="date"
        value={state.to}
        min={state.from}
        max={last}
        onChange={(e) => e.target.value && set({ preset: 'custom', from: state.from, to: e.target.value })}
        aria-label="to"
      />
      {showCompare && (
        <select value={state.compare} onChange={(e) => set({ compare: e.target.value as CompareMode })} aria-label="compare with">
          <option value="previous">vs previous period</option>
          <option value="year">vs same dates last year</option>
          <option value="none">no comparison</option>
        </select>
      )}
      {last && state.to === last && <span className="faint small nowrap">as of {fmtDate(last)}</span>}
    </div>
  )
}
