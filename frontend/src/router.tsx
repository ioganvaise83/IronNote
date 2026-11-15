import { createBrowserRouter } from 'react-router-dom'
import { AppShell } from './components/AppShell'
import { WorkoutsPage } from './pages/WorkoutsPage'
import { ActiveWorkoutPage } from './pages/ActiveWorkoutPage'
import { TemplateEditorPage } from './pages/TemplateEditorPage'
import { HistoryPage } from './pages/HistoryPage'
import { SessionDetailPage } from './pages/SessionDetailPage'
import { ExercisesPage } from './pages/ExercisesPage'
import { ExerciseFormPage } from './pages/ExerciseFormPage'
import { StatsPage } from './pages/StatsPage'
import { SettingsPage } from './pages/SettingsPage'
import { NotFoundPage } from './pages/NotFoundPage'

export const router = createBrowserRouter([
  {
    path: '/',
    element: <AppShell />,
    children: [
      { index: true, element: <WorkoutsPage /> },
      { path: 'workouts/:id', element: <ActiveWorkoutPage /> },
      { path: 'templates/new', element: <TemplateEditorPage /> },
      { path: 'templates/:id', element: <TemplateEditorPage /> },
      { path: 'history', element: <HistoryPage /> },
      { path: 'history/:id', element: <SessionDetailPage /> },
      { path: 'exercises', element: <ExercisesPage /> },
      { path: 'exercises/new', element: <ExerciseFormPage /> },
      { path: 'exercises/:id', element: <ExerciseFormPage /> },
      { path: 'stats', element: <StatsPage /> },
      { path: 'settings', element: <SettingsPage /> },
      { path: '*', element: <NotFoundPage /> },
    ],
  },
])
