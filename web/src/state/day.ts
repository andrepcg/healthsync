import { useCallback } from 'react'
import { useSearchParams } from 'react-router-dom'

/** The open day lives in the URL (?day=YYYY-MM-DD) so it survives reloads and links. */
export function useDayParam(): [string | null, (date: string | null, step?: boolean) => void] {
  const [params, setParams] = useSearchParams()
  const raw = params.get('day')
  const day = raw && /^\d{4}-\d{2}-\d{2}$/.test(raw) ? raw : null
  const set = useCallback(
    (date: string | null, step = false) =>
      setParams(
        (p) => {
          const n = new URLSearchParams(p)
          if (date) n.set('day', date)
          else n.delete('day')
          return n
        },
        // Opening adds a history entry so Back closes it; stepping does not.
        { replace: step },
      ),
    [setParams],
  )
  return [day, set]
}

/** Opens the clicked day on daily charts; undefined for week/month buckets. */
export function useDayClick(bucket: 'day' | 'week' | 'month') {
  const [, openDay] = useDayParam()
  const onDay = useCallback((d: string) => openDay(d), [openDay])
  return bucket === 'day' ? onDay : undefined
}
