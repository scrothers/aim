import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { pickOption } from '@/test/utils'
import { SelectField } from './select'

const OPTIONS = [
  { value: '', label: 'Any' },
  { value: 'low', label: 'Low' },
  { value: 'high', label: 'High' },
]

function Harness({ onChange }: { onChange?: (v: string) => void }) {
  const [v, setV] = useState('')
  return (
    <label>
      Priority
      <SelectField
        aria-label="Priority"
        value={v}
        onValueChange={(x) => {
          setV(x)
          onChange?.(x)
        }}
        options={OPTIONS}
      />
    </label>
  )
}

describe('SelectField', () => {
  it('is a labelled combobox, not a native select', () => {
    const { container } = render(<Harness />)
    const box = screen.getByRole('combobox', { name: 'Priority' })
    expect(box.tagName).toBe('BUTTON')
    expect(box).toHaveTextContent('Any')
    expect(container.querySelector('select')).toBeNull()
  })

  it('picks an option with the pointer and maps "" to the any option', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<Harness onChange={onChange} />)
    const box = screen.getByRole('combobox', { name: 'Priority' })
    await pickOption(user, box, 'High')
    expect(onChange).toHaveBeenLastCalledWith('high')
    expect(box).toHaveTextContent('High')
    await pickOption(user, box, 'Any')
    expect(onChange).toHaveBeenLastCalledWith('')
    expect(box).toHaveTextContent('Any')
  })

  it('works from the keyboard', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<Harness onChange={onChange} />)
    screen.getByRole('combobox', { name: 'Priority' }).focus()
    await user.keyboard('{Enter}')
    expect(await screen.findByRole('listbox')).toBeInTheDocument()
    await user.keyboard('{ArrowDown}{Enter}')
    expect(onChange).toHaveBeenLastCalledWith('low')
  })

  it('uses the shared field tokens', () => {
    render(<Harness />)
    const box = screen.getByRole('combobox', { name: 'Priority' })
    expect(box).toHaveClass('border-input', 'text-text', 'focus-visible:ring-2', 'focus-visible:ring-ring', 'data-[placeholder]:text-faint')
  })
})
