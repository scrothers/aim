import { useId } from 'react'

// The AIM mark: a yellow speech bubble with a terminal prompt and a presence
// dot notched into its corner. Colors are fixed (the dark outline keeps it
// visible on dark panels) except the dot, which uses the buddy list's online
// color.
export function Logo({ size = 28 }: { size?: number }) {
  const mask = `aim-logo-${useId().replace(/[^\w-]/g, '')}`
  return (
    <svg aria-hidden="true" width={size} height={size} viewBox="0 0 32 32">
      <mask id={mask}>
        <rect width="32" height="32" fill="#fff" />
        <circle cx="26.5" cy="6.5" r="6" fill="#000" />
      </mask>
      <path
        mask={`url(#${mask})`}
        fill="#f5c518"
        stroke="#18181b"
        strokeWidth="1.5"
        strokeLinejoin="round"
        d="M8 3h16a6 6 0 0 1 6 6v10a6 6 0 0 1-6 6H14.5L7 30.5V25a6 6 0 0 1-5-5.9V9a6 6 0 0 1 6-6z"
      />
      <circle cx="26.5" cy="6.5" r="3.75" fill="var(--online)" />
      <path
        fill="none"
        stroke="#18181b"
        strokeWidth="2.75"
        strokeLinecap="round"
        strokeLinejoin="round"
        d="m8.5 10 4.5 4-4.5 4M16.5 18H23"
      />
    </svg>
  )
}
