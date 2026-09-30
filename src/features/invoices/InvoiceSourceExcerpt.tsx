import type { ReactNode } from 'react'
import type { Invoice } from '../../domain/invoice'
import { lineItemDescription, unitLabel, type InvoiceFocus } from './commercial-view'
import { counterparty } from './invoice-view'

const mark = 'rounded-[3px] bg-[var(--evidence-mark)] px-1 ring-2 ring-[var(--evidence-ring)] [box-decoration-break:clone]'

function Marked({ on, children }: { on: boolean; children: ReactNode }) {
  return on ? <mark data-testid="invoice-highlight" className={`${mark} text-[var(--text)]`}>{children}</mark> : <>{children}</>
}

// Marks `text` inside `value` when present, else leaves the value as it is.
function MarkedText({ value, text }: { value: string; text?: string }) {
  const index = text ? value.toLocaleLowerCase('ro-RO').indexOf(text.toLocaleLowerCase('ro-RO')) : -1
  if (!text || index < 0) return <>{value}</>
  return <>{value.slice(0, index)}<Marked on>{value.slice(index, index + text.length)}</Marked>{value.slice(index + text.length)}</>
}

const money = (amount: number, currency: string) => new Intl.NumberFormat('ro-RO', { style: 'currency', currency }).format(amount)
const plain = (amount: number) => new Intl.NumberFormat('ro-RO', { minimumFractionDigits: 2, maximumFractionDigits: 4 }).format(amount)
const date = (value?: string) => value ? new Intl.DateTimeFormat('ro-RO', { dateStyle: 'short', timeZone: 'Europe/Bucharest' }).format(new Date(value)) : '—'

// The part of the e-Factura a check relies on: the invoice header and its
// lines, with the compared line and value marked and the XML path it was read
// from. Unrelated lines stay visible but muted so the context is kept.
export function InvoiceSourceExcerpt({ invoice, focus }: { invoice: Invoice; focus: InvoiceFocus }) {
  const focused = new Set(focus.lineIds)
  const lines = [...invoice.lines].sort((left, right) => left.position - right.position)
  const period = invoice.sourceFacts?.periodStart || invoice.sourceFacts?.periodEnd ? `${date(invoice.sourceFacts?.periodStart)} – ${date(invoice.sourceFacts?.periodEnd)}` : undefined
  const facts: Array<[string, ReactNode]> = [
    [counterparty(invoice).role, <>{counterparty(invoice).name}{counterparty(invoice).identifier ? <span className="font-normal text-[var(--text-muted)]"> · {counterparty(invoice).identifier}</span> : null}</>],
    ['Număr factură', invoice.documentNumber],
    ['Data emiterii', <Marked on={focus.header === 'dueDate'}>{date(invoice.issueDate)}</Marked>],
    ['Scadența', <Marked on={focus.header === 'dueDate'}>{date(invoice.dueDate)}</Marked>],
    ...(period ? [['Perioada facturată', period] as [string, ReactNode]] : []),
    ['Total de plată', money(invoice.total.amount, invoice.total.currency)],
    ...(focus.header === 'reference' && focus.text ? [['Referință contract', <Marked on>{focus.text}</Marked>] as [string, ReactNode]] : []),
  ]
  return (
    <div className="flex h-full min-h-0 flex-col gap-5 overflow-auto p-5">
      <dl className="grid grid-cols-2 gap-x-5 gap-y-3 sm:grid-cols-3">
        {facts.map(([label, value]) => <div key={label}><dt className="text-xs text-[var(--text-muted)]">{label}</dt><dd className="mt-0.5 text-sm font-semibold">{value}</dd></div>)}
      </dl>
      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-left text-sm">
          <caption className="sr-only">Liniile facturii din e-Factura</caption>
          <thead className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]">
            <tr><th className="border-b border-[var(--border-strong)] px-2 py-2">#</th><th className="border-b border-[var(--border-strong)] px-2 py-2">Denumire</th><th className="border-b border-[var(--border-strong)] px-2 py-2">UM</th><th className="border-b border-[var(--border-strong)] px-2 py-2 text-right">Cant.</th><th className="border-b border-[var(--border-strong)] px-2 py-2 text-right">Preț unitar</th><th className="border-b border-[var(--border-strong)] px-2 py-2 text-right">TVA</th><th className="border-b border-[var(--border-strong)] px-2 py-2 text-right">Valoare</th></tr>
          </thead>
          <tbody>
            {lines.map((line) => {
              const on = focused.has(line.id)
              const muted = focused.size > 0 && !on && focus.field !== 'vat'
              const description = lineItemDescription(line)
              const describe = on && focus.field === 'description'
              return (
                <tr key={line.id} data-focused={on || undefined} className={`${on ? 'bg-[var(--evidence-row)]' : ''} ${muted || (focus.header && !focused.size) ? 'opacity-40' : ''}`}>
                  <td className="border-b border-[var(--border)] px-2 py-3 align-top">{line.position}</td>
                  <td className="border-b border-[var(--border)] px-2 py-3 align-top">
                    <div className="font-semibold">{describe && !focus.text ? <Marked on>{line.description}</Marked> : <MarkedText value={line.description} text={describe ? focus.text : undefined} />}</div>
                    {description && <div className="mt-0.5 text-xs text-[var(--text-secondary)]"><MarkedText value={description} text={describe ? focus.text : undefined} /></div>}
                  </td>
                  <td className="border-b border-[var(--border)] px-2 py-3 align-top whitespace-nowrap">{unitLabel(line.unit)}</td>
                  <td className="border-b border-[var(--border)] px-2 py-3 text-right align-top tabular-nums"><Marked on={on && focus.field === 'quantity'}>{line.quantity}</Marked></td>
                  <td className="border-b border-[var(--border)] px-2 py-3 text-right align-top whitespace-nowrap tabular-nums"><Marked on={on && focus.field === 'unitPrice'}>{plain(line.unitPrice.amount)} {line.unitPrice.currency}</Marked></td>
                  <td className="border-b border-[var(--border)] px-2 py-3 text-right align-top whitespace-nowrap"><Marked on={on && focus.field === 'vat'}>{line.vatLabel}</Marked></td>
                  <td className="border-b border-[var(--border)] px-2 py-3 text-right align-top whitespace-nowrap tabular-nums">{plain(line.netValue.amount)}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      {focus.sourcePath && <div className="mt-auto rounded-lg border border-dashed border-[var(--border-strong)] bg-[var(--surface-subtle)] px-3 py-2.5"><div className="text-xs text-[var(--text-muted)]">Valoarea de pe factură provine din e-Factura (XML SPV)</div><code className="mt-1 block whitespace-pre-wrap break-all font-mono text-xs">{focus.sourcePath}</code></div>}
    </div>
  )
}
