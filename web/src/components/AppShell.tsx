import { NavLink, Outlet, useLocation, useParams } from 'react-router-dom'
import { ErrorBoundary } from './ErrorBoundary'
import { usePerson } from '../api/hooks'
import { useThemeCtx } from '../state/ThemeContext'
import { PersonSwitcher } from './PersonSwitcher'
import { ImportBanner } from './ImportBanner'

const NAV: { to: string; label: string; ico: string; section?: string }[] = [
  { to: 'overview', label: 'Overview', ico: '🏠' },
  { to: 'activity', label: 'Activity', ico: '🔥', section: 'Health' },
  { to: 'heart', label: 'Heart', ico: '❤️' },
  { to: 'sleep', label: 'Sleep', ico: '🌙' },
  { to: 'body', label: 'Body', ico: '⚖️' },
  { to: 'mobility', label: 'Mobility & Running', ico: '🚶' },
  { to: 'workouts', label: 'Workouts', ico: '🏃', section: 'Records' },
  { to: 'ecg', label: 'ECG', ico: '📈' },
  { to: 'environment', label: 'Environment & Hearing', ico: '🔊' },
  { to: 'mindfulness', label: 'Mindfulness', ico: '🧘' },
  { to: 'explore', label: 'Explore data', ico: '🗂️', section: 'Data' },
]

export function AppShell() {
  const { personId } = useParams()
  const { data: person } = usePerson(personId)
  const { theme, setTheme } = useThemeCtx()
  const loc = useLocation()
  let lastPerson: string | null = null
  try {
    lastPerson = localStorage.getItem('healthsync.person')
  } catch {
    /* ignore */
  }
  const base = personId ? `/p/${personId}` : lastPerson ? `/p/${lastPerson}` : null

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand">
          <span className="logo">❤</span> healthsync
        </div>
        {NAV.map((n) => (
          <div key={n.to} style={{ display: 'contents' }}>
            {n.section && <div className="nav-section">{n.section}</div>}
            <NavLink
              to={base ? `${base}/${n.to}${loc.search}` : '/people'}
              className={({ isActive }) => `nav-link ${isActive && personId ? 'active' : ''}`}
              end={false}
              style={base ? undefined : { opacity: 0.5 }}
              title={base ? undefined : 'Add a person first'}
            >
              <span className="ico">{n.ico}</span>
              {n.label}
            </NavLink>
          </div>
        ))}
        <div className="nav-section">Family</div>
        <NavLink to="/compare" className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}>
          <span className="ico">⚖️</span>Compare
        </NavLink>
        <NavLink to="/people" className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}>
          <span className="ico">👨‍👩‍👧</span>People & imports
        </NavLink>
        <div className="side-foot">
          <label className="check">
            theme{' '}
            <select value={theme} onChange={(e) => setTheme(e.target.value as 'light' | 'dark' | 'system')} style={{ padding: '2px 6px' }}>
              <option value="system">system</option>
              <option value="light">light</option>
              <option value="dark">dark</option>
            </select>
          </label>
        </div>
      </aside>
      <header className="topbar">
        <PersonSwitcher current={person} />
        {person?.last_date && <span className="faint small">data through {person.last_date}</span>}
      </header>
      <main className="main">
        {personId && <ImportBanner personId={personId} />}
        <ErrorBoundary label="this page">
          <Outlet />
        </ErrorBoundary>
      </main>
    </div>
  )
}
