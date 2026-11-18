import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Badge, CountBadge } from './badge'

describe('Badge', () => {
  it('fills only attention counts with the brand colour', () => {
    render(
      <>
        <Badge variant="count">3</Badge>
        <Badge variant="count-attention">4</Badge>
      </>,
    )
    expect(screen.getByText('3')).toHaveClass('bg-code', 'text-muted-foreground', 'rounded-full')
    expect(screen.getByText('3')).not.toHaveClass('bg-brand')
    expect(screen.getByText('4')).toHaveClass('bg-brand', 'text-on-accent', 'rounded-full')
  })

  it('shares one pill shape across count variants', () => {
    render(
      <>
        <Badge variant="count">1</Badge>
        <Badge variant="count-attention">2</Badge>
        <Badge variant="attention">3</Badge>
      </>,
    )
    for (const t of ['1', '2', '3']) expect(screen.getByText(t)).toHaveClass('h-[18px]', 'min-w-5', 'rounded-full', 'tabular-nums')
    expect(screen.getByText('3')).toHaveClass('bg-attention-soft', 'text-attention-text')
  })

  it('lets a tone override the variant colour', () => {
    render(
      <>
        <Badge variant="priority" tone="attention">urgent</Badge>
        <Badge variant="count" tone="online">5 online</Badge>
      </>,
    )
    const urgent = screen.getByText('urgent')
    expect(urgent).toHaveClass('uppercase', 'bg-attention-soft', 'text-attention-text')
    expect(urgent).toHaveAttribute('data-tone', 'attention')
    const online = screen.getByText('5 online')
    expect(online).toHaveClass('bg-online-soft', 'text-online-text')
    expect(online).not.toHaveClass('bg-code')
  })

  it('marks the variant for styling and tests', () => {
    render(<Badge variant="health">x</Badge>)
    expect(screen.getByText('x')).toHaveAttribute('data-variant', 'health')
  })
})

describe('CountBadge', () => {
  it('renders nothing for zero or unknown counts', () => {
    const { container } = render(
      <>
        <CountBadge count={0} label="unread" />
        <CountBadge count={undefined} label="unread" />
      </>,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('names the count and picks the attention fill on request', () => {
    render(
      <>
        <CountBadge count={2} label="open" />
        <CountBadge count={7} label="pending" attention />
      </>,
    )
    expect(screen.getByLabelText('2 open')).toHaveAttribute('data-variant', 'count')
    expect(screen.getByLabelText('7 pending')).toHaveAttribute('data-variant', 'count-attention')
  })
})
