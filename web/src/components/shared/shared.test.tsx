import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { hueFor, initialsFor, parseAddress, runtimeFor } from '@/lib/identity'
import { AddressChip } from './AddressChip'
import { AgentAvatar } from './AgentAvatar'
import { RuntimeBadge } from './RuntimeBadge'
import { Sparkline } from './Sparkline'

describe('AddressChip', () => {
  it('splits identity accent from muted path', () => {
    render(<AddressChip address="reviewer@github.com/acme/api" />)
    const chip = screen.getByTestId('address-chip')
    expect(chip).toHaveTextContent('reviewer@github.com/acme/api')
    expect(screen.getByText('github.com/acme/api')).toHaveClass('font-mono')
    expect(screen.getByText('reviewer', { exact: false })).toHaveClass('text-accent-text')
  })

  it('renders subagent addresses', () => {
    render(<AddressChip address="researcher@coordinator@github.com/acme" />)
    expect(screen.getByTestId('address-chip')).toHaveTextContent('researcher@coordinator@github.com/acme')
  })

  it('renders the operator without a path or @', () => {
    render(<AddressChip address="operator" />)
    expect(screen.getByTestId('address-chip')).toHaveTextContent(/^operator$/)
  })

  it('parses addresses', () => {
    expect(parseAddress('a@b@h.com/o')).toEqual({ subagent: 'a', identity: 'b', path: 'h.com/o' })
    expect(parseAddress('h.com/o/r')).toEqual({ path: 'h.com/o/r' })
  })
})

describe('AgentAvatar', () => {
  it('derives a deterministic hue per name', () => {
    expect(hueFor('reviewer')).toBe(hueFor('reviewer'))
    expect(hueFor('reviewer')).toBeGreaterThanOrEqual(0)
    expect(hueFor('reviewer')).toBeLessThan(360)
    expect(hueFor('reviewer')).not.toBe(hueFor('builder'))
  })

  it('computes initials', () => {
    expect(initialsFor('coordinator')).toBe('CO')
    expect(initialsFor('code-reviewer')).toBe('CR')
  })

  it('renders initials with the hue-based background', () => {
    const { container } = render(<AgentAvatar name="reviewer" />)
    expect(container.firstChild).toHaveTextContent('RE')
    expect((container.firstChild as HTMLElement).style.background).not.toBe('')
  })
})

describe('operator console identity', () => {
  it('shows the console mark and no runtime badge for the operator', () => {
    render(<AgentAvatar name="operator" client="aim-web" />)
    const mark = screen.getByTestId('console-mark')
    expect(mark).toHaveTextContent('>_')
    expect(mark).not.toHaveTextContent('aim-web')
    expect(screen.queryByTitle('Web console')).toBeNull()
  })

  it('maps the aim-web client to a plain "Web console" label without an icon', () => {
    expect(runtimeFor('aim-web')).toEqual({ label: 'Web console', console: true })
    const { container } = render(<RuntimeBadge client="aim-web" />)
    expect(container).toBeEmptyDOMElement()
  })
})

describe('runtime mapping', () => {
  it.each([
    ['claude-code', 'claude'],
    ['codex-cli', 'codex'],
    ['codex', 'codex'],
    ['gemini-cli', 'gemini'],
    ['opencode', 'opencode'],
    ['grok-build', 'grok'],
    ['cline', 'cline'],
    ['kilo-code', 'kilo-code'],
    ['mcp-inspector', 'model-context-protocol'],
    ['deepseek-tui', 'deepseek'],
  ])('maps %s to %s', (client, icon) => {
    expect(runtimeFor(client).icon).toBe(icon)
  })

  it('falls back to the raw name with no icon', () => {
    expect(runtimeFor('my-agent')).toEqual({ label: 'my-agent' })
  })

  it('renders an icon image for known clients', () => {
    render(<RuntimeBadge client="claude-code" />)
    expect(screen.getByRole('img', { name: 'Claude Code' })).toBeInTheDocument()
  })

  it('renders a text badge with the raw name for unknown clients', () => {
    render(<RuntimeBadge client="my-agent" />)
    expect(screen.getByText('my-agent')).toBeInTheDocument()
    expect(screen.queryByRole('img')).toBeNull()
  })
})

describe('Sparkline', () => {
  it('renders one bar per value and marks the last as accent', () => {
    render(<Sparkline values={[0, 2, 4, 8, 1, 0, 3]} />)
    const bars = screen.getAllByTestId('spark-bar')
    expect(bars).toHaveLength(7)
    expect(bars[6]).toHaveAttribute('fill', 'var(--accent)')
    expect(bars[0]).toHaveAttribute('fill', 'var(--border)')
    // tallest bar fills the height, zero bars stay a 2px stub
    expect(bars[3]).toHaveAttribute('height', '14')
    expect(bars[0]).toHaveAttribute('height', '2')
  })

  it('renders nothing without data', () => {
    const { container } = render(<Sparkline values={null} />)
    expect(container).toBeEmptyDOMElement()
  })
})
