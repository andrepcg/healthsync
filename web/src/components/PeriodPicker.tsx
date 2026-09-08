import { PRESETS, type PeriodState, type Preset } from '../state/period'
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
        <label className="check">
          <input type="checkbox" checked={state.compare} onChange={(e) => set({ compare: e.target.checked })} />
          vs previous period
        </label>
      )}
      {last && state.to === last && <span className="faint small nowrap">as of {fmtDate(last)}</span>}
    </div>
  )
}
