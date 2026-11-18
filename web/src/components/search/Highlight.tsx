import type { components } from '@/api/schema'
import { stripMarkdown } from '@/lib/markdown'

type Segment = components['schemas']['Segment']

/**
 * Renders server-split snippet segments; matches get <mark>. Always text nodes, never HTML.
 * Markdown syntax is stripped per segment (edges kept, so segments still join up).
 */
export function Highlight({ segments }: { segments: ReadonlyArray<Segment> }) {
  return (
    <>
      {segments.map((s, i) => {
        const text = stripMarkdown(s.text, { trim: false })
        return s.match ? (
          <mark key={i} className="rounded-sm bg-accent-soft px-0.5 font-semibold text-accent-text">
            {text}
          </mark>
        ) : (
          <span key={i}>{text}</span>
        )
      })}
    </>
  )
}
