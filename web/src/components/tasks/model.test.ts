import { describe, expect, it } from 'vitest'
import type { TaskView } from '@/api/tasks'
import { actionsFor, byPriority, dueLabel, eventText, fromDateInput, groupByStatus, isOverdue, planMove, resultStatus, STATUSES, toDateInput } from './model'

const t = (over: Partial<TaskView>): TaskView => ({
  id: 'task-1',
  title: 'T',
  body: '',
  status: 'not_started',
  priority: 'normal',
  created_by: 'operator',
  created_at: '2026-11-17T10:00:00Z',
  updated_at: '2026-11-17T10:00:00Z',
  ...over,
})

describe('planMove', () => {
  it('maps drags to lifecycle actions', () => {
    expect(planMove('not_started', 'started')).toEqual({ actions: ['start'] })
    expect(planMove('claimed', 'started')).toEqual({ actions: ['start'] })
    expect(planMove('abandoned', 'started')).toEqual({ actions: ['start'] })
    expect(planMove('started', 'finished')).toEqual({ actions: ['finish'] })
    expect(planMove('finished', 'not_started')).toEqual({ actions: ['reopen'] })
    expect(planMove('abandoned', 'not_started')).toEqual({ actions: ['reopen'] })
  })

  it('confirms moves that take work away or skip a step', () => {
    expect(planMove('started', 'not_started')).toMatchObject({ actions: ['release'], confirm: expect.any(String) })
    expect(planMove('claimed', 'not_started')).toMatchObject({ actions: ['release'], confirm: expect.any(String) })
    expect(planMove('not_started', 'claimed')).toMatchObject({ actions: ['claim'], confirm: expect.any(String) })
    expect(planMove('not_started', 'finished')).toMatchObject({ actions: ['start', 'finish'], confirm: expect.any(String) })
    expect(planMove('claimed', 'finished')).toMatchObject({ actions: ['start', 'finish'], confirm: expect.any(String) })
  })

  it('refuses abandoned and impossible moves, and ignores same-column drops', () => {
    expect(planMove('started', 'abandoned')).toHaveProperty('reject')
    expect(planMove('started', 'claimed')).toHaveProperty('reject')
    expect(planMove('finished', 'claimed')).toHaveProperty('reject')
    expect(planMove('started', 'started')).toBeNull()
  })

  it('knows where a plan ends', () => {
    expect(resultStatus(['claim'])).toBe('claimed')
    expect(resultStatus(['start', 'finish'])).toBe('finished')
    expect(resultStatus(['release'])).toBe('not_started')
    expect(resultStatus(['reopen'])).toBe('not_started')
  })
})

describe('actionsFor', () => {
  it('offers only valid actions per status', () => {
    expect(actionsFor('not_started')).toEqual(['start', 'claim'])
    expect(actionsFor('claimed')).toEqual(['start', 'release'])
    expect(actionsFor('started')).toEqual(['finish', 'release'])
    expect(actionsFor('finished')).toEqual(['reopen'])
    expect(actionsFor('abandoned')).toContain('claim')
  })
})

describe('grouping and ordering', () => {
  it('groups by status in board order, urgent first then newest', () => {
    const g = groupByStatus([
      t({ id: 'a', priority: 'low', updated_at: '2026-11-17T12:00:00Z' }),
      t({ id: 'b', priority: 'urgent', updated_at: '2026-11-17T09:00:00Z' }),
      t({ id: 'c', priority: 'urgent', updated_at: '2026-11-17T11:00:00Z' }),
      t({ id: 'd', status: 'started' }),
    ])
    expect(STATUSES).toEqual(['not_started', 'claimed', 'started', 'abandoned', 'finished'])
    expect(g.not_started.map((x) => x.id)).toEqual(['c', 'b', 'a'])
    expect(g.started.map((x) => x.id)).toEqual(['d'])
    expect(g.finished).toEqual([])
    expect(byPriority(t({ priority: 'high' }), t({ priority: 'normal' }))).toBeLessThan(0)
  })
})

describe('dates', () => {
  const now = Date.parse('2026-11-17T12:00:00Z')
  it('flags overdue open tasks only', () => {
    expect(isOverdue({ due_at: '2026-11-16T00:00:00Z', status: 'started' }, now)).toBe(true)
    expect(isOverdue({ due_at: '2026-11-18T00:00:00Z', status: 'started' }, now)).toBe(false)
    expect(isOverdue({ due_at: '2026-11-16T00:00:00Z', status: 'finished' }, now)).toBe(false)
    expect(isOverdue({ status: 'started' }, now)).toBe(false)
  })

  it('round-trips a due date through the date input at end of day', () => {
    const iso = fromDateInput('2026-12-05')!
    expect(toDateInput(iso)).toBe('2026-12-05')
    expect(new Date(iso).getHours()).toBe(23)
    expect(fromDateInput('')).toBeUndefined()
    expect(toDateInput(undefined)).toBe('')
    expect(dueLabel('not a date')).toBe('')
  })
})

describe('eventText', () => {
  it('describes status changes and assignments', () => {
    expect(eventText({ actor: 'operator', at: '', kind: 'start', from_status: 'not_started', to_status: 'started' })).toBe('started it (Not started to Started)')
    expect(eventText({ actor: 'operator', at: '', kind: 'assign', note: 'a@h/o' })).toBe('assigned it to a@h/o')
    expect(eventText({ actor: 'x', at: '', kind: 'comment', note: 'hi' })).toBe('commented')
  })
})
