import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchJson, personPath, postJson, type Params } from './client'
import type {
  Availability,
  ECGDetail,
  ECGSummary,
  EnvironmentOverview,
  HeartOverview,
  Highlight,
  ImportRow,
  Metric,
  ObservationReport,
  Person,
  Rings,
  RoutePoint,
  Series,
  SleepNightsResponse,
  Summary,
  TablePage,
  UploadStatus,
  WorkoutDetail,
  WorkoutItem,
} from './types'

export type Range = { from: string; to: string }

export function usePeople() {
  return useQuery({ queryKey: ['people'], queryFn: () => fetchJson<Person[]>('/api/people') })
}

export function usePerson(id: string | undefined) {
  return useQuery({
    queryKey: ['person', id],
    queryFn: () => fetchJson<Person>(personPath(id!)),
    enabled: !!id,
  })
}

export function useMetrics() {
  return useQuery({ queryKey: ['metrics'], queryFn: () => fetchJson<Metric[]>('/api/metrics'), staleTime: Infinity })
}

export function useAvailability(id: string | undefined) {
  return useQuery({
    queryKey: ['availability', id],
    queryFn: () => fetchJson<Availability>(personPath(id!, '/availability')),
    enabled: !!id,
  })
}

export function useSeries(id: string | undefined, metric: string | undefined, range: Range, bucket = 'day', enabled = true) {
  return useQuery({
    queryKey: ['series', id, metric, range.from, range.to, bucket],
    queryFn: () => fetchJson<Series>(personPath(id!, `/series/${encodeURIComponent(metric!)}`), { ...range, bucket }),
    enabled: !!id && !!metric && enabled,
  })
}

export function useSummary(id: string | undefined, range: Range, compare: boolean) {
  return useQuery({
    queryKey: ['summary', id, range.from, range.to, compare],
    queryFn: () => fetchJson<Summary>(personPath(id!, '/summary'), { ...range, compare: compare ? 'previous' : 'none' }),
    enabled: !!id && !!range.from && !!range.to,
  })
}

export function useHighlights(id: string | undefined, range: Range) {
  return useQuery({
    queryKey: ['highlights', id, range.from, range.to],
    queryFn: () => fetchJson<Highlight[]>(personPath(id!, '/highlights'), range),
    enabled: !!id && !!range.from && !!range.to,
  })
}

export function useObservations(id: string | undefined, asOf?: string) {
  return useQuery({
    queryKey: ['observations', id, asOf ?? ''],
    queryFn: () => fetchJson<ObservationReport>(personPath(id!, '/observations'), { as_of: asOf }),
    enabled: !!id,
  })
}

export function useRings(id: string | undefined, range: Range) {
  return useQuery({
    queryKey: ['rings', id, range.from, range.to],
    queryFn: () => fetchJson<Rings>(personPath(id!, '/activity/rings'), range),
    enabled: !!id,
  })
}

export function useSleepNights(id: string | undefined, range: Range) {
  return useQuery({
    queryKey: ['sleep', id, range.from, range.to],
    queryFn: () => fetchJson<SleepNightsResponse>(personPath(id!, '/sleep/nights'), range),
    enabled: !!id,
  })
}

export function useHeart(id: string | undefined, range: Range, bucket = 'day') {
  return useQuery({
    queryKey: ['heart', id, range.from, range.to, bucket],
    queryFn: () => fetchJson<HeartOverview>(personPath(id!, '/heart/overview'), { ...range, bucket }),
    enabled: !!id,
  })
}

export function useEnvironment(id: string | undefined, range: Range, bucket = 'day') {
  return useQuery({
    queryKey: ['environment', id, range.from, range.to, bucket],
    queryFn: () => fetchJson<EnvironmentOverview>(personPath(id!, '/environment'), { ...range, bucket }),
    enabled: !!id,
  })
}

export function useWorkouts(id: string | undefined, params: Params) {
  return useQuery({
    queryKey: ['workouts', id, params],
    queryFn: () => fetchJson<{ total: number; items: WorkoutItem[] }>(personPath(id!, '/workouts'), params),
    enabled: !!id,
  })
}

export function useWorkoutTypes(id: string | undefined) {
  return useQuery({
    queryKey: ['workout-types', id],
    queryFn: () => fetchJson<{ type: string; count: number }[]>(personPath(id!, '/workouts/types')),
    enabled: !!id,
  })
}

export function useWorkout(id: string | undefined, wid: string | undefined) {
  return useQuery({
    queryKey: ['workout', id, wid],
    queryFn: () => fetchJson<WorkoutDetail>(personPath(id!, `/workouts/${wid}`)),
    enabled: !!id && !!wid,
  })
}

export function useRoute(id: string | undefined, wid: string | undefined, enabled: boolean) {
  return useQuery({
    queryKey: ['route', id, wid],
    queryFn: () => fetchJson<{ route_id: number; points: RoutePoint[] }>(personPath(id!, `/workouts/${wid}/route`)),
    enabled: !!id && !!wid && enabled,
  })
}

export function useECGs(id: string | undefined, range?: Range) {
  return useQuery({
    queryKey: ['ecgs', id, range?.from, range?.to],
    queryFn: () => fetchJson<ECGSummary[]>(personPath(id!, '/ecg'), range),
    enabled: !!id,
  })
}

export function useECG(id: string | undefined, eid: string | number | undefined, points?: number) {
  return useQuery({
    queryKey: ['ecg', id, eid, points],
    queryFn: () => fetchJson<ECGDetail>(personPath(id!, `/ecg/${eid}`), { points }),
    enabled: !!id && eid !== undefined,
    staleTime: Infinity,
  })
}

export function useTable(id: string | undefined, table: string | undefined, params: Params) {
  return useQuery({
    queryKey: ['table', id, table, params],
    queryFn: () => fetchJson<TablePage>(personPath(id!, `/tables/${encodeURIComponent(table!)}`), params),
    enabled: !!id && !!table,
    placeholderData: (prev) => prev,
  })
}

export function useImports(id: string | undefined) {
  return useQuery({
    queryKey: ['imports', id],
    queryFn: () => fetchJson<ImportRow[]>(personPath(id!, '/imports')),
    enabled: !!id,
  })
}

/** Polls while an import is running and invalidates everything when it ends. */
export function useUploadStatus(id: string | undefined, active: boolean) {
  const qc = useQueryClient()
  return useQuery({
    queryKey: ['upload-status', id],
    queryFn: async () => {
      const st = await fetchJson<UploadStatus>(personPath(id!, '/upload/status'))
      return st
    },
    enabled: !!id,
    refetchInterval: (q) => {
      const st = q.state.data
      if (active || st?.status === 'running') return 1500
      return false
    },
    structuralSharing: (prev, next) => {
      const p = prev as UploadStatus | undefined
      const n = next as UploadStatus
      if (p?.status === 'running' && n.status !== 'running') {
        // Import just finished: refresh every person-scoped query.
        setTimeout(() => qc.invalidateQueries({ predicate: (query) => query.queryKey[1] === id || query.queryKey[0] === 'people' }), 0)
      }
      return next
    },
  })
}

export function usePeopleMutations() {
  const qc = useQueryClient()
  const invalidate = () => qc.invalidateQueries({ queryKey: ['people'] })
  const create = useMutation({
    mutationFn: (p: Partial<Person>) => postJson<Person>('/api/people', p),
    onSuccess: invalidate,
  })
  const update = useMutation({
    mutationFn: ({ id, ...p }: Partial<Person> & { id: string }) => postJson<Person>(personPath(id), p, 'PATCH'),
    onSuccess: (_d, v) => {
      invalidate()
      qc.invalidateQueries({ queryKey: ['person', v.id] })
    },
  })
  const remove = useMutation({
    mutationFn: (id: string) => fetchJson<void>(personPath(id), undefined, { method: 'DELETE' }),
    onSuccess: invalidate,
  })
  return { create, update, remove }
}
