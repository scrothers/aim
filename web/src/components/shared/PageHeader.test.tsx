import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { PageHeader } from './PageHeader'

describe('PageHeader', () => {
  it('renders an h1 at the page scale with description and actions', () => {
    render(<PageHeader title="Tasks" description="Work items" badge={<span>3</span>} actions={<button type="button">New task</button>} />)
    const h = screen.getByRole('heading', { level: 1, name: 'Tasks' })
    expect(h).toHaveClass('text-[22px]', 'font-semibold')
    expect(screen.getByText('Work items')).toHaveClass('text-muted-foreground')
    expect(screen.getByRole('button', { name: 'New task' })).toBeInTheDocument()
    // The badge sits right after the title.
    expect(h.nextElementSibling).toHaveTextContent('3')
  })

  it('renders a pane title as h2 on the column scale', () => {
    render(<PageHeader as="h2" size="column" title="Operator inbox" />)
    const h = screen.getByRole('heading', { level: 2, name: 'Operator inbox' })
    expect(h).toHaveClass('text-lg', 'font-semibold')
    expect(h.closest('[data-slot="page-header"]')).toHaveAttribute('data-size', 'column')
  })

  it('omits empty slots', () => {
    const { container } = render(<PageHeader title="Settings" />)
    expect(container.querySelector('p')).toBeNull()
    expect(screen.queryByRole('button')).toBeNull()
  })
})
