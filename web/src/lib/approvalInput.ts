export type InputView =
  | { kind: 'command'; command: string }
  | { kind: 'file'; path: string; note?: string }
  | { kind: 'truncated'; bytes: number; preview: string }
  | { kind: 'json'; text: string }
  | { kind: 'none' }

const MAX_JSON = 2000

const isRec = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v)
const str = (v: unknown): string | undefined => (typeof v === 'string' ? v : undefined)

const plural = (n: number, w: string) => `${n} ${w}${n === 1 ? '' : 's'}`
const lines = (s: string) => (s === '' ? 0 : s.split('\n').length)

function fileNote(input: Record<string, unknown>): string | undefined {
  const oldS = str(input.old_string)
  const newS = str(input.new_string)
  if (oldS !== undefined && newS !== undefined) return `replace ${plural(oldS.length, 'char')} with ${plural(newS.length, 'char')}`
  const content = str(input.content)
  if (content !== undefined) return plural(lines(content), 'line')
  if (Array.isArray(input.edits)) return plural(input.edits.length, 'edit')
  const src = str(input.new_source)
  if (src !== undefined) return `${str(input.edit_mode) ?? 'replace'} cell, ${plural(lines(src), 'line')}`
  return undefined
}

/** Turns a tool-call input into a safe, readable description (rendered as text only). */
export function describeInput(input: unknown): InputView {
  if (input === undefined || input === null) return { kind: 'none' }
  if (isRec(input)) {
    if (input.truncated === true && typeof input.bytes === 'number') {
      return { kind: 'truncated', bytes: input.bytes, preview: str(input.preview) ?? '' }
    }
    const command = str(input.command)
    if (command !== undefined) return { kind: 'command', command }
    // file_path/notebook_path: Claude Code; filePath: OpenCode.
    const path = str(input.file_path) ?? str(input.notebook_path) ?? str(input.filePath)
    if (path !== undefined) return { kind: 'file', path, note: fileNote(input) }
  }
  let text: string
  try {
    text = JSON.stringify(input, null, 2) ?? String(input)
  } catch {
    text = String(input)
  }
  return { kind: 'json', text: text.length > MAX_JSON ? `${text.slice(0, MAX_JSON)}\n…` : text }
}
