import type { Invoice } from '../../domain/invoice'
import { counterparty, isIssued, vatCashNoteWarning } from './invoice-view'

const received = { supplierName: 'Furnizor SRL', supplierCui: 'RO37403193' } as Pick<Invoice, 'direction' | 'supplierName' | 'supplierCui' | 'customerName' | 'customerIdentifier'>
const issued = { ...received, direction: 'OUTGOING' as const, supplierName: 'Emitent SRL', customerName: 'Persoană Fizică', customerIdentifier: '180***' }

describe('invoice direction helpers (D-124, D-128)', () => {
  it('names the supplier of a received invoice and the customer of an issued one', () => {
    expect(isIssued(received)).toBe(false)
    expect(counterparty(received)).toEqual({ role: 'Furnizor', name: 'Furnizor SRL', identifier: 'RO37403193' })
    expect(isIssued(issued)).toBe(true)
    expect(counterparty(issued)).toEqual({ role: 'Client', name: 'Persoană Fizică', identifier: '180***' })
  })
  it('warns only when an issued invoice note contradicts the client profile', () => {
    const base = { direction: 'OUTGOING' as const, sourceFacts: { parserVersion: 'V', cashAccounting: 'YES' }, accountingSnapshot: { profile: { id: 'p', clientId: 'c', version: 1, chartPolicy: 'x', cashAccounting: 'NO' } } }
    expect(vatCashNoteWarning(base)).toContain('TVA la încasare')
    expect(vatCashNoteWarning({ ...base, direction: 'INCOMING' })).toBeUndefined()
    expect(vatCashNoteWarning({ ...base, accountingSnapshot: { profile: { ...base.accountingSnapshot.profile, cashAccounting: 'YES' } } })).toBeUndefined()
    expect(vatCashNoteWarning({ ...base, sourceFacts: { parserVersion: 'V', cashAccounting: 'NO' } })).toBeUndefined()
  })
})
