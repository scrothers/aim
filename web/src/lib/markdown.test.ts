import { describe, expect, it } from 'vitest'
import { stripMarkdown } from './markdown'

describe('stripMarkdown', () => {
  it('removes emphasis, code and heading syntax', () => {
    expect(stripMarkdown('## Plan\nShip the **release train** with `aim sync` and _care_')).toBe('Plan Ship the release train with aim sync and care')
  })

  it('removes list markers, task boxes and quote markers', () => {
    expect(stripMarkdown('- one\n* two\n1. three\n- [x] four\n> quoted')).toBe('one two three four quoted')
  })

  it('flattens tables', () => {
    expect(stripMarkdown('| Name | Role |\n|------|:----:|\n| a | b |')).toBe('Name Role a b')
  })

  it('keeps link text and image alt text', () => {
    expect(stripMarkdown('see [the docs](https://x.example/a) ![logo](l.png)')).toBe('see the docs logo')
  })

  it('leaves snake_case alone', () => {
    expect(stripMarkdown('call my_func_name now')).toBe('call my_func_name now')
  })

  it('drops fences and rules but keeps code content', () => {
    expect(stripMarkdown('```go\nfmt.Println()\n```\n---\nend')).toBe('fmt.Println() end')
  })

  it('can keep edge spacing for joinable segments', () => {
    expect(stripMarkdown(' the `foo` ', { trim: false })).toBe(' the foo ')
  })
})
