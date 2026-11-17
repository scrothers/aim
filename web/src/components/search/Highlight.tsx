import type { components } from '@/api/schema'

type Segment = components['schemas']['Segment']

/** Renders server-split snippet segments; matches get <mark>. Always text nodes, never HTML. */
export function Highlight({ segments }: { segments: ReadonlyArray<Segment> }) {
  return (
    <>
      {segments.map((s, i) =>
        s.match ? (
          <mark key={i} className="rounded-sm bg-accent-soft px-0.5 font-semibold text-accent-text">
            {s.text}
          </mark>
        ) : (
          <span key={i}>{s.text}</span>
        ),
      )}
    </>
  )
}
