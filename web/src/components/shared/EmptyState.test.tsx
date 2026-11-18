import { render, screen } from '@testing-library/react'
import { Inbox } from 'lucide-react'
import { describe, expect, it } from 'vitest'
import { EmptyState } from './EmptyState'

describe('EmptyState', () => {
  it('shows a decorative icon, one line and an optional action', () => {
    const { container } = render(
      <EmptyState icon={Inbox} action={<button type="button">Turn on</button>}>
        Nothing here.
      </EmptyState>,
    )
    const root = container.querySelector('[data-slot="empty-state"]')!
    expect(root).toHaveTextContent('Nothing here.')
    expect(root.querySelector('svg')?.closest('[aria-hidden="true"]')).not.toBeNull()
    expect(screen.getByRole('button', { name: 'Turn on' })).toBeInTheDocument()
  })

  it('has a compact form for narrow slots', () => {
    const { container } = render(
      <EmptyState compact icon={Inbox}>
        Nothing waiting
      </EmptyState>,
    )
    expect(container.querySelector('[data-slot="empty-state"]')).toHaveClass('border-dashed')
    expect(screen.getByText('Nothing waiting')).toHaveClass('text-xs')
  })
})
