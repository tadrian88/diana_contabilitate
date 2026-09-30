import type { CommercialRule, ContractDocument, ContractProposal, DocumentCommercialState, ExtractedContractField, ProposedCommercialClause, ReviewedContract, ReviewedServiceTerm, SkippedServicePriceReason } from '../../repositories/invoiceRepository'
import { formatAmount } from '../invoices/commercial-view'
import { describeCommercialRule, ruleKindLabel } from './commercialRuleText'

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

export type ClauseStatus = 'ACTIVE' | 'PROPOSED' | 'CLOSED'

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
export function serviceEvidence(proposed: ContractProposal['serviceTerms'][number] | undefined, term: ReviewedServiceTerm) {
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
      label: term.serviceDescription || `Serviciul ${index + 1}`, value: serviceSummary(term), tone, status, ...serviceEvidence(proposal?.serviceTerms[index], term),
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
