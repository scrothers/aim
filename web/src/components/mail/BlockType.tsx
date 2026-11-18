import { BlockTypeSelect, currentBlockType$, Select, useCellValue } from '@mdxeditor/editor'

const KNOWN = /^(paragraph|quote|h[1-6])$/

const LABEL: Record<string, string> = { list: 'List', listitem: 'List', table: 'Table', code: 'Code block', codeblock: 'Code block' }

/**
 * MDXEditor's block type select shows nothing when the cursor is in a block it
 * has no item for (a list reports "list"). In that case show a one-item select
 * naming the block; otherwise defer to the stock control.
 */
export function BlockType() {
  const type = useCellValue(currentBlockType$)
  if (!type || KNOWN.test(type)) return <BlockTypeSelect />
  const label = LABEL[type] ?? type.charAt(0).toUpperCase() + type.slice(1)
  return <Select value={type} onChange={() => {}} triggerTitle="Block type" placeholder="Block type" items={[{ label, value: type }]} />
}
