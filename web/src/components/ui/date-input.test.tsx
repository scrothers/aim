import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DateInput } from './date-input'

describe('DateInput', () => {
  it('stays a native, labelled date field', () => {
    const onChange = vi.fn()
    render(<DateInput aria-label="Due date" value="" onChange={onChange} />)
    const input = screen.getByLabelText('Due date')
    expect(input).toHaveAttribute('type', 'date')
    fireEvent.change(input, { target: { value: '2026-12-05' } })
    expect(onChange).toHaveBeenCalled()
  })

  it('marks an empty field so its mask uses the placeholder colour', () => {
    const { rerender } = render(<DateInput aria-label="Due date" value="" onChange={() => {}} />)
    expect(screen.getByLabelText('Due date')).toHaveAttribute('data-empty')
    rerender(<DateInput aria-label="Due date" value="2026-12-05" onChange={() => {}} />)
    expect(screen.getByLabelText('Due date')).not.toHaveAttribute('data-empty')
  })
})
