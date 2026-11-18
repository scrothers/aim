import type { ReactNode } from 'react'
import { render } from '@testing-library/react'
import { RouterProvider, createMemoryHistory, createRootRoute, createRoute, createRouter } from '@tanstack/react-router'

/** Renders `ui` inside a memory router that knows the app's link targets. */
export async function renderWithRouter(ui: ReactNode, initial = '/mail') {
  const root = createRootRoute({ component: () => <>{ui}</> })
  const routes = ['/', '/mail', '/roster', '/org-chart', '/settings', '/search', '/approvals', '/tasks'].map((path) =>
    createRoute({ getParentRoute: () => root, path, validateSearch: (s: Record<string, unknown>) => s }),
  )
  const router = createRouter({
    routeTree: root.addChildren(routes),
    history: createMemoryHistory({ initialEntries: [initial] }),
  })
  await router.load()
  return render(<RouterProvider router={router} />)
}
