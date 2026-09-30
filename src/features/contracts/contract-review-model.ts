import type { CommercialRule, ContractDocument, ContractProposal, DocumentCommercialState, ExtractedContractField, ProposedCommercialClause, ProposedServiceTerm, ReviewedContract, ReviewedServiceTerm, SkippedServicePriceReason } from '../../repositories/invoiceRepository'
import { formatAmount } from '../invoices/commercial-view'
import { describeCommercialRule, ruleKindLabel } from './commercialRuleText'
import { normalizeForSearch } from './pdfTextSearch'
import type { LocatedSnippets } from './useContractPdf'

// The contract review page as a list of items, each tied to the words of the
// PDF it was read from. The page shows one item at a time next to the PDF,
// highlighted at those words. Items are grouped like the contract itself.

export type ReviewGroup = 'parties' | 'contract' | 'payment' | 'services' | 'clauses'
export const reviewGroups: Array<{ id: ReviewGroup; label: string }> = [
  { id: 'parties', label: 'Părți' },
  { id: 'contract', label: 'Contract' },
  { id: 'payment', label: 'Plată' },
  { id: 'services', label: 'Servicii și tarife' },
  { id: 'clauses', label: 'Clauze comerciale' },
]

// ok: nothing to do · attention: the reviewer has to act · closed: settled
// without a rule · unknown: the state could not be read.
export type ReviewTone = 'ok' | 'attention' | 'closed' | 'unknown'

export type ReviewItemKind =
  | { type: 'field'; key: keyof ReviewedContract }
  | { type: 'service'; index: number; ruleId?: string; activatable: boolean }
  | { type: 'clause'; ruleId: string; kind: string; clause?: ProposedCommercialClause; rule?: CommercialRule; status: ClauseStatus }
  | { type: 'coverage' }
  | { type: 'draft-clause'; ruleId: string; kind: string; clause: ProposedCommercialClause; status: DraftClauseStatus }

export type ClauseStatus = 'ACTIVE' | 'PROPOSED' | 'CLOSED'
// Before confirmation: read from the clause text and confirmed with the
// contract (RECOGNIZED); an AI rule included in the confirmation (INCLUDED),
// still to decide (CANDIDATE) or left for after it (DEFERRED); a clause
// without a formula, settled on the confirmed page (AFTER_CONFIRM).
export type DraftClauseStatus = 'RECOGNIZED' | 'INCLUDED' | 'CANDIDATE' | 'DEFERRED' | 'AFTER_CONFIRM'

export interface ReviewSignal { label: string; tone: 'ok' | 'attention' | 'neutral' }

export interface ReviewItem {
  id: string
  // Other addresses of the same item, e.g. the rule id of a service tariff,
  // so a link from an invoice check lands on it.
  aliases: string[]
  group: ReviewGroup
  kind: ReviewItemKind
  label: string
  value: string
  detail?: string
  tone: ReviewTone
  status: string
  page?: number
  highlights: string[]
  // Before confirmation: why the item is ticked or left to check, what stops
  // the confirmation on it, and the key its reviewer decision is kept under.
  signals?: ReviewSignal[]
  blocker?: string
  decisionKey?: string
}

export const blankField: ExtractedContractField = { value: null, status: 'MISSING', confidence: 'UNKNOWN', evidence: { page: null }, alternatives: [] }
export const proposalField = (proposal: ContractProposal | undefined, key: string): ExtractedContractField => (proposal as unknown as Record<string, ExtractedContractField> | undefined)?.[key] ?? blankField

export const documentRoleLabels: Record<ReviewedContract['documentRole'], string> = { BASE_CONTRACT: 'Contract de bază', ANNEX: 'Anexă', AMENDMENT: 'Act adițional', SOW: 'SOW', ORDER: 'Comandă', PRICE_LIST: 'Listă de prețuri', OTHER: 'Alt document' }
export const pricingModelLabels: Record<string, string> = { FIXED_FEE: 'Tarif fix', UNIT_RATE: 'Tarif unitar', FIXED_TOTAL: 'Valoare totală' }
export const billingFrequencyLabels: Record<string, string> = { MONTHLY: 'lunar', QUARTERLY: 'trimestrial', ANNUAL: 'anual', PER_OCCURRENCE: 'per prestație', UNKNOWN: 'periodicitate nespecificată' }

export const skippedReasonLabel: Record<SkippedServicePriceReason, string> = {
  PRICE_MISSING: 'lipsește prețul sau descrierea serviciului',
  PRICING_MODEL_UNSUPPORTED: 'modelul de tarifare nu poate fi verificat automat',
  SOURCE_EVIDENCE_MISSING: 'extragerea nu a citat fragmentul cu prețul',
  PRICE_CHANGED_FROM_SOURCE: 'prețul confirmat diferă de cel citat din PDF; corectează-l sau reextrage',
  RULE_INVALID: 'regula rezultată nu este validă',
  PRICE_NOT_IN_SOURCE: 'prețul și moneda nu apar în fragmentul citat din PDF',
  PRICED_BY_CONTRACT_CLAUSES: 'prețurile se verifică deja după clauzele confirmate ale contractului',
}

export const clauseReasonLabel: Record<string, string> = {
  COVERED_BY_CONTRACT_REFERENCE: 'Acoperită de referința contractului',
  COVERED_BY_SUPPLIER_IDENTITY: 'Acoperită de CUI-ul furnizorului',
  COVERED_BY_PARTY_IDENTITY: 'Acoperită de CUI-urile părților',
  COVERED_BY_SERVICE_TARIFF: 'Acoperită de tariful din „Servicii și tarife”',
  NOT_INVOICE_VERIFIABLE: 'Închisă: nu influențează facturile',
}

// Clause kinds whose terms would otherwise be compared on every invoice; closing
// one of them means those terms are no longer checked.
export const invoiceCheckedKinds = new Set(['FIXED_PRICE', 'UNIT_RATE', 'TIERED_PRICE', 'DISCOUNT', 'MINIMUM', 'MAXIMUM', 'VAT', 'PAYMENT_DUE'])

export function formatDate(value: string | undefined | null) {
  const match = /^(\d{4})-(\d{2})-(\d{2})/.exec(value ?? '')
  return match ? `${match[3]}.${match[2]}.${match[1]}` : value ?? ''
}

const cuiToken = /(?<![\p{L}\p{N}])(?:RO\s*)?(\d{6,10})(?![\p{L}\p{N}])/giu
const digits = (value: string) => value.replace(/\D/g, '')

export type IdentityParties = 'SUPPLIER' | 'BUYER' | 'BOTH'

// Mirrors the server check: which contract parties an identity clause names,
// when it names at least one CUI and every CUI it names is the supplier's or
// the client's. Undefined when it names no CUI or anyone else's, which is
// left for the reviewer.
export function identityParties(snippet: string | undefined, supplierCui: string | undefined, buyerCui: string | undefined): IdentityParties | undefined {
  const supplier = digits(supplierCui ?? '')
  const buyer = digits(buyerCui ?? '')
  const found = [...(snippet ?? '').matchAll(cuiToken)].map((match) => match[1])
  if (!found.length) return undefined
  let namesSupplier = false
  let namesBuyer = false
  for (const value of found) {
    if (supplier && value === supplier) namesSupplier = true
    else if (buyer && value === buyer) namesBuyer = true
    else return undefined
  }
  return namesSupplier && namesBuyer ? 'BOTH' : namesBuyer ? 'BUYER' : 'SUPPLIER'
}

const identityLabels: Record<IdentityParties, string> = { SUPPLIER: 'Identitate furnizor', BUYER: 'Identitate client', BOTH: 'Identitate părți' }

interface FieldConfig { key: keyof ReviewedContract; label: string; group: ReviewGroup; format?: (value: string, contract: ReviewedContract) => string }
export const reviewFieldConfigs: FieldConfig[] = [
  { key: 'supplierName', label: 'Furnizor', group: 'parties' },
  { key: 'supplierCui', label: 'CUI furnizor', group: 'parties' },
  { key: 'buyerCui', label: 'CUI cumpărător', group: 'parties' },
  { key: 'buyerName', label: 'Cumpărător (locatar)', group: 'parties' },
  { key: 'documentRole', label: 'Rol document', group: 'contract', format: (value) => documentRoleLabels[value as ReviewedContract['documentRole']] ?? value },
  { key: 'relatedReference', label: 'Referință document părinte', group: 'contract' },
  { key: 'reference', label: 'Referință contract', group: 'contract' },
  { key: 'effectiveFrom', label: 'Data de început', group: 'contract', format: formatDate },
  { key: 'periodType', label: 'Durata', group: 'contract', format: (value, contract) => value === 'INDEFINITE_TERM' ? 'Nedeterminată' : contract.effectiveTo ? `Determinată, până la ${formatDate(contract.effectiveTo)}` : 'Determinată' },
  { key: 'currency', label: 'Monedă', group: 'contract' },
  { key: 'totalValue', label: 'Valoare contractuală totală', group: 'contract', format: (value, contract) => value ? formatAmount(value, contract.currency) ?? value : 'Nespecificată' },
  { key: 'unitType', label: 'Tip unitate / bază comercială', group: 'contract' },
  { key: 'paymentTerms', label: 'Termeni de plată', group: 'payment' },
]

// The PDF words a field was read from. The duration is read from the end
// date when the contract has one, otherwise from the clause stating it.
function fieldEvidence(proposal: ContractProposal | undefined, key: keyof ReviewedContract, contract: ReviewedContract) {
  const source = key === 'periodType' && contract.periodType === 'FIXED_TERM' && proposalField(proposal, 'effectiveTo').evidence.snippet ? proposalField(proposal, 'effectiveTo') : proposalField(proposal, String(key))
  return { page: source.evidence.page ?? undefined, highlights: source.evidence.snippet ? [source.evidence.snippet] : [] }
}

export function serviceSummary(term: ReviewedServiceTerm) {
  const price = term.unitPrice ? formatAmount(term.unitPrice, term.currency) ?? term.unitPrice : 'preț lipsă'
  const unit = term.unit ? ` / ${term.unit}` : ''
  const frequency = billingFrequencyLabels[term.billingFrequency] ?? ''
  return [`${price}${unit}`, pricingModelLabels[term.pricingModel] ?? '', frequency].filter(Boolean).join(' · ')
}

// A tariff row is highlighted by its description and the price on the same
// row; the price page is the one to open.
export function serviceEvidence(proposed: ProposedServiceTerm | undefined, term: ReviewedServiceTerm) {
  const description = proposed?.serviceDescription.evidence ?? term.evidence
  const price = proposed?.unitPrice.evidence
  const highlights = [description?.snippet, price?.snippet].filter((snippet): snippet is string => Boolean(snippet?.trim()))
  return { page: price?.page ?? description?.page ?? undefined, highlights }
}

const clauseEvidence = (clause: ProposedCommercialClause | undefined, rule: CommercialRule | undefined) => {
  if (clause?.evidence.snippet) return { page: clause.evidence.page ?? undefined, highlights: [clause.evidence.snippet] }
  const cited = rule?.evidence?.find((item) => item.snippet)
  return { page: cited?.page ?? undefined, highlights: cited ? [cited.snippet] : [] }
}

// Items of a confirmed document, with what each one does for invoice checks
// now. Without a commercial state (it could not be read) tariffs and clauses
// say so instead of guessing.
export function buildConfirmedItems(document: ContractDocument): ReviewItem[] {
  const confirmed = document.confirmedValues
  if (!confirmed) return []
  const proposal = document.extraction?.proposal
  const state = document.commercialState
  const items: ReviewItem[] = []
  for (const config of reviewFieldConfigs) {
    if (config.key === 'relatedReference' && confirmed.documentRole === 'BASE_CONTRACT') continue
    if (config.key === 'buyerName' && !confirmed.buyerName) continue
    const raw = String(confirmed[config.key] ?? '')
    const extracted = proposalField(proposal, String(config.key)).value ?? ''
    const corrected = config.key !== 'periodType' && raw !== extracted
    items.push({
      id: `field:${config.key}`, aliases: [], group: config.group, kind: { type: 'field', key: config.key }, label: config.label,
      value: (config.format ? config.format(raw, confirmed) : raw) || 'Nespecificat',
      detail: corrected ? `Corectat de utilizator · AI: ${(extracted && config.format ? config.format(extracted, confirmed) : extracted) || 'lipsă'}` : undefined,
      tone: 'ok', status: corrected ? 'Corectat la confirmare' : 'Confirmat', ...fieldEvidence(proposal, config.key, confirmed),
    })
  }
  const services = state?.services ?? []
  ;(confirmed.serviceTerms ?? []).forEach((term, index) => {
    const service = services.find((item) => item.position === index + 1)
    const tone: ReviewTone = !state ? 'unknown' : service?.active ? 'ok' : 'attention'
    const status = !state ? 'Stare necunoscută' : service?.active ? 'Folosit la verificarea facturilor' : service?.skipReason ? `Nefolosit: ${skippedReasonLabel[service.skipReason] ?? service.skipReason}` : 'Nefolosit încă: poate fi activat'
    items.push({
      id: `service:${index}`, aliases: service?.ruleId ? [`rule:${service.ruleId}`] : [], group: 'services',
      kind: { type: 'service', index, ruleId: service?.ruleId, activatable: Boolean(state && service && !service.active && !service.skipReason) },
      label: term.serviceDescription || `Serviciul ${index + 1}`, value: serviceSummary(term), tone, status, ...serviceEvidence(serviceSource(proposal, term, index), term),
    })
  })
  const rules = (confirmed.commercialRules ?? []).filter((rule) => !rule.id.startsWith('service-'))
  const clauses = proposal?.commercialClauses ?? []
  const ruleIds = [...new Set([...rules.map((rule) => rule.id), ...clauses.map((clause) => clause.rule?.id).filter((id): id is string => Boolean(id))])]
  for (const ruleId of ruleIds) {
    const clause = clauses.find((item) => item.rule?.id === ruleId)
    const rule = rules.find((item) => item.id === ruleId)
    const settled = state?.clauses.find((item) => item.ruleId === ruleId)
    const kind = rule?.kind ?? clause?.rule?.kind ?? clause?.kind.value ?? ''
    const status: ClauseStatus = settled?.status === 'REJECTED' ? 'CLOSED' : rule || settled?.status === 'CONFIRMED' ? 'ACTIVE' : 'PROPOSED'
    const enforced = !state || state.activeRuleIds.includes(ruleId)
    items.push({
      id: `rule:${ruleId}`, aliases: [], group: 'clauses', kind: { type: 'clause', ruleId, kind, clause, rule, status },
      label: kind === 'CONTRACT_REFERENCE' && !clause ? 'Referința contractului' : kind === 'IDENTITY' && !rule ? clauseIdentityLabel(clause, confirmed) : ruleKindLabel(kind),
      value: rule ? describeCommercialRule(rule) : clause?.narrative.value ?? 'Clauză fără text',
      detail: status === 'CLOSED' ? settled?.reason : undefined,
      tone: status === 'PROPOSED' ? 'attention' : status === 'CLOSED' ? 'closed' : enforced ? 'ok' : 'unknown',
      status: status === 'PROPOSED' ? 'De rezolvat' : status === 'CLOSED' ? clauseReasonLabel[settled?.reasonCode ?? ''] ?? 'Închisă' : enforced ? 'Se verifică pe facturi' : 'Confirmată, în afara versiunii active',
      ...clauseEvidence(clause, rule),
    })
  }
  return items
}

// An identity clause is labelled by the party it names: the extraction reads
// one per party, and the client's is not the supplier's.
function clauseIdentityLabel(clause: ProposedCommercialClause | undefined, confirmed: ReviewedContract) {
  const parties = identityParties(clause?.evidence.snippet, confirmed.supplierCui, confirmed.buyerCui)
  return parties ? identityLabels[parties] : 'Identitate parte'
}

export function coverageLabel(state: DocumentCommercialState | undefined) {
  if (!state) return { text: 'Acoperire necunoscută', tone: 'neutral' as const }
  if (state.coverage === 'COMPLETE') return { text: 'Acoperire completă', tone: 'success' as const }
  if (state.coverage === 'CONFLICTED') return { text: 'Acoperire cu conflicte', tone: 'danger' as const }
  return { text: 'Acoperire parțială', tone: 'warning' as const }
}

// Resolves a deep link (`?element=`) to an item: its id, one of its aliases,
// or a group (`group:services`), which lands on the group's first item that
// needs attention, else its first item.
export function findItem(items: ReviewItem[], address: string | null) {
  if (!address) return undefined
  const direct = items.find((item) => item.id === address || item.aliases.includes(address))
  if (direct) return direct
  const group = address.startsWith('group:') ? address.slice(6) : undefined
  const members = items.filter((item) => item.group === group)
  return members.find((item) => item.tone === 'attention') ?? members[0]
}

// The proposal row a reviewed service was read from: its sourceIndex, or its
// position for a contract confirmed before services carried one.
export function serviceSource(proposal: ContractProposal | undefined, term: ReviewedServiceTerm, index: number) {
  const source = term.sourceIndex ?? index
  return source >= 0 ? proposal?.serviceTerms[source] : undefined
}

// ---------------------------------------------------------------------------
// Review before confirmation. An item comes pre-ticked only when the AI read
// it with high confidence, its snippet is found exactly in the PDF and the
// snippet states the value; every other item waits for the reviewer.

export const expressionRequiredKinds = new Set(['IDENTITY', 'CONTRACT_REFERENCE', 'FIXED_PRICE', 'UNIT_RATE', 'TIERED_PRICE', 'DISCOUNT', 'TRANCHE', 'PRORATA', 'MINIMUM', 'MAXIMUM', 'COST_PLUS', 'FX', 'VAT', 'PAYMENT_DUE'])
export const clauseNeedsRuleReview = (clause: ProposedCommercialClause) => expressionRequiredKinds.has(clause.rule?.kind ?? '') && !clause.rule?.expression

export function proposedTerm(term: ProposedServiceTerm, sourceIndex: number): ReviewedServiceTerm {
  return { serviceDescription: term.serviceDescription.value ?? '', pricingModel: (term.pricingModel.value ?? '') as ReviewedServiceTerm['pricingModel'], unitPrice: term.unitPrice.value ?? '', currency: term.currency.value ?? '', unit: term.unit.value ?? '', quantitySource: (term.quantitySource.value ?? 'UNKNOWN') as ReviewedServiceTerm['quantitySource'], quantityValue: term.quantityValue.value ?? '', quantityDriver: term.quantityDriver.value ?? '', billingFrequency: (term.billingFrequency.value ?? 'UNKNOWN') as ReviewedServiceTerm['billingFrequency'], evidence: term.serviceDescription.evidence, sourceIndex }
}

export function initialReviewValues(proposal: ContractProposal | undefined): ReviewedContract {
  const value = (key: string) => proposalField(proposal, key).value ?? ''
  return { supplierName: value('supplierName'), supplierCui: value('supplierCui'), buyerCui: value('buyerCui'), buyerName: value('buyerName'), reference: value('reference'), effectiveFrom: value('effectiveFrom'), effectiveTo: value('effectiveTo'), totalValue: value('totalValue'), currency: value('currency'), unitType: value('unitType'), paymentTerms: value('paymentTerms'), periodType: (value('periodType') || 'FIXED_TERM') as ReviewedContract['periodType'], documentRole: (value('documentRole') || 'BASE_CONTRACT') as ReviewedContract['documentRole'], relatedReference: value('relatedReference'), coverage: 'PARTIAL', serviceTerms: (proposal?.serviceTerms ?? []).map(proposedTerm), commercialRules: [] }
}

export const manualServiceTerm = (currency: string): ReviewedServiceTerm => ({ serviceDescription: '', pricingModel: '', unitPrice: '', currency, unit: '', quantitySource: 'UNKNOWN', quantityValue: '', quantityDriver: '', billingFrequency: 'UNKNOWN', evidence: { page: null }, sourceIndex: -1 })

const monthNames = ['ianuarie', 'februarie', 'martie', 'aprilie', 'mai', 'iunie', 'iulie', 'august', 'septembrie', 'octombrie', 'noiembrie', 'decembrie']
const escapeRegExp = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
// Lower case, Romanian diacritics folded, whitespace kept as single spaces.
const foldWords = (value: string) => value.split(/\s+/).filter(Boolean).map(normalizeForSearch).join(' ')
const standsAlone = (text: string, token: string) => new RegExp(`(?<!\\d)${escapeRegExp(token)}(?!\\d)`).test(text)

function canonicalDecimal(value: string) {
  const match = /^(\d+)(?:\.(\d+))?$/.exec(value)
  if (!match) return undefined
  const whole = match[1].replace(/^0+(?=\d)/, '')
  const fraction = (match[2] ?? '').replace(/0+$/, '')
  return fraction ? `${whole}.${fraction}` : whole
}

// Mirrors the server (sourceStatesValue): 1.800,00 · 1 800,00 · 1,800.00 ·
// 125000.00. A lone separator followed by three digits reads both ways; the
// last of two different separators is the decimal one.
const amountPattern = /(?<![\d.,])(?:\d{1,3}(?:[.,  ]\d{3})+(?:[.,]\d+)?|\d+(?:[.,]\d+)?)(?!\d)/g
function amountStated(snippet: string, value: string) {
  const wanted = canonicalDecimal(value.trim())
  if (!wanted) return false
  return [...snippet.matchAll(amountPattern)].some(([token]) => {
    const compact = token.replace(/[  ]/g, '')
    const dots = compact.split('.').length - 1
    const commas = compact.split(',').length - 1
    const readings: string[] = []
    if (dots && commas) readings.push(compact.lastIndexOf(',') > compact.lastIndexOf('.') ? compact.replace(/\./g, '').replace(',', '.') : compact.replace(/,/g, ''))
    else if (dots > 1) readings.push(compact.replace(/\./g, ''))
    else if (commas > 1) readings.push(compact.replace(/,/g, ''))
    else if (dots || commas) {
      const separator = dots ? '.' : ','
      readings.push(compact.replace(separator, '.'))
      if (compact.length - compact.indexOf(separator) - 1 === 3) readings.push(compact.replace(separator, ''))
    } else readings.push(compact)
    return readings.some((reading) => canonicalDecimal(reading) === wanted)
  })
}

function dateStated(snippet: string, value: string) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value.trim())
  if (!match) return false
  const [, year, month, day] = match
  const text = foldWords(snippet).replace(/\s*([./-])\s*/g, '$1')
  const d = String(Number(day))
  const m = String(Number(month))
  const numeric = [`${year}-${month}-${day}`, ...['.', '/', '-'].flatMap((separator) => [`${day}${separator}${month}${separator}${year}`, `${d}${separator}${m}${separator}${year}`])]
  const named = [day, d].map((dayText) => `${dayText} ${monthNames[Number(month) - 1]} ${year}`)
  return [...numeric, ...named].some((candidate) => standsAlone(text, candidate))
}

const currencyWords: Record<string, RegExp> = {
  RON: /(?<![a-z])(ron|lei|leu)(?![a-z])/,
  EUR: /(?<![a-z])(eur|euro)(?![a-z])|€/,
  USD: /(?<![a-z])(usd|dolari)(?![a-z])|\$/,
}

// Whether a snippet states the value the reviewer is asked to confirm. Null
// when the check does not apply (the document role is read from the whole
// document, not from a value written in it).
export function valueStatedInSnippet(key: string, value: string, snippet: string | undefined): boolean | null {
  if (key === 'documentRole') return null
  const text = snippet ?? ''
  const trimmed = value.trim()
  if (!trimmed || !text.trim()) return false
  switch (key) {
    case 'supplierCui':
    case 'buyerCui': {
      const digits = trimmed.replace(/\D/g, '')
      return Boolean(digits) && [...text.matchAll(/(?<![\p{L}\p{N}])(?:RO\s*)?(\d{2,10})(?![\p{L}\p{N}])/giu)].some((match) => match[1] === digits)
    }
    case 'effectiveFrom':
    case 'effectiveTo':
      return dateStated(text, trimmed)
    case 'currency': {
      const folded = foldWords(text)
      const code = trimmed.toUpperCase()
      return (currencyWords[code] ?? new RegExp(`(?<![a-z])${escapeRegExp(code.toLowerCase())}(?![a-z])`)).test(folded)
    }
    case 'totalValue':
    case 'amount':
      return amountStated(text, trimmed)
    case 'periodType': {
      const folded = foldWords(text)
      return trimmed === 'INDEFINITE_TERM' ? /nedeterminat/.test(folded) : /(?<!ne)determinat/.test(folded)
    }
    default:
      return normalizeForSearch(text).includes(normalizeForSearch(trimmed))
  }
}

export interface ReviewBlocker { itemId: string; message: string }

const normalizeCui = (raw: string) => raw.toUpperCase().replace(/\s/g, '').replace(/^RO/, '')

// D-126: the client may be the supplier of the contract (e.g. the Locator of a
// lease); the buyer is then the tenant or customer, named and identified by a
// CUI or, for a natural person, a CNP.
export const saleContract = (values: ReviewedContract, clientCui?: string) => Boolean(clientCui) && normalizeCui(values.supplierCui) === normalizeCui(clientCui ?? '') && normalizeCui(values.buyerCui) !== normalizeCui(clientCui ?? '')
const validBuyerId = (raw: string) => /^(RO\s*)?[1-9][0-9\s]{1,12}$/i.test(raw.trim()) || /^[0-9]{13}$/.test(raw.replace(/\s/g, ''))

// What stops the confirmation, each tied to the item the reviewer fixes it
// on. The messages are the server's (ConfirmationReadinessFor).
export function confirmationBlockers(values: ReviewedContract, document: ContractDocument, unconfirmedClauses: number): ReviewBlocker[] {
  const result: ReviewBlocker[] = []
  const add = (itemId: string, message: string) => result.push({ itemId, message })
  const base = values.documentRole === 'BASE_CONTRACT'
  if (base && !values.supplierName.trim()) add('field:supplierName', 'Denumirea furnizorului este obligatorie.')
  if (base && !/^(RO\s*)?[1-9][0-9\s]{1,12}$/i.test(values.supplierCui)) add('field:supplierCui', 'CUI-ul furnizorului nu este valid.')
  if (base && !values.reference.trim()) add('field:reference', 'Referința contractului este obligatorie.')
  if (base && !values.effectiveFrom) add('field:effectiveFrom', 'Data de început este obligatorie.')
  if (base && values.periodType === 'FIXED_TERM' && !values.effectiveTo) add('field:periodType', 'Data de sfârșit este obligatorie pentru durata determinată.')
  if (base && values.periodType === 'FIXED_TERM' && values.effectiveFrom && values.effectiveTo && values.effectiveTo < values.effectiveFrom) add('field:periodType', 'Data de sfârșit trebuie să fie după data de început.')
  if (base && values.periodType === 'INDEFINITE_TERM' && values.effectiveTo) add('field:periodType', 'Durata nedeterminată nu poate avea dată de sfârșit.')
  if (base && !/^[A-Z]{3}$/.test(values.currency)) add('field:currency', 'Moneda ISO este obligatorie.')
  if (base && values.totalValue && !/^\d{1,16}(\.\d{1,4})?$/.test(values.totalValue)) add('field:totalValue', 'Valoarea contractuală totală nu este validă.')
  if (base && !values.unitType.trim()) add('field:unitType', 'Tipul unității / baza comercială este obligatoriu.')
  if (base && !values.paymentTerms.trim()) add('field:paymentTerms', 'Termenii de plată sunt obligatorii.')
  if (!base && !values.relatedReference.trim()) add('field:relatedReference', 'Referința contractului părinte este obligatorie.')
  if (!base && values.commercialRules.length === 0 && values.serviceTerms.length === 0 && unconfirmedClauses === 0) add('field:documentRole', 'Documentul suplimentar trebuie să conțină cel puțin o clauză comercială.')
  const sale = saleContract(values, document.clientCui)
  if (sale) {
    if (!values.buyerName?.trim()) add('field:buyerName', 'Denumirea cumpărătorului (locatarului) este obligatorie când clientul este furnizor.')
    if (!validBuyerId(values.buyerCui)) add('field:buyerCui', 'CUI-ul sau CNP-ul cumpărătorului (locatarului) nu este valid.')
  } else if (document.clientCui && normalizeCui(values.buyerCui) !== normalizeCui(document.clientCui)) add('field:buyerCui', 'CUI-ul cumpărătorului nu corespunde clientului selectat.')
  values.serviceTerms.forEach((term, index) => {
    const id = `service:${index}`
    if (!term.serviceDescription.trim()) add(id, `Serviciul ${index + 1}: descrierea este obligatorie.`)
    if (!term.pricingModel) add(id, `Serviciul ${index + 1}: modelul de tarifare este obligatoriu.`)
    if (!/^\d{1,16}(\.\d{1,4})?$/.test(term.unitPrice)) add(id, `Serviciul ${index + 1}: prețul nu este valid.`)
    if (!/^[A-Z]{3}$/.test(term.currency)) add(id, `Serviciul ${index + 1}: moneda nu este validă.`)
    if (term.pricingModel === 'UNIT_RATE' && !term.unit.trim()) add(id, `Serviciul ${index + 1}: unitatea este obligatorie pentru tariful unitar.`)
  })
  values.commercialRules.forEach((rule, index) => {
    if (!sale && rule.kind === 'IDENTITY' && (base || values.supplierCui.trim()) && normalizeCui(rule.expression?.value ?? '') !== normalizeCui(values.supplierCui)) add(`rule:${rule.id}`, `Regula comercială ${index + 1} verifică alt CUI decât al furnizorului; pe factură se compară doar furnizorul.`)
  })
  if (!base && values.commercialRules.length === 0 && unconfirmedClauses === 0) add('field:documentRole', 'Documentul suplimentar necesită o regulă comercială identificată explicit.')
  return result
}

export type ReviewDecision = 'CHECKED' | 'DEFERRED'

export interface PdfTextIndex {
  // The text of every page has been read (or the PDF could not be).
  ready: boolean
  scanned: boolean
  locate: (snippets: string[], page?: number | null) => LocatedSnippets
}

const confidenceSignal = (field: ExtractedContractField): ReviewSignal =>
  field.status === 'MISSING' ? { label: 'Nespecificat în document', tone: 'attention' }
    : field.status === 'AMBIGUOUS' ? { label: `Valoare ambiguă: ${field.alternatives.join(' / ') || 'mai multe variante'}`, tone: 'attention' }
      : field.confidence === 'HIGH' ? { label: 'Încredere mare', tone: 'ok' }
        : field.confidence === 'MEDIUM' ? { label: 'Încredere medie', tone: 'attention' }
          : field.confidence === 'LOW' ? { label: 'Încredere mică', tone: 'attention' }
            : { label: 'Încredere necunoscută', tone: 'attention' }

function locationSignal(pdf: PdfTextIndex, highlights: string[], page: number | undefined): { signal?: ReviewSignal; exact: boolean } {
  if (!highlights.length) return { signal: { label: 'Fără fragment citat', tone: 'attention' }, exact: false }
  if (!pdf.ready) return { signal: { label: 'Se caută în PDF…', tone: 'neutral' }, exact: false }
  // A scan has one banner for the whole page, not one per item.
  if (pdf.scanned) return { exact: false }
  const located = pdf.locate(highlights, page)
  if (located.location === 'EXACT') return { signal: { label: 'Găsit exact în PDF', tone: 'ok' }, exact: true }
  if (located.location === 'PARTIAL') return { signal: { label: 'Găsit doar parțial în PDF', tone: 'attention' }, exact: false }
  return { signal: { label: 'Negăsit în textul PDF-ului', tone: 'attention' }, exact: false }
}

const statedSignal = (stated: boolean | null): ReviewSignal | undefined =>
  stated === null ? undefined : stated ? { label: 'Valoarea apare în fragment', tone: 'ok' } : { label: 'Valoarea nu apare în fragment', tone: 'attention' }

interface Verdict { tone: ReviewTone; status: string }

// The row's state: a blocker first, then the reviewer's decision, then the
// automatic check. Nothing turns while the PDF text is still being read.
function verdict({ blocker, checked, corrected, auto, ready, signals }: { blocker?: string; checked: boolean; corrected: boolean; auto: boolean; ready: boolean; signals: ReviewSignal[] }): Verdict {
  if (blocker) return { tone: 'attention', status: blocker }
  if (checked) return { tone: 'ok', status: corrected ? 'Corectat de tine' : 'Verificat de tine' }
  if (!ready) return { tone: 'unknown', status: 'Se verifică în PDF…' }
  if (auto) return { tone: 'ok', status: 'Verificat automat: găsit exact în PDF' }
  const reason = signals.find((signal) => signal.tone === 'attention')
  return { tone: 'attention', status: reason ? `De verificat: ${reason.label.charAt(0).toLowerCase()}${reason.label.slice(1)}` : 'De verificat' }
}

// The editable label of each field, as the form has always named it.
export const reviewFieldInputs: Partial<Record<keyof ReviewedContract, { label: string; type?: string }>> = {
  supplierName: { label: 'Denumire furnizor' }, supplierCui: { label: 'CUI furnizor' }, buyerCui: { label: 'CUI cumpărător' }, buyerName: { label: 'Denumire cumpărător (locatar)' }, reference: { label: 'Referință contract' },
  effectiveFrom: { label: 'Data de început', type: 'date' }, totalValue: { label: 'Valoare contractuală totală (dacă există)' }, currency: { label: 'Monedă ISO' },
  unitType: { label: 'Tip unitate / bază comercială' }, paymentTerms: { label: 'Termeni de plată' }, relatedReference: { label: 'Referință document părinte' },
}

function reviewFieldItem(config: FieldConfig, proposal: ContractProposal | undefined, values: ReviewedContract, decisions: Record<string, ReviewDecision>, pdf: PdfTextIndex, blocker: string | undefined): ReviewItem {
  const id = `field:${config.key}`
  // The duration is read from the end date when the contract has one.
  const fromEnd = config.key === 'periodType' && values.periodType === 'FIXED_TERM' && Boolean(proposalField(proposal, 'effectiveTo').evidence.snippet)
  const sourceKey = fromEnd ? 'effectiveTo' : String(config.key)
  const source = proposalField(proposal, sourceKey)
  const raw = String(values[config.key] ?? '')
  const extracted = proposalField(proposal, String(config.key)).value ?? ''
  const corrected = config.key === 'periodType'
    ? raw !== (extracted || 'FIXED_TERM') || values.effectiveTo !== (proposalField(proposal, 'effectiveTo').value ?? '')
    : raw !== extracted
  const { page, highlights } = fieldEvidence(proposal, config.key, values)
  const location = locationSignal(pdf, highlights, page)
  const stated = source.status === 'PRESENT' ? valueStatedInSnippet(sourceKey, fromEnd ? values.effectiveTo : raw, source.evidence.snippet) : false
  const signals = [confidenceSignal(source), location.signal, source.status === 'PRESENT' && location.exact ? statedSignal(stated) : undefined].filter((signal): signal is ReviewSignal => Boolean(signal))
  const auto = !corrected && source.status === 'PRESENT' && source.confidence === 'HIGH' && location.exact && stated !== false
  const format = (value: string, contract: ReviewedContract) => (config.format ? config.format(value, contract) : value)
  // What the AI read, shown next to a corrected value (the duration with the
  // AI's end date, not the corrected one).
  const aiValue = extracted ? format(extracted, { ...values, effectiveTo: proposalField(proposal, 'effectiveTo').value ?? '' }) : ''
  return {
    id, aliases: [], group: config.group, kind: { type: 'field', key: config.key }, label: config.label,
    value: format(raw, values) || 'Nespecificat', detail: corrected ? `AI: ${aiValue || 'lipsă'}` : undefined,
    page, highlights, signals, blocker, decisionKey: id,
    ...verdict({ blocker, checked: decisions[id] === 'CHECKED', corrected, auto, ready: pdf.ready, signals }),
  }
}

function reviewServiceItem(term: ReviewedServiceTerm, index: number, proposal: ContractProposal | undefined, decisions: Record<string, ReviewDecision>, pdf: PdfTextIndex, blocker: string | undefined): ReviewItem {
  const id = `service:${index}`
  const proposed = serviceSource(proposal, term, index)
  const { page, highlights } = serviceEvidence(proposed, term)
  const base = { id, aliases: [], group: 'services' as const, kind: { type: 'service' as const, index, activatable: false }, label: term.serviceDescription || `Serviciul ${index + 1}`, value: serviceSummary(term), page, highlights }
  if (!proposed) {
    // Added by the reviewer: nothing in the PDF to compare with.
    return { ...base, signals: [{ label: 'Adăugat de tine · fără fragment în PDF', tone: 'neutral' }], blocker, tone: blocker ? 'attention' : 'ok', status: blocker ?? 'Adăugat de tine; nu va fi comparat cu PDF-ul' }
  }
  const decisionKey = `service-source:${term.sourceIndex ?? index}`
  const original = proposedTerm(proposed, term.sourceIndex ?? index)
  const corrected = (['serviceDescription', 'pricingModel', 'unitPrice', 'currency', 'unit', 'billingFrequency', 'quantitySource', 'quantityValue', 'quantityDriver'] as const).some((key) => term[key] !== original[key])
  const fields = [proposed.serviceDescription, proposed.unitPrice, proposed.currency, proposed.pricingModel]
  const weakest = fields.find((field) => field.status !== 'PRESENT' || field.confidence !== 'HIGH')
  const location = locationSignal(pdf, highlights, page)
  const priceSnippet = proposed.unitPrice.evidence.snippet
  const stated = proposed.unitPrice.status === 'PRESENT'
    ? valueStatedInSnippet('amount', term.unitPrice, priceSnippet) === true && valueStatedInSnippet('currency', term.currency, [priceSnippet, proposed.serviceDescription.evidence.snippet].filter(Boolean).join(' ')) === true
    : false
  const signals = [weakest ? confidenceSignal(weakest) : { label: 'Încredere mare', tone: 'ok' as const }, location.signal, location.exact ? (stated ? { label: 'Prețul și moneda apar în fragment', tone: 'ok' as const } : { label: 'Prețul sau moneda nu apar în fragment', tone: 'attention' as const }) : undefined].filter((signal): signal is ReviewSignal => Boolean(signal))
  const auto = !corrected && !weakest && location.exact && stated
  return {
    ...base, signals, blocker, decisionKey,
    detail: corrected ? `AI: ${serviceSummary(original)}` : undefined,
    ...verdict({ blocker, checked: decisions[decisionKey] === 'CHECKED', corrected, auto, ready: pdf.ready, signals }),
  }
}

function reviewClauseItem(clause: ProposedCommercialClause, values: ReviewedContract, decisions: Record<string, ReviewDecision>, blocker: string | undefined): ReviewItem {
  const ruleId = clause.rule.id
  const id = `rule:${ruleId}`
  const kind = clause.rule.kind || clause.kind.value || ''
  const needsReview = clauseNeedsRuleReview(clause)
  const included = values.commercialRules.some((rule) => rule.id === ruleId)
  const status: DraftClauseStatus = needsReview ? (clause.recognizedRule ? 'RECOGNIZED' : 'AFTER_CONFIRM') : included ? 'INCLUDED' : decisions[id] === 'DEFERRED' ? 'DEFERRED' : 'CANDIDATE'
  const parties = kind === 'IDENTITY' ? identityParties(clause.evidence.snippet, values.supplierCui, values.buyerCui) : undefined
  const texts: Record<DraftClauseStatus, { tone: ReviewTone; status: string }> = {
    RECOGNIZED: { tone: 'ok', status: 'Recunoscută automat din text · se confirmă odată cu contractul' },
    INCLUDED: { tone: 'ok', status: 'Inclusă în confirmare: se va verifica pe facturi' },
    CANDIDATE: { tone: 'attention', status: 'De verificat: regulă propusă de AI' },
    DEFERRED: { tone: 'closed', status: 'Lăsată pentru după confirmare' },
    AFTER_CONFIRM: { tone: 'closed', status: parties ? 'Se închide automat după confirmare: acoperită de CUI-ul părții' : kind === 'CONTRACT_REFERENCE' ? 'Se închide automat după confirmare: acoperită de referința contractului' : 'Se rezolvă după confirmare' },
  }
  const text = texts[status]
  return {
    id, aliases: [], group: 'clauses', kind: { type: 'draft-clause', ruleId, kind, clause, status },
    label: kind === 'IDENTITY' ? (parties ? identityLabels[parties] : 'Identitate parte') : ruleKindLabel(kind),
    // A rule to decide on is explained in the open row; the list names it.
    value: status === 'RECOGNIZED' ? describeCommercialRule(clause.recognizedRule!) : status === 'AFTER_CONFIRM' ? clause.narrative.value ?? 'Clauză fără text' : clause.rule.narrative || clause.narrative.value || 'Clauză fără text',
    page: clause.evidence.page ?? undefined, highlights: clause.evidence.snippet ? [clause.evidence.snippet] : [], blocker, decisionKey: id,
    tone: blocker ? 'attention' : text.tone, status: blocker ?? text.status,
  }
}

// Every item of a document still in review, pre-ticked or left to check.
export function buildReviewItems(document: ContractDocument, values: ReviewedContract, decisions: Record<string, ReviewDecision>, pdf: PdfTextIndex): ReviewItem[] {
  const proposal = document.extraction?.proposal
  const clauses = proposal?.commercialClauses ?? []
  const unconfirmed = clauses.filter((clause) => !values.commercialRules.some((rule) => rule.id === clause.rule?.id))
  const blockers = confirmationBlockers(values, document, unconfirmed.length)
  const blockerOf = (id: string) => blockers.find((blocker) => blocker.itemId === id)?.message
  const items: ReviewItem[] = []
  for (const config of reviewFieldConfigs) {
    if (config.key === 'relatedReference' && values.documentRole === 'BASE_CONTRACT') continue
    // The buyer's name matters only when the client is the supplier (D-126).
    if (config.key === 'buyerName' && !saleContract(values, document.clientCui)) continue
    items.push(reviewFieldItem(config, proposal, values, decisions, pdf, blockerOf(`field:${config.key}`)))
    if (config.key === 'unitType') {
      const coverage = { COMPLETE: 'Completă', PARTIAL: 'Parțială', CONFLICTED: 'Cu conflicte' }[values.coverage]
      items.push({ id: 'field:coverage', aliases: [], group: 'contract', kind: { type: 'coverage' }, label: 'Acoperire comercială', value: coverage, highlights: [], tone: 'ok', status: unconfirmed.length ? 'Parțială cât timp rămân clauze de rezolvat' : 'Alegerea ta la confirmare' })
    }
  }
  values.serviceTerms.forEach((term, index) => items.push(reviewServiceItem(term, index, proposal, decisions, pdf, blockerOf(`service:${index}`))))
  for (const clause of clauses) if (clause.rule?.id) items.push(reviewClauseItem(clause, values, decisions, blockerOf(`rule:${clause.rule.id}`)))
  return items
}

export function reviewProgress(items: ReviewItem[]) {
  // The coverage is a choice, not something read from the PDF.
  const counted = items.filter((item) => item.kind.type !== 'coverage' && (item.tone === 'ok' || item.tone === 'attention'))
  const remaining = items.filter((item) => item.tone === 'attention')
  return { verified: counted.length - remaining.length, total: counted.length, remaining }
}
