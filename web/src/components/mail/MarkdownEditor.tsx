import { useEffect, useRef } from 'react'
import {
  BlockTypeSelect,
  BoldItalicUnderlineToggles,
  codeBlockPlugin,
  CodeToggle,
  codeMirrorPlugin,
  CreateLink,
  diffSourcePlugin,
  DiffSourceToggleWrapper,
  headingsPlugin,
  InsertCodeBlock,
  InsertTable,
  InsertThematicBreak,
  linkDialogPlugin,
  linkPlugin,
  listsPlugin,
  ListsToggle,
  markdownShortcutPlugin,
  MDXEditor,
  quotePlugin,
  Separator,
  tablePlugin,
  thematicBreakPlugin,
  toolbarPlugin,
  UndoRedo,
} from '@mdxeditor/editor'
import '@mdxeditor/editor/style.css'
import './markdown-editor.css'

export interface MarkdownEditorProps {
  value: string
  onChange: (markdown: string) => void
  label?: string
  placeholder?: string
  autoFocus?: boolean
}

const LANGUAGES = {
  '': 'Plain text',
  bash: 'Shell',
  css: 'CSS',
  diff: 'Diff',
  go: 'Go',
  html: 'HTML',
  js: 'JavaScript',
  json: 'JSON',
  py: 'Python',
  rust: 'Rust',
  sql: 'SQL',
  ts: 'TypeScript',
  tsx: 'TSX',
  yaml: 'YAML',
}

// Plugins are stateless factories; build them once.
const plugins = [
  headingsPlugin(),
  listsPlugin(),
  quotePlugin(),
  thematicBreakPlugin(),
  linkPlugin(),
  linkDialogPlugin(),
  tablePlugin(),
  codeBlockPlugin({ defaultCodeBlockLanguage: '' }),
  codeMirrorPlugin({ codeBlockLanguages: LANGUAGES }),
  markdownShortcutPlugin(),
  diffSourcePlugin({ viewMode: 'rich-text' }),
  toolbarPlugin({
    toolbarContents: () => (
      <DiffSourceToggleWrapper options={['rich-text', 'source']}>
        <UndoRedo />
        <Separator />
        <BlockTypeSelect />
        <BoldItalicUnderlineToggles options={['Bold', 'Italic']} />
        <CodeToggle />
        <Separator />
        <ListsToggle options={['bullet', 'number']} />
        <CreateLink />
        <InsertTable />
        <InsertCodeBlock />
        <InsertThematicBreak />
      </DiffSourceToggleWrapper>
    ),
  }),
]

/** Markdown WYSIWYG editor. The value in and out is always a Markdown string. */
export default function MarkdownEditor({ value, onChange, label = 'Message body', placeholder, autoFocus }: MarkdownEditorProps) {
  const root = useRef<HTMLDivElement>(null)

  // MDXEditor has no aria-label prop: label its editable surfaces (rich text
  // and the CodeMirror source view) as they mount.
  useEffect(() => {
    const el = root.current
    if (!el) return
    const apply = () => {
      for (const t of el.querySelectorAll<HTMLElement>('[contenteditable="true"]')) {
        if (!t.closest('[data-lexical-decorator]') && t.getAttribute('aria-label') !== label) {
          t.setAttribute('role', 'textbox')
          t.setAttribute('aria-multiline', 'true')
          t.setAttribute('aria-label', label) // replaces MDXEditor's "editable markdown"
        }
      }
    }
    apply()
    const mo = new MutationObserver(apply)
    mo.observe(el, { childList: true, subtree: true })
    return () => mo.disconnect()
  }, [label])

  return (
    <div ref={root}>
      <MDXEditor
        className="aim-editor"
        contentEditableClassName="prose-aim aim-editor-content"
        markdown={value}
        onChange={(md) => onChange(md)}
        plugins={plugins}
        placeholder={<span className="aim-editor-placeholder">{placeholder}</span>}
        autoFocus={autoFocus ? { defaultSelection: 'rootEnd' } : false}
      />
    </div>
  )
}
