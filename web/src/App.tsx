import { lazy, Suspense } from 'react'
import { createBrowserRouter, Navigate, Outlet, useParams } from 'react-router-dom'
import { usePeople } from './api/hooks'
import { AppShell } from './components/AppShell'
import { Loading } from './components/Section'
import { OverviewPage } from './pages/Overview'
import { ActivityPage } from './pages/Activity'
import { HeartPage } from './pages/Heart'
import { SleepPage } from './pages/Sleep'
import { BodyPage } from './pages/Body'
import { MobilityPage } from './pages/Mobility'
import { WorkoutsPage } from './pages/Workouts'
import { EcgPage, EcgDetailPage } from './pages/Ecg'
import { EnvironmentPage } from './pages/Environment'
import { MindfulnessPage } from './pages/Mindfulness'
import { ExplorePage } from './pages/Explore'
import { PeoplePage } from './pages/People'
import { ComparePage } from './pages/Compare'
import { ObservationsPage } from './pages/Observations'

const WorkoutDetailPage = lazy(() => import('./pages/WorkoutDetail'))

/** "/" → last used person, else first person, else the people page. */
function Home() {
  const { data, isLoading, error } = usePeople()
  if (isLoading) return <Loading />
  if (error) return <div className="main err">Cannot reach the API: {(error as Error).message}</div>
  if (!data || data.length === 0) return <Navigate to="/people" replace />
  let last: string | null = null
  try {
    last = localStorage.getItem('healthsync.person')
  } catch {
    /* ignore */
  }
  const pick = data.find((p) => p.id === last) ?? data.find((p) => p.has_data) ?? data[0]
  return <Navigate to={`/p/${pick.id}/overview`} replace />
}

function PersonRoot() {
  const { personId } = useParams()
  return <Outlet key={personId} />
}

export const router = createBrowserRouter([
  { path: '/', element: <Home /> },
  {
    element: <AppShell />,
    children: [
      { path: '/people', element: <PeoplePage /> },
      { path: '/compare', element: <ComparePage /> },
      {
        path: '/p/:personId',
        element: <PersonRoot />,
        children: [
          { index: true, element: <Navigate to="overview" replace /> },
          { path: 'overview', element: <OverviewPage /> },
          { path: 'observations', element: <ObservationsPage /> },
          { path: 'activity', element: <ActivityPage /> },
          { path: 'heart', element: <HeartPage /> },
          { path: 'sleep', element: <SleepPage /> },
          { path: 'body', element: <BodyPage /> },
          { path: 'mobility', element: <MobilityPage /> },
          { path: 'workouts', element: <WorkoutsPage /> },
          {
            path: 'workouts/:wid',
            element: (
              <Suspense fallback={<Loading />}>
                <WorkoutDetailPage />
              </Suspense>
            ),
          },
          { path: 'ecg', element: <EcgPage /> },
          { path: 'ecg/:eid', element: <EcgDetailPage /> },
          { path: 'environment', element: <EnvironmentPage /> },
          { path: 'mindfulness', element: <MindfulnessPage /> },
          { path: 'explore', element: <ExplorePage /> },
        ],
      },
      { path: '*', element: <Navigate to="/" replace /> },
    ],
  },
])
