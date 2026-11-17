export function Logo({ size = 28 }: { size?: number }) {
  return (
    <svg aria-hidden="true" width={size} height={size} viewBox="0 0 28 28">
      <rect width="28" height="28" rx="8" fill="var(--accent)" />
      <path
        d="M8 9.5h12a1.5 1.5 0 0 1 1.5 1.5v6a1.5 1.5 0 0 1-1.5 1.5h-6l-3.5 3v-3H8A1.5 1.5 0 0 1 6.5 17v-6A1.5 1.5 0 0 1 8 9.5z"
        fill="none"
        stroke="var(--on-accent)"
        strokeWidth="1.6"
        strokeLinejoin="round"
      />
      <path d="M9.5 12.5l4.5 2.5 4.5-2.5" fill="none" stroke="var(--on-accent)" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}
