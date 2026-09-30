import type { Invoice, InvoiceLine } from '../../domain/invoice'
import type { CommercialEvidence, CommercialFinding, CommercialOutcome, CommercialValidationRun } from '../../repositories/invoiceRepository'

// View model of a commercial validation run as an accountant reads it: checks
// that concern the whole invoice first, then one group per invoice line, each
// saying what the invoice states, what the contract states and the verdict.

export type CheckKind = 'comparison' | 'mapping' | 'input' | 'notice'

export interface CommercialCheck {
  id: string
  code: string
  kind: CheckKind
  outcome: CommercialOutcome
  findings: CommercialFinding[]
  lineIds: string[]
  title: string
  scope: string
  invoiceValue?: string
  invoiceDetail?: string
  contractValue?: string
  contractDetail?: string
  explanation?: string
  evidence: CommercialEvidence[]
}

export interface CommercialLineGroup { line: InvoiceLine; checks: CommercialCheck[]; serviceLabel?: string }

export interface CommercialView {
  invoiceChecks: CommercialCheck[]
  lineGroups: CommercialLineGroup[]
  // Comparisons that can be opened side by side, in reading order.
  verifiable: CommercialCheck[]
  // Uncovered lines for which the contract offers services to choose from.
  mappings: Array<{ line: InvoiceLine; finding: CommercialFinding }>
  pendingCount: number
  checkCount: number
}

const titles: Array<[RegExp, string]> = [
  [/^CONTRACT_REFERENCE_/, 'Referința contractului'],
  [/^PAYMENT_DUE_DATE_MISSING$/, 'Scadența lipsește de pe factură'],
  [/^PAYMENT_DUE_/, 'Scadența'],
  [/^VAT_RATE_/, 'Cota TVA'],
  [/^(PRICE_|EXPECTED_PRICE_INVALID)/, 'Preț unitar'],
  [/^(CURRENCY_MISMATCH|LINE_PRICE_CURRENCY_MISSING)$/, 'Moneda prețului'],
  [/^UNIT_QUANTITY_SOURCE_MISSING$/, 'Lipsește dovada cantității facturate'],
  [/^UNIT_QUANTITY_/, 'Cantitate facturată'],
  [/^SERVICE_LINE_UNCOVERED$/, 'Serviciu neasociat'],
  [/^SERVICE_LINE_AMBIGUOUS$/, 'Linia se potrivește cu mai multe servicii'],
  [/^CONTRACT_COVERAGE_INCOMPLETE$/, 'Unele condiții contractuale nu pot fi încă verificate'],
  [/^CONTRACT_NOT_EFFECTIVE$/, 'Contractul nu este valabil la data facturii'],
  [/^CONTRACT_WAIVED$/, 'Continuat fără contract'],
  [/^ORIGINAL_INVOICE_UNAVAILABLE$/, 'Factura inițială pentru storno nu este disponibilă'],
  [/^RULE_INPUT_MISSING$/, 'Lipsesc date necesare calculului'],
  [/^RULE_INPUT_OUTSIDE_VALIDITY$/, 'Informația este salvată, dar nu este valabilă la data facturii'],
  [/^RULE_DATE_BASIS_MISSING$/, 'Lipsește data de la care începe termenul de plată'],
  [/^SUPPLIER_IDENTITY_/, 'Identitatea furnizorului'],
  [/^(FREQUENCY_|SERVICE_PERIOD_)/, 'Periodicitatea facturării'],
  [/^CREDIT_NOTE_/, 'Storno'],
]

const explanations: Record<string, string> = {
  CONTRACT_COVERAGE_INCOMPLETE: 'Unele clauze nu au o valoare verificabilă din contract. Tarifele confirmate sunt totuși comparate separat.',
  CONTRACT_WAIVED: 'Contabilul a decis, cu motiv, continuarea fără contract. Factura nu este verificată față de un contract.',
  SERVICE_LINE_AMBIGUOUS: 'Mai multe servicii din contract folosesc aceeași formulare. Clarifică denumirile serviciilor în contract.',
  RULE_DATE_BASIS_MISSING: 'Contractul stabilește termenul de plată de la transmiterea, primirea sau acceptarea facturii. Completează data evenimentului cerut și dovada ei; data importului nu o înlocuiește.',
  RULE_INPUT_OUTSIDE_VALIDITY: 'Valoarea completată anterior a fost salvată, însă intervalul ales nu include data acestei facturi. Corectează perioada de valabilitate, fără să schimbi valoarea sau sursa dacă acestea sunt corecte.',
}

const comparisonCodes = /^(CONTRACT_REFERENCE_(MATCH|MISMATCH)|PAYMENT_DUE_(MATCH|MISMATCH)|VAT_RATE_|PRICE_(MATCH|MISMATCH)|EXPECTED_PRICE_INVALID|CURRENCY_MISMATCH|UNIT_QUANTITY_(MATCH|MISMATCH)|SUPPLIER_IDENTITY_(MATCH|MISMATCH)|CONTRACT_NOT_EFFECTIVE|FREQUENCY_(MATCH|MISMATCH))/
const inputCodes = new Set(['RULE_INPUT_MISSING', 'RULE_INPUT_OUTSIDE_VALIDITY', 'UNIT_QUANTITY_SOURCE_MISSING', 'RULE_DATE_BASIS_MISSING'])

const unitNames: Record<string, string> = { MON: 'lună', HUR: 'oră', DAY: 'zi', ANN: 'an', C62: 'buc.', H87: 'buc.', EA: 'buc.', KGM: 'kg', MTR: 'm', LTR: 'l' }

export function unitLabel(code: string | undefined) {
  if (!code) return ''
  const name = unitNames[code.toUpperCase()]
  return name ? `${name} (${code})` : code
}

const numberFormat = new Intl.NumberFormat('ro-RO', { minimumFractionDigits: 2, maximumFractionDigits: 4 })
const rateFormat = new Intl.NumberFormat('ro-RO', { maximumFractionDigits: 4 })

function decimal(value: string | undefined) {
  if (value === undefined || value.trim() === '' || !/^-?\d+(\.\d+)?$/.test(value.trim())) return undefined
  return Number(value)
}

export function formatAmount(value: string | undefined, currency?: string) {
  const number = decimal(value)
  if (number === undefined) return value
  return `${numberFormat.format(number)}${currency ? ` ${currency}` : ''}`
}

export function formatRate(value: string | undefined) {
  const number = decimal(value)
  return number === undefined ? value : `${rateFormat.format(number)}%`
}

// The service a price finding checked against: the rule narrative of a
// reviewed service price reads "<service description> · <price> <currency>".
export function serviceNameFromCalculation(calculation: string | undefined) {
  if (!calculation) return undefined
  const index = calculation.indexOf(' · ')
  return index > 0 ? calculation.slice(0, index) : undefined
}

export function linePath(line: InvoiceLine) {
  return line.sourceFacts?.path || `/Invoice/InvoiceLine[${line.position}]`
}

export function lineItemDescription(line: InvoiceLine) {
  if (line.sourceFacts?.itemDescription) return line.sourceFacts.itemDescription
  const match = line.additionalInfo?.match(/item_description=([^;]*)/)
  return match?.[1]?.trim() || undefined
}

function lineRate(line: InvoiceLine | undefined) {
  if (!line) return undefined
  const match = line.vatLabel.match(/^(-?\d+(?:[.,]\d+)?)%$/)
  return match ? Number(match[1].replace(',', '.')) : undefined
}

// Runs computed by COMMERCIAL_VALIDATION_V1 reported the contract's VAT
// clause as a price check (PRICE_MATCH comparing VAT rates). Such findings
// are shown for what they are: the invoice value equals the line's VAT rate
// and the clause is about VAT.
function reinterpretLegacyVAT(finding: CommercialFinding, lines: Map<string, InvoiceLine>): CommercialFinding {
  if (!/^PRICE_(MATCH|MISMATCH)$/.test(finding.code) || !finding.lineId) return finding
  const text = `${finding.calculation ?? ''} ${(finding.evidence ?? []).map((item) => item.snippet).join(' ')}`
  const rate = lineRate(lines.get(finding.lineId))
  if (!/\bTVA\b/i.test(text) || rate === undefined || decimal(finding.actual) !== rate) return finding
  return { ...finding, code: finding.code === 'PRICE_MATCH' ? 'VAT_RATE_MATCH' : 'VAT_RATE_MISMATCH', actual: String(rate), expected: String(decimal(finding.expected) ?? finding.expected) }
}

function kindOf(code: string, finding: CommercialFinding): CheckKind {
  if (code === 'SERVICE_LINE_UNCOVERED' || code === 'SERVICE_LINE_AMBIGUOUS') return 'mapping'
  if (inputCodes.has(code)) return 'input'
  if (comparisonCodes.test(code) && (finding.actual || finding.expected)) return 'comparison'
  return 'notice'
}

function titleOf(code: string) {
  return titles.find(([pattern]) => pattern.test(code))?.[1] ?? code
}

function positions(lines: InvoiceLine[]) {
  const sorted = lines.map((line) => line.position).sort((left, right) => left - right)
  if (sorted.length === 1) return `linia ${sorted[0]}`
  const contiguous = sorted.every((value, index) => index === 0 || value === sorted[index - 1] + 1)
  return contiguous ? `liniile ${sorted[0]}–${sorted[sorted.length - 1]}` : `liniile ${sorted.join(', ')}`
}

function uniqueEvidence(findings: CommercialFinding[]) {
  const seen = new Set<string>()
  return findings.flatMap((finding) => finding.evidence ?? []).filter((item) => {
    const key = `${item.documentId}|${item.page ?? ''}|${item.snippet}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

function describe(finding: CommercialFinding, line: InvoiceLine | undefined): Pick<CommercialCheck, 'invoiceValue' | 'invoiceDetail' | 'contractValue' | 'contractDetail' | 'explanation'> {
  const code = finding.code
  const currency = line?.unitPrice.currency
  const override = finding.override ? `Excepție aprobată: ${finding.override.reason}` : undefined
  // The reason counts the clauses still open; older runs only state the gap.
  if (code === 'CONTRACT_COVERAGE_INCOMPLETE' && finding.evidence?.length) return { explanation: override ?? `${finding.reason} Cât timp rămân deschise, toate facturile acestui contract ajung la verificare; tarifele confirmate sunt totuși comparate.` }
  if (code.startsWith('VAT_RATE_')) {
    const legal = /legal/i.test(finding.calculation ?? '') || /legal/i.test(finding.reason)
    return { invoiceValue: formatRate(finding.actual), contractValue: formatRate(finding.expected), contractDetail: legal ? 'cota legală la data facturii' : 'cota din contract', explanation: override ?? (finding.outcome === 'CONFORM' ? undefined : finding.reason) }
  }
  if (/^(PRICE_|EXPECTED_PRICE_INVALID)/.test(code)) {
    const service = serviceNameFromCalculation(finding.calculation)
    return { invoiceValue: formatAmount(finding.actual, currency), invoiceDetail: 'preț unitar fără TVA', contractValue: formatAmount(finding.expected, currency), contractDetail: service ? `tariful „${service}”` : undefined, explanation: override ?? (finding.outcome === 'CONFORM' ? undefined : finding.reason) }
  }
  if (code.startsWith('PAYMENT_DUE_') && code !== 'PAYMENT_DUE_DATE_MISSING') {
    return { invoiceValue: finding.actual ? `${finding.actual} zile` : undefined, invoiceDetail: finding.actualSource || 'de la emitere la scadență', contractValue: finding.expected ? `${formatRate(finding.expected)?.replace('%', '')} zile` : undefined, explanation: override ?? (finding.outcome === 'CONFORM' ? undefined : finding.reason) }
  }
  if (code === 'RULE_INPUT_OUTSIDE_VALIDITY') {
    return { explanation: `${explanations[code]} Perioada salvată: ${finding.calculation ?? '—'}.` }
  }
  if (code === 'SERVICE_LINE_UNCOVERED') {
    const suggestion = finding.serviceCandidates?.find((candidate) => candidate.suggested)
    return { explanation: suggestion ? `Sugestie: ${suggestion.label}` : finding.serviceCandidates?.length ? 'Alege serviciul din contract pe baza descrierii.' : 'Contractul nu are încă tarife active pentru această linie.' }
  }
  return { invoiceValue: finding.actual, invoiceDetail: finding.actualSource, contractValue: finding.expected, explanation: override ?? explanations[code] ?? (finding.outcome === 'CONFORM' ? undefined : finding.reason) }
}

const outcomeOrder: Record<CommercialOutcome, number> = { NECONFORM: 0, NEVERIFICABIL: 1, CONFORM: 2 }

function effectiveOutcome(check: Pick<CommercialCheck, 'outcome' | 'findings'>) {
  return check.findings.every((finding) => finding.override) ? 'CONFORM' : check.outcome
}

function sortChecks(checks: CommercialCheck[]) {
  return [...checks].sort((left, right) => outcomeOrder[effectiveOutcome(left)] - outcomeOrder[effectiveOutcome(right)])
}

function check(finding: CommercialFinding, lines: InvoiceLine[], scope: string): CommercialCheck {
  return { id: finding.id, code: finding.code, kind: kindOf(finding.code, finding), outcome: finding.outcome, findings: [finding], lineIds: finding.lineId ? [finding.lineId] : [], title: titleOf(finding.code), scope, evidence: uniqueEvidence([finding]), ...describe(finding, lines[0]) }
}

export function buildCommercialView(run: CommercialValidationRun, invoice: Invoice): CommercialView {
  const lines = new Map(invoice.lines.map((line) => [line.id, line]))
  const findings = run.findings.map((finding) => reinterpretLegacyVAT(finding, lines))
  const invoiceChecks: CommercialCheck[] = []
  const byLine = new Map<string, CommercialCheck[]>()
  const addToLine = (lineId: string, item: CommercialCheck) => byLine.set(lineId, [...(byLine.get(lineId) ?? []), item])

  // A VAT rate that matches on every line it applies to is one check about
  // the invoice, not one identical card per line.
  const vatByRule = new Map<string, CommercialFinding[]>()
  for (const finding of findings) if (finding.code.startsWith('VAT_RATE_') && finding.lineId) vatByRule.set(finding.ruleId, [...(vatByRule.get(finding.ruleId) ?? []), finding])
  const merged = new Map<string, CommercialCheck>()
  for (const [ruleId, group] of vatByRule) {
    const conform = group.filter((finding) => finding.outcome === 'CONFORM')
    if (conform.length < 1 || conform.length !== group.length) continue
    const groupLines = conform.map((finding) => lines.get(finding.lineId!)).filter((line): line is InvoiceLine => Boolean(line))
    const sameRate = new Set(conform.map((finding) => decimal(finding.actual))).size === 1
    const first = conform[0]
    const vat: CommercialCheck = {
      id: `vat:${ruleId}`, code: 'VAT_RATE_MATCH', kind: 'comparison', outcome: 'CONFORM', findings: conform, lineIds: conform.map((finding) => finding.lineId!),
      title: 'Cota TVA', scope: positions(groupLines), evidence: uniqueEvidence(conform),
      ...describe(first, groupLines[0]),
      invoiceValue: sameRate ? formatRate(first.actual) : conform.map((finding) => formatRate(finding.actual)).join(', '),
      invoiceDetail: groupLines.length > 1 ? `pe ${positions(groupLines)}` : undefined,
    }
    conform.forEach((finding) => merged.set(finding.id, vat))
  }

  for (const finding of findings) {
    const vat = merged.get(finding.id)
    if (vat) {
      // The merged check takes the place of its first finding.
      if (!invoiceChecks.includes(vat)) invoiceChecks.push(vat)
      continue
    }
    const line = finding.lineId ? lines.get(finding.lineId) : undefined
    if (line) addToLine(line.id, check(finding, [line], `Linia ${line.position}`))
    else invoiceChecks.push(check(finding, [], 'Factură'))
  }

  const lineGroups = [...invoice.lines].sort((left, right) => left.position - right.position).filter((line) => byLine.has(line.id)).map((line) => {
    const checks = sortChecks(byLine.get(line.id)!)
    const price = checks.find((item) => /^(PRICE_|EXPECTED_PRICE_INVALID)/.test(item.code))
    return { line, checks, serviceLabel: serviceNameFromCalculation(price?.findings[0].calculation) }
  })
  const sortedInvoiceChecks = sortChecks(invoiceChecks)
  const all = [...sortedInvoiceChecks, ...lineGroups.flatMap((group) => group.checks)]
  return {
    invoiceChecks: sortedInvoiceChecks,
    lineGroups,
    verifiable: all.filter((item) => item.kind === 'comparison'),
    mappings: findings.filter((finding) => finding.code === 'SERVICE_LINE_UNCOVERED' && finding.lineId && finding.serviceCandidates?.length && lines.has(finding.lineId)).map((finding) => ({ line: lines.get(finding.lineId!)!, finding })).sort((left, right) => left.line.position - right.line.position),
    pendingCount: all.filter((item) => effectiveOutcome(item) !== 'CONFORM').length,
    checkCount: all.length,
  }
}

// Where the compared value sits in the e-Factura, for the side-by-side view.
export interface InvoiceFocus { lineIds: string[]; field?: 'unitPrice' | 'vat' | 'quantity' | 'description'; header?: 'dueDate' | 'reference'; text?: string; sourcePath?: string }

export function invoiceFocus(item: CommercialCheck, invoice: Invoice): InvoiceFocus {
  const lines = item.lineIds.map((id) => invoice.lines.find((line) => line.id === id)).filter((line): line is InvoiceLine => Boolean(line))
  const paths = (suffix: string) => lines.map((line) => `${linePath(line)}${suffix}`).join('\n')
  if (item.code.startsWith('VAT_RATE_')) return { lineIds: item.lineIds, field: 'vat', sourcePath: paths('/cac:Item/cac:ClassifiedTaxCategory/cbc:Percent') }
  if (/^(PRICE_|EXPECTED_PRICE_INVALID)/.test(item.code)) return { lineIds: item.lineIds, field: 'unitPrice', sourcePath: paths('/cac:Price/cbc:PriceAmount') }
  if (/^(CURRENCY_MISMATCH|LINE_PRICE_CURRENCY_MISSING)$/.test(item.code)) return { lineIds: item.lineIds, field: 'unitPrice', sourcePath: paths('/cac:Price/cbc:PriceAmount/@currencyID') }
  if (/^UNIT_QUANTITY_/.test(item.code)) return { lineIds: item.lineIds, field: 'quantity', sourcePath: paths('/cbc:InvoicedQuantity') }
  if (/^SERVICE_LINE_/.test(item.code)) return { lineIds: item.lineIds, field: 'description', sourcePath: paths('/cac:Item/cbc:Name') }
  if (item.code.startsWith('PAYMENT_DUE_')) return { lineIds: [], header: 'dueDate', sourcePath: '/Invoice/cbc:IssueDate → /Invoice/cbc:DueDate' }
  if (item.code.startsWith('CONTRACT_REFERENCE_')) {
    const finding = item.findings[0]
    const position = Number(finding.actualSource?.match(/liniei\s+(\d+)/i)?.[1])
    const line = invoice.lines.find((candidate) => candidate.position === position)
    if (line) return { lineIds: [line.id], field: 'description', text: finding.actual, sourcePath: `${linePath(line)}/cac:Item/cbc:Description` }
    return { lineIds: [], header: 'reference', text: finding.actual, sourcePath: '/Invoice/cac:ContractDocumentReference/cbc:ID' }
  }
  return { lineIds: item.lineIds }
}
