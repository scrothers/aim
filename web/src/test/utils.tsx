import type { ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import type { UserEvent } from '@testing-library/user-event'

/** Opens a Radix select (role combobox) and picks the option named `name`. */
export async function pickOption(user: UserEvent, trigger: HTMLElement, name: string) {
  await user.click(trigger)
  await user.click(await screen.findByRole('option', { name }))
}
import { RouterProvider, createMemoryHistory, createRootRoute, createRoute, createRouter } from '@tanstack/react-router'

/**
 * Stubs window.matchMedia as a viewport `width` px wide, answering min-width
 * and max-width queries. Undo with `delete window.matchMedia`.
 */
export function mockViewport(width: number) {
  window.matchMedia = ((q: string) => {
    const min = /min-width:\s*(\d+)px/.exec(q)
    const max = /max-width:\s*(\d+)px/.exec(q)
    const matches = (!min || width >= Number(min[1])) && (!max || width <= Number(max[1]))
    return { matches, media: q, addEventListener: () => {}, removeEventListener: () => {} }
  }) as unknown as typeof window.matchMedia
}

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
