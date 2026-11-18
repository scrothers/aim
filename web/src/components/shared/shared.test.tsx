import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { parseAddress, runtimeFor } from '@/lib/identity'
import { AddressChip } from './AddressChip'
import { RobotAvatar } from './RobotAvatar'
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

describe('RobotAvatar', () => {
  const addr = 'reviewer@github.com/acme/api'

  it('renders the full robot with a unique clipPath per instance', () => {
    const { container } = render(
      <>
        <RobotAvatar address={addr} size={40} />
        <RobotAvatar address={addr} size={40} />
      </>,
    )
    const ids = [...container.querySelectorAll('clipPath')].map((c) => c.id)
    expect(ids).toHaveLength(2)
    expect(new Set(ids).size).toBe(2)
    expect(container.querySelector('svg')).toHaveAttribute('viewBox', '0 0 64 64')
  })

  it('is decorative by default and labelled when asked', () => {
    const { container, rerender } = render(<RobotAvatar address={addr} />)
    expect(container.querySelector('svg')).toHaveAttribute('aria-hidden', 'true')
    rerender(<RobotAvatar address={addr} decorative={false} />)
    expect(screen.getByRole('img', { name: `Robot avatar for ${addr}` })).toBeInTheDocument()
  })

  it('uses the compact head crop at 24px and below', () => {
    const { container } = render(<RobotAvatar address={addr} size={18} />)
    expect(container.querySelector('svg')).toHaveAttribute('viewBox', '12 9 40 40')
    expect(container.querySelector('clipPath')).toBeNull()
  })

  it('overlays the runtime badge', () => {
    render(<RobotAvatar address={addr} client="codex-cli" />)
    expect(screen.getByAltText('Codex')).toBeInTheDocument()
  })
})

describe('operator console identity', () => {
  it('shows the console mark and no runtime badge for the operator', () => {
    render(<RobotAvatar address="operator" client="aim-web" />)
    expect(document.querySelector('svg')).toBeNull()
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

  // The badge chip is always white, so a white mark renders as an empty box.
  it('ships no white-filled runtime icons', () => {
    const svgs = import.meta.glob('../../assets/runtimes/*.svg', { eager: true, query: '?raw', import: 'default' }) as Record<string, string>
    expect(Object.keys(svgs).length).toBeGreaterThan(5)
    for (const [file, src] of Object.entries(svgs)) {
      expect(src, file).not.toMatch(/fill="(white|#fff|#ffffff)"/i)
    }
  })
})

describe('Sparkline', () => {
  it('renders one bar per value and marks the last as accent', () => {
    render(<Sparkline values={[0, 2, 4, 8, 1, 0, 3]} />)
    const bars = screen.getAllByTestId('spark-bar')
    expect(bars).toHaveLength(7)
    expect(bars[6]).toHaveAttribute('fill', 'var(--accent-strong)')
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
