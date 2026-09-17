import * as Dialog from '@radix-ui/react-dialog'
import { useState } from 'react'
import type { ClassificationReviewItem, DomainValue, Invoice } from '../../domain/invoice'
import { Button } from '../../components/ui/button'
import { DOMAIN_VALUE_LABELS } from './domain-decision-view'

export function DomainCorrectionDialog({ item, invoice, onCancel, onSubmit }: { item: ClassificationReviewItem; invoice: Invoice; onCancel: () => void; onSubmit: (value: DomainValue, reason: string) => void }) {
  const current = item.typedValue ?? item.proposedTypedValue
  const choices = item.dimension === 'ACCOUNT' ? ['ACCOUNT'] : item.dimension === 'VAT_TREATMENT' ? ['ORDINARY', 'SPECIAL_UNSUPPORTED'] : item.dimension === 'VAT_DEDUCTIBILITY' ? ['FULL', 'NONE', 'LIMITED', 'NOT_APPLICABLE'] : ['FULLY_DEDUCTIBLE', 'NONDEDUCTIBLE', 'LIMITED', 'PERIOD_LIMIT_CATEGORY', 'NOT_APPLICABLE']
  const [kind, setKind] = useState(current?.kind ?? choices[0])
  const [account, setAccount] = useState(current?.account ?? '')
  const [percentage, setPercentage] = useState(current?.percentage ?? '')
  const [basis, setBasis] = useState(current?.basis ?? '')
  const [category, setCategory] = useState(current?.category ?? '')
  const [reason, setReason] = useState(current?.reason ?? '')
  const [timing, setTiming] = useState(current?.timing ?? 'IMMEDIATE')
  const [error, setError] = useState('')
  const source = invoice.lines.find((line) => line.id === item.lineId)?.sourceFacts
  return <Dialog.Root open onOpenChange={(open) => !open && onCancel()}><Dialog.Portal><Dialog.Overlay className="fixed inset-0 z-40 bg-black/40" /><Dialog.Content className="fixed left-1/2 top-1/2 z-50 w-[min(95vw,32rem)] -translate-x-1/2 -translate-y-1/2"><form className="card max-h-[90vh] w-full space-y-4 overflow-auto p-6" onSubmit={(event) => {
    event.preventDefault()
    if (!reason.trim()) { setError('Completează motivul și documentele justificative.'); return }
    if (kind === 'ACCOUNT' && !/^[0-9]{3,}(?:\.[0-9A-Za-z]+)*$/.test(account)) { setError('Completează un cod de cont valid.'); return }
    if (kind === 'LIMITED' && (!/^\d+(?:\.\d{1,4})?$/.test(percentage) || !basis.trim())) { setError('Completează procentul exact și baza limitării.'); return }
    if (kind === 'LIMITED') { const [whole, fraction = ''] = percentage.split('.'); const exact = BigInt(whole) * 10000n + BigInt(fraction.padEnd(4, '0')); if (exact <= 0n || exact >= 1000000n) { setError('Procentul trebuie să fie mai mare de 0 și mai mic de 100.'); return } }
    if (kind === 'PERIOD_LIMIT_CATEGORY' && (!category.trim() || !basis.trim())) { setError('Completează categoria și baza plafonului.'); return }
    if (item.dimension === 'VAT_TREATMENT' && (!source?.rate || !source.code)) { setError('Sursa nu conține cota/categoria necesară. Faptele sursă nu se modifică prin clasificare.'); return }
    const value: DomainValue = { kind }
    if (kind === 'ACCOUNT') value.account = account
    if (kind === 'LIMITED') { value.percentage = percentage; value.basis = basis.trim() }
    if (kind === 'PERIOD_LIMIT_CATEGORY') { value.category = category.trim(); value.basis = basis.trim() }
    if (['NONE', 'NONDEDUCTIBLE', 'NOT_APPLICABLE', 'SPECIAL_UNSUPPORTED'].includes(kind)) value.reason = reason.trim()
    if (item.dimension === 'VAT_TREATMENT') { value.timing = timing; value.sourceRate = source?.rate; value.sourceCategory = source?.code }
    onSubmit(value, reason.trim())
  }}>
    <Dialog.Title className="font-bold">Corectează decizia contabilă</Dialog.Title>
    <Dialog.Description className="text-xs">Faptele e-Factura rămân nemodificate. Decizia se aplică acestei facturi.</Dialog.Description>
    {item.dimension !== 'ACCOUNT' && <div className="block text-sm"><label htmlFor="domain-decision">Decizie</label><select id="domain-decision" className="mt-1 w-full rounded border p-2" value={kind} onChange={(e) => setKind(e.target.value)}>{choices.map((choice) => <option key={choice} value={choice}>{DOMAIN_VALUE_LABELS[choice]}</option>)}</select></div>}
    {kind === 'ACCOUNT' && <Field label="Cont contabil / analitic" value={account} onChange={setAccount} />}
    {kind === 'LIMITED' && <><Field label="Procent deductibil (%) — separator punct" value={percentage} onChange={setPercentage} /><Field label="Baza legală / utilizarea justificată" value={basis} onChange={setBasis} /></>}
    {kind === 'PERIOD_LIMIT_CATEGORY' && <><Field label="Categoria plafonului fiscal" value={category} onChange={setCategory} /><Field label="Baza plafonului" value={basis} onChange={setBasis} /><p className="text-xs">Diana nu calculează plafonul perioadei.</p></>}
    {item.dimension === 'VAT_TREATMENT' && <div className="block text-sm"><label htmlFor="domain-timing">Momentul deducerii</label><select id="domain-timing" className="mt-1 w-full rounded border p-2" value={timing} onChange={(e) => setTiming(e.target.value)}><option value="IMMEDIATE">La înregistrare — domeniu obișnuit</option><option value="DEFERRED">Amânat — necesită verificare</option><option value="UNSUPPORTED">Tratament neacceptat de maparea inițială</option></select></div>}
    <Field label="Motiv / documente justificative" value={reason} onChange={setReason} />
    {error && <p role="alert" className="text-sm text-[var(--danger)]">{error}</p>}
    <div className="flex justify-end gap-3"><Button type="button" variant="ghost" onClick={onCancel}>Anulează</Button><Button type="submit">Salvează decizia</Button></div>
  </form></Dialog.Content></Dialog.Portal></Dialog.Root>
}
function Field({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) { return <label className="block text-sm">{label}<input className="mt-1 w-full rounded border p-2" value={value} onChange={(e) => onChange(e.target.value)} /></label> }
