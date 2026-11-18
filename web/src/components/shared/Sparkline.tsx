export interface SparklineProps {
  values: ReadonlyArray<number> | null | undefined
  width?: number
  height?: number
}

/** Seven (or n) bars; the last bar is strong accent, earlier ones border-coloured. */
export function Sparkline({ values, width = 40, height = 14 }: SparklineProps) {
  const vals = values ?? []
  if (vals.length === 0) return null
  const gap = 2
  const bw = (width - gap * (vals.length - 1)) / vals.length
  const max = Math.max(1, ...vals)
  return (
    <svg
      role="img"
      aria-label={`activity, last ${vals.length} days: ${vals.join(', ')}`}
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
    >
      {vals.map((v, i) => {
        const h = v === 0 ? 2 : Math.max(2, Math.round((v / max) * height))
        return (
          <rect
            key={i}
            data-testid="spark-bar"
            data-last={i === vals.length - 1 ? 'true' : undefined}
            x={i * (bw + gap)}
            y={height - h}
            width={bw}
            height={h}
            rx={1}
            fill={i === vals.length - 1 ? 'var(--accent-strong)' : 'var(--border)'}
          />
        )
      })}
    </svg>
  )
}
