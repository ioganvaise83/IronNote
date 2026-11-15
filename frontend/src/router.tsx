import { createBrowserRouter } from 'react-router-dom'
import { AppShell } from './components/AppShell'
import { RedirectIfAuthed, RequireAdmin, RequireAuth, RequireSession } from './components/Guards'
import { WorkoutsPage } from './pages/WorkoutsPage'
import { ActiveWorkoutPage } from './pages/ActiveWorkoutPage'
import { TemplateEditorPage } from './pages/TemplateEditorPage'
import { HistoryPage } from './pages/HistoryPage'
import { SessionDetailPage } from './pages/SessionDetailPage'
import { ExercisesPage } from './pages/ExercisesPage'
import { ExerciseFormPage } from './pages/ExerciseFormPage'
import { StatsPage } from './pages/StatsPage'
import { SettingsPage } from './pages/SettingsPage'
import { UsersPage } from './pages/UsersPage'
import { LoginPage } from './pages/LoginPage'
import { ChangePasswordPage } from './pages/ChangePasswordPage'
import { NotFoundPage } from './pages/NotFoundPage'

export const router = createBrowserRouter([
  {
    // Signed-out surface.
    element: <RedirectIfAuthed />,
    children: [{ path: '/login', element: <LoginPage /> }],
  },
  {
    // Reachable with a session even while a password change is required.
    element: <RequireSession />,
    children: [{ path: '/change-password', element: <ChangePasswordPage /> }],
  },
  {
    path: '/',
    element: <RequireAuth />,
    children: [
      {
        element: <AppShell />,
        children: [
          { index: true, element: <WorkoutsPage /> },
          { path: 'workouts/:id', element: <ActiveWorkoutPage /> },
          { path: 'templates/new', element: <TemplateEditorPage /> },
          { path: 'templates/:id', element: <TemplateEditorPage /> },
          { path: 'history', element: <HistoryPage /> },
          { path: 'history/:id', element: <SessionDetailPage /> },
          { path: 'exercises', element: <ExercisesPage /> },
          // Catalogue editing is admin-only; normal users still read the list.
          {
            element: <RequireAdmin />,
            children: [
              { path: 'exercises/new', element: <ExerciseFormPage /> },
              { path: 'exercises/:id', element: <ExerciseFormPage /> },
              { path: 'users', element: <UsersPage /> },
            ],
          },
          { path: 'stats', element: <StatsPage /> },
          { path: 'settings', element: <SettingsPage /> },
          { path: '*', element: <NotFoundPage /> },
        ],
      },
    ],
  },
])
