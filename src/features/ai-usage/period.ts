import type { AIUsagePeriodInput } from '../../domain/ai-usage'

// Periods are Romanian calendar days, matching the backend's Europe/Bucharest bounds.
export type AIUsagePreset = 'CURRENT_MONTH' | 'PREVIOUS_MONTH' | 'CUSTOM'

const DAY = /^\d{4}-\d{2}-\d{2}$/

export function bucharestToday(now = new Date()): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'Europe/Bucharest', year: 'numeric', month: '2-digit', day: '2-digit' }).format(now)
}

export function currentMonthPeriod(now = new Date()): AIUsagePeriodInput {
  const today = bucharestToday(now)
  return { from: `${today.slice(0, 8)}01`, to: today }
}

export function previousMonthPeriod(now = new Date()): AIUsagePeriodInput {
  const [year, month] = bucharestToday(now).split('-').map(Number)
  const previousYear = month === 1 ? year - 1 : year
  const previousMonth = month === 1 ? 12 : month - 1
  const lastDay = new Date(Date.UTC(previousYear, previousMonth, 0)).getUTCDate()
  const prefix = `${previousYear}-${String(previousMonth).padStart(2, '0')}`
  return { from: `${prefix}-01`, to: `${prefix}-${String(lastDay).padStart(2, '0')}` }
}

export function validPeriod(period: AIUsagePeriodInput): boolean {
  return DAY.test(period.from) && DAY.test(period.to) && period.from <= period.to
}

export function formatDay(day: string): string {
  const [year, month, date] = day.split('-')
  return `${date}.${month}.${year}`
}
