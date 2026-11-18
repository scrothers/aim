/** Remote-approval timeout for hints: whole minutes as "N min", else "N s". */
export function timeoutLabel(seconds: number): string {
  if (seconds % 60 === 0) return `${seconds / 60} min`
  return `${seconds} s`
}
