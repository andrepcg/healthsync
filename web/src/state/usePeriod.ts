import { useCallback, useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'
import { autoBucket, previousPeriod, readPeriod, writePeriod, type PeriodState } from './period'

/**
 * Period lives in the URL so it survives reloads and person switches. The
 * anchor is the person's last day with data so presets stay meaningful for a
 * stale export.
 */
export function usePeriod(anchor?: string, first?: string) {
  const [params, setParams] = useSearchParams()
  const anchorDay = anchor ?? new Date().toISOString().slice(0, 10)
  const state = useMemo(() => readPeriod(params, anchorDay, first), [params, anchorDay, first])
  const set = useCallback(
    (next: Partial<PeriodState>) => {
      setParams((prev) => writePeriod(prev, next), { replace: true })
    },
    [setParams],
  )
  const range = useMemo(() => ({ from: state.from, to: state.to }), [state.from, state.to])
  const prev = useMemo(() => previousPeriod(state.from, state.to), [state.from, state.to])
  const bucket = useMemo(() => autoBucket(state.from, state.to), [state.from, state.to])
  return { ...state, range, prev, bucket, set }
}
