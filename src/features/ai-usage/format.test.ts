import { describe, expect, it } from 'vitest'
import { formatTokens, formatUsd } from './format'
import { currentMonthPeriod, formatDay, previousMonthPeriod, validPeriod } from './period'

describe('AI usage formatting', () => {
  it('keeps sub-cent costs visible instead of rounding them to zero', () => {
    expect(formatUsd(0.0071625)).toBe('0,00716 USD')
    expect(formatUsd(0.00000275)).toBe('0,00000275 USD')
    expect(formatUsd(0)).toBe('0,00 USD')
    expect(formatUsd(12.5)).toBe('12,50 USD')
    expect(formatTokens(1234567)).toBe('1.234.567')
  })
})

describe('AI usage periods', () => {
  it('uses Romanian calendar days for month presets', () => {
    // 22:30 UTC on 31 October is already 1 November in Bucharest.
    const lateOctober = new Date('2026-10-31T22:30:00Z')
    expect(currentMonthPeriod(lateOctober)).toEqual({ from: '2026-11-01', to: '2026-11-01' })
    expect(previousMonthPeriod(lateOctober)).toEqual({ from: '2026-10-01', to: '2026-10-31' })
    expect(previousMonthPeriod(new Date('2026-01-15T10:00:00Z'))).toEqual({ from: '2025-12-01', to: '2025-12-31' })
    expect(previousMonthPeriod(new Date('2028-03-10T10:00:00Z'))).toEqual({ from: '2028-02-01', to: '2028-02-29' })
  })

  it('validates custom ranges', () => {
    expect(validPeriod({ from: '2026-09-01', to: '2026-09-30' })).toBe(true)
    expect(validPeriod({ from: '2026-09-30', to: '2026-09-01' })).toBe(false)
    expect(validPeriod({ from: '', to: '2026-09-01' })).toBe(false)
    expect(formatDay('2026-09-01')).toBe('01.09.2026')
  })
})
