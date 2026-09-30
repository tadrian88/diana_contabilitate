// A single call often costs a fraction of a cent, so small amounts keep
// three significant digits instead of rounding to 0,00.
export function formatUsd(amount: number): string {
  const options: Intl.NumberFormatOptions = amount > 0 && amount < 0.01 ? { maximumSignificantDigits: 3 } : { minimumFractionDigits: 2, maximumFractionDigits: 2 }
  return `${new Intl.NumberFormat('ro-RO', options).format(amount)} USD`
}

export function formatTokens(value: number): string {
  return new Intl.NumberFormat('ro-RO').format(value)
}

export function formatDateTime(value: string): string {
  return new Intl.DateTimeFormat('ro-RO', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'Europe/Bucharest' }).format(new Date(value))
}
