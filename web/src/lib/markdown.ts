/**
 * Flattens Markdown to plain text for one-line previews and search snippets.
 * Not a parser: it removes the syntax characters people notice (backticks,
 * emphasis markers, heading hashes, list markers, quote markers, table pipes,
 * link and image wrappers) and collapses whitespace. Output is for text nodes only.
 *
 * `trim: false` keeps leading and trailing space, so segments of one snippet can
 * be stripped separately and still join up.
 */
export function stripMarkdown(src: string, opts: { trim?: boolean } = {}): string {
  let s = src
  // Fences and rules vanish; table separator rows (|---|:--:|) carry no content.
  s = s.replace(/^[ \t]*(```|~~~)[^\n]*$/gm, ' ')
  s = s.replace(/^[ \t]*\|?[ \t]*:?-{2,}:?[ \t]*(\|[ \t]*:?-{2,}:?[ \t]*)*\|?[ \t]*$/gm, ' ')
  s = s.replace(/^[ \t]*([-*_])([ \t]*\1){2,}[ \t]*$/gm, ' ')
  // Line-leading markers: headings, quotes, bullets, numbers, task boxes.
  s = s.replace(/^[ \t]*#{1,6}[ \t]+/gm, '')
  s = s.replace(/^[ \t]*(>[ \t]?)+/gm, '')
  s = s.replace(/^[ \t]*([-*+]|\d+[.)])[ \t]+(\[[ xX]\][ \t]+)?/gm, '')
  // Images and links keep their visible text.
  s = s.replace(/!\[([^\]]*)\]\([^)]*\)/g, '$1')
  s = s.replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
  // Inline code and emphasis.
  s = s.replace(/`+/g, '')
  s = s.replace(/(\*\*|__|~~)/g, '')
  s = s.replace(/\*/g, '')
  s = s.replace(/(^|[^\p{L}\p{N}])_+|_+(?=$|[^\p{L}\p{N}])/gu, '$1')
  // Table pipes become spacing.
  s = s.replace(/[ \t]*\|[ \t]*/g, ' ')
  s = s.replace(/\s+/g, ' ')
  return opts.trim === false ? s : s.trim()
}
