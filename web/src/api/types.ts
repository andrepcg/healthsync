// Hand-written mirrors of the Go JSON shapes. Keep in sync with internal/server.

export interface Person {
  id: string
  name: string
  color: string
  emoji: string
  dob: string
  sex: string
  created_at: string
  updated_at: string
  has_data: boolean
  first_date?: string
  last_date?: string
  last_import_at?: string
  importing: boolean
  total_rows: number
}

export interface Metric {
  key: string
  name: string
  unit: string
  group: string
  kind: 'cumulative' | 'sample' | 'duration' | 'event'
  table: string
  good?: 'up' | 'down' | ''
}

export interface SeriesPoint {
  t: string
  v: number
  n: number
  /** Days with data behind a point: weekly/monthly totals are an average day over these. */
  days?: number
  min?: number
  max?: number
}

/** A person's usual range: median ± one robust spread of daily values. */
export interface Band {
  median: number
  lo: number
  hi: number
  days: number
  from: string
  to: string
}

export interface Series {
  metric: string
  name: string
  unit: string
  bucket: 'day' | 'week' | 'month'
  agg: string
  points: SeriesPoint[]
  /** "Your normal" over the 90 days ending at the period's last day (sample metrics and sleep). */
  normal?: Band
}

export interface Tile {
  metric: string
  name: string
  unit: string
  kind: string
  value: number
  total?: number
  days: number
  previous?: number
  delta_pct?: number
  direction: 'up' | 'down' | 'flat' | ''
  good: 'up' | 'down' | ''
  sparkline: SeriesPoint[]
}

export interface Summary {
  period: { from: string; to: string }
  previous?: { from: string; to: string }
  tiles: Tile[]
}

export interface Highlight {
  kind: string
  title: string
  detail: string
  date?: string
  value?: number
  unit?: string
  link?: string
  tone: 'good' | 'bad' | 'neutral' | 'info'
}

export interface ActivityDay {
  date: string
  energy: number | null
  energy_goal: number | null
  energy_unit: string
  exercise: number | null
  exercise_goal: number | null
  stand: number | null
  stand_goal: number | null
  move: number | null
  move_goal: number | null
  closed: { energy: boolean; exercise: boolean; stand: boolean; all: boolean }
}

export interface Rings {
  days: ActivityDay[]
  streaks: {
    current_all: number
    longest_all: number
    current_energy: number
    longest_energy: number
    days_all_closed: number
    days: number
  }
}

export interface SleepNight {
  night: string
  hours: number
  naps: number
  onset: string
  wake: string
  stages: { core: number; deep: number; rem: number; awake: number; unspecified: number; inbed: number }
  vitals: {
    wrist_temp?: number
    resp_rate?: number
    spo2_min?: number
    spo2_avg?: number
    breathing_disturbances?: number
    hr_min?: number
    hr_avg?: number
    hrv_rmssd?: number
    hrv?: number
  }
}

export interface SleepTiming {
  debt: { minutes: number; nights: number; learning: boolean; need_hours: number; from: string; to: string }
  chronotype?: { msf: string; msfsc: string; msw: string; label: string; social_jetlag_min: number; free_nights: number; work_nights: number }
  chronotype_reason?: string
  chronotype_window: string
  need_source: string
}

export interface SleepNightsResponse {
  nights: SleepNight[]
  days_in_period: number
  timing?: SleepTiming
}

export interface ReadinessComponent {
  key: string
  label: string
  value: number
  unit: string
  usual: number
  spread: number
  z: number
  score: number
  weight: number
}

export interface ReadinessDay {
  date: string
  night: string
  score: number | null
  band?: 'good' | 'fair' | 'low'
  components: ReadinessComponent[]
  missing: { check: string; reason: string }[]
}

export interface Readiness {
  days: ReadinessDay[]
  sleep_need: number
  sleep_need_source: string
}

export interface DayMetric {
  key: string
  name: string
  group: string
  unit: string
  agg: string
  value: number
  n: number
  normal?: Band
  note?: string
  /** A total on the last day with data, taken before the day ended. */
  partial?: boolean
}

export interface DayView {
  date: string
  metrics: DayMetric[]
  night?: SleepNight
  workouts: WorkoutItem[]
  events: { time: string; kind: string; name: string; value?: string; link?: string }[]
  heart_rate: { t: string; v: number; min: number; max: number; n: number }[]
  first_date?: string
  last_date?: string
}

export interface WorkoutItem {
  id: number
  activity_type: string
  source: string
  start: string
  end: string
  duration_min: number | null
  distance_km?: number
  energy_kcal?: number
  avg_hr?: number
  max_hr?: number
  effort_score?: number
  has_route: boolean
  indoor?: boolean
}

export interface WorkoutDetail extends WorkoutItem {
  source_version?: string
  device?: string
  metadata: Record<string, string>
  statistics: Record<string, unknown>[]
  events: Record<string, unknown>[]
  zones: Record<string, unknown>[]
  route?: Record<string, unknown>
  hr_samples: SeriesPoint[]
}

export interface RoutePoint {
  seq: number
  t: string
  lat: number
  lon: number
  ele: number | null
  speed: number | null
  course: number | null
  hacc: number | null
  vacc: number | null
}

export interface ECGSummary {
  id: number
  recorded_at: string
  classification: string
  symptoms: string
  software_version: string
  device: string
  sample_rate: number
  lead: string
  unit: string
  sample_count: number
  avg_hr?: number
  file_name: string
}

export interface ECGDetail extends ECGSummary {
  duration_s: number
  samples?: number[]
  envelope?: [number, number][]
}

export interface TableAvailability {
  table: string
  metric_key?: string
  name: string
  group: string
  kind: string
  unit?: string
  rows: number
  first?: string
  last?: string
}

export interface Availability {
  first_date?: string
  last_date?: string
  total_rows: number
  tables: TableAvailability[]
  other_types: { hk_type: string; table: string; rows: number; first?: string; last?: string }[]
  workouts: number
  routes: number
  ecgs: number
  activity_days: number
}

export interface TablePage {
  table: string
  columns: string[]
  total: number
  rows: Record<string, unknown>[]
  limit: number
  offset: number
}

export interface ImportRow {
  id: number
  filename: string
  size_bytes: number
  export_date: string
  locale: string
  started_at: string
  finished_at: string
  status: string
  error: string
  records: number
  workouts: number
  routes: number
  ecgs: number
  activity_days: number
  hrv_beats: number
  errors: number
  table_stats: string
}

export interface Progress {
  records: number
  workouts: number
  routes: number
  route_points: number
  ecgs: number
  activity_days: number
  hrv_beats: number
  errors: number
}

export interface UploadStatus {
  status: 'idle' | 'running' | 'completed' | 'failed'
  running: boolean
  progress: Progress
  filename: string
  started_at?: string
  elapsed?: string
  error?: string
  result?: Progress & { Warnings?: string[]; ExportDate?: string }
  profile_updated?: boolean
}

export interface HeartOverview {
  heart_rate?: Series
  resting_heart_rate?: Series
  hrv?: Series
  hrv_rmssd?: Series
  walking_heart_rate?: Series
  heart_rate_recovery?: Series
  vo2max?: Series
  spo2?: Series
  respiratory_rate?: Series
  afib_burden?: Series
  events: Record<string, unknown>[]
  blood_pressure: Record<string, unknown>[]
}

export interface EnvironmentOverview {
  headphone_audio_exposure?: Series
  environmental_audio_exposure?: Series
  time_in_daylight?: Series
  uv_exposure?: Series
  water_temperature?: Series
  events: Record<string, unknown>[]
}

export interface Evidence {
  label: string
  value: string
}

export interface Observation {
  id: string
  category: string
  severity: 'alert' | 'warning' | 'notice' | 'info'
  tone: 'good' | 'bad' | 'neutral'
  title: string
  detail: string
  advice?: string
  window: string
  evidence: Evidence[]
  link?: string
  days: number
}

export interface ObservationReport {
  as_of: string
  observations: Observation[]
  lead?: string
  checked: string[]
  skipped: { check: string; reason: string }[]
}

export interface ECGAnalysis {
  sample_rate: number
  duration_s: number
  beats: number
  r_peaks_s: number[]
  r_peaks_mv: number[]
  rr_ms: number[]
  hr_mean: number
  hr_min: number
  hr_max: number
  sdnn_ms: number
  rmssd_ms: number
  pnn50_pct: number
  rr_cv_pct: number
  irregularity: 'regular' | 'mildly irregular' | 'irregular' | 'very irregular'
  premature_beats: number
  quality: 'good' | 'fair' | 'poor'
  noise_mv: number
  r_amplitude_mv: number
  snr: number
  notes: string[]
  template_mv: number[]
  template_t0_ms: number
  template_dt_ms: number
}

export interface ECGAnalysisResponse {
  id: number
  recorded_at: string
  classification: string
  device_avg_hr?: number
  analysis: ECGAnalysis
}
