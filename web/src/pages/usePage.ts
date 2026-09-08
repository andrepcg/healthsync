import { useMemo } from 'react'
import { useParams } from 'react-router-dom'
import { useAvailability, usePerson } from '../api/hooks'
import { usePeriod } from '../state/usePeriod'
import { tokens } from '../charts/theme'
import { useThemeMode } from '../state/ThemeContext'

/** Everything a person page needs: who, what data exists, and the period. */
export function usePage() {
  const { personId } = useParams()
  const person = usePerson(personId)
  const availability = useAvailability(personId)
  const period = usePeriod(person.data?.last_date, person.data?.first_date)
  const mode = useThemeMode()
  const t = useMemo(() => tokens(), [mode]) // eslint-disable-line react-hooks/exhaustive-deps
  const has = (table: string) => !!availability.data?.tables.some((x) => x.table === table || x.metric_key === table)
  return { personId: personId!, person: person.data, availability: availability.data, period, t, has, loading: person.isLoading || availability.isLoading }
}
