import { format, parseISO, startOfISOWeek, startOfMonth } from 'date-fns'

export function bucketKey(day: string, bucket: 'day' | 'week' | 'month'): string {
  const d = parseISO(day)
  if (bucket === 'week') return format(startOfISOWeek(d), 'yyyy-MM-dd')
  if (bucket === 'month') return format(startOfMonth(d), 'yyyy-MM-dd')
  return day
}
