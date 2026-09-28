export type ClientScope = 'all' | string

export type PipelineStatus =
  | 'DOWNLOADED'
  | 'ARCHIVED'
  | 'MATCHING'
  | 'AWAITING_CONTRACT'
  | 'AWAITING_MATCH_CONFIRM'
  | 'DEDUPE_CHECKED'
  | 'HEADER_READ'
  | 'LINES_READ'
	| 'COMMERCIAL_VALIDATING'
	| 'AWAITING_COMMERCIAL_REVIEW'
	| 'COMMERCIALLY_VALIDATED'
  | 'CLASSIFIED'
  | 'AWAITING_REVIEW'
  | 'READY_FOR_SAGA'
  | 'EXPORTING'
  | 'EXPORTED'
  | 'DUPLICATE'

export type SagaStatus = 'NOT_READY' | 'READY' | 'EXPORTING' | 'EXPORTED' | 'FAILED'
export type TaskType = 'CONTRACT_MATCH' | 'MISSING_CONTRACT' | 'COMMERCIAL_REVIEW' | 'CLASSIFICATION'
export type TaskStatus = 'OPEN' | 'WAITING' | 'RESOLVED'
export type ReviewStatus = 'PENDING' | 'ACCEPTED' | 'CORRECTED' | 'REJECTED'
export type ClassificationReviewAction = 'APPROVE' | 'EDIT' | 'REJECT'
export type ClassificationSource = 'RULE' | 'NO_MATCH' | 'AMBIGUOUS' | 'LEARNED_MAPPING' | 'AI_PROPOSAL'
export type AccountMappingAction = 'NONE' | 'CREATE' | 'VALIDATE' | 'OCCURRENCE_ONLY' | 'CORRECT' | 'POLICY_CHANGE'
export interface AccountMappingReference { mappingId: string; version: number; accountCode: string; serviceIdentityKind: string; serviceIdentityValue: string; normalizerVersion: string; revision: number }
export interface AccountMappingScopePreview { clientDisplay:string; supplierDisplay:string; serviceIdentityKind:string; serviceIdentityValue:string; normalizerVersion:string }
export interface KnowledgeReference { id:string;version:number;sourceInvoiceId:string;sourceInvoiceLineId:string;sourceClassificationId:string;promotedBy:string;promotedAt:string }
export interface KnowledgeScope {clientId:string;clientDisplay:string;supplierDisplay:string;normalizedSupplierId:string;serviceIdentityKind:string;serviceIdentityValue:string;normalizerVersion:string;currency:string;documentType:string;vatRate:string;profileId?:string;profileVersion?:number}
export interface PromotionPreview {classificationId:string;classificationRevision:number;classificationRunId:string;dimension:RuleCategory;value:DomainValue;scope:KnowledgeScope}
export interface ApprovedKnowledge {id:string;version:number;dimension:RuleCategory;value:DomainValue;scope:KnowledgeScope;status:'ACTIVE'|'STALE'|'REVOKED';staleReason?:string;sourceInvoiceId:string;sourceInvoiceLineId:string;sourceClassificationId:string;sourceClassificationRunId:string;originalSource:string;promotedBy:string;promotedAt:string;revision:number;legislationVersionIds?:string[]}
export interface LegislationSourceView {id:string;kind:string;title:string;issuer:string;officialUrl:string;versionId:string;versionLabel:string;effectiveFrom:string;effectiveTo?:string;status:'ACTIVE'|'EXPIRED'|'FUTURE';fragmentCount:number}
export type RuleCategory = 'ACCOUNT' | 'VAT' | 'DEDUCTIBILITY' | 'VAT_TREATMENT' | 'VAT_DEDUCTIBILITY' | 'EXPENSE_TAX_TREATMENT'
export type RuleScope = 'GLOBAL' | 'CLIENT_OVERRIDE'

export interface Client {
 company?:import('./client-management').Company
 normalizedIdentifier?:string
 status?:import('./client-management').ClientLifecycle
 revision?:number
 createdAt?:string
 updatedAt?:string
  id: string
  name: string
  cui: string
}

export interface Money {
  amount: number
  currency: string
}

export interface ContractCandidate {
  id: string
  reference: string
  supplierName: string
  period: string
  value: Money
  confidence: string
  reasons: string[]
  unitType?: string
  paymentTerms?: string
  recommended?: boolean
}

export interface ContractSummary {
  id: string
  reference: string
  supplierName: string
  period: string
  value: Money
  currency: string
  unitType: string
  paymentTerms: string
  hasLegacyTotalValue?:boolean
}

export interface Contract extends ContractSummary {
  clientId: string
  revision?:number
  lifecycleState?:'ACTIVE'|'ARCHIVED'
  sourceReference?: string
  sourceMetadata?: string
  sourceDocumentId?:string
  extractionAttemptId?:string
  periodType?:'FIXED_TERM'|'INDEFINITE_TERM'
  serviceTerms?:Array<{serviceDescription:string;pricingModel:'FIXED_FEE'|'UNIT_RATE'|'FIXED_TOTAL';unitPrice?:string;currency:string;unit?:string;quantitySource:string;quantityValue?:string;quantityDriver?:string;billingFrequency:string;evidence?:{page?:number;snippet?:string}}>
}

export interface RuleProvenance {
 sourceType: string
 sourceTitle: string
 issuer: string
 legalInstrument: string
 reference: string
 sourceURL: string
 effectiveFrom: string
 effectiveTo?: string
 verifiedAt: string
 verifiedBy: string
 notes: string
 accountingRegime?: string
 clientPolicyReference?: string
}

export interface RuleReference {
 ruleVersionId?: string
 productionEligible?: boolean
 rulePackVersion?: string
 provenance?: RuleProvenance
 effectiveFrom?: string
 effectiveTo?: string
  ruleId: string
  reference: string
  version: number
  origin: RuleScope
}

export interface RuleVersion {
 id?: string
 productionEligible?: boolean
 rulePackVersion?: string
 provenance?: RuleProvenance
  version: number
  effectiveFrom: string
  effectiveTo?: string
  criteria: string
  result: string
  explanation?: string
  legalBasis: string
  createdAt: string
  actor: string
}

export interface ClassificationRule {
  id: string
  reference: string
  name: string
  category: RuleCategory
  scope: RuleScope
  clientId?: string
  parentRuleId?: string
  versions: RuleVersion[]
  revision?: number
}

export interface LineClassification {
 modelVersion?: string
 typedValue?: DomainValue
 proposedTypedValue?: DomainValue
 evidence?: DecisionEvidence
 legalCitations?: LegalCitation[]
 validationResults?: ClassificationValidation[]
 proposalProvenance?: ProposalProvenance
 reviewReason?: string
 invoiceDateUsed?: string
 humanReviewed?: boolean
  id: string
  dimension: RuleCategory
  value: string
  confidence?: string
  explanation: string
  legalBasis: string
 status: ReviewStatus
 effectiveSource?: string
  rule?: RuleReference
  revision?: number
  source?: ClassificationSource
  mapping?: AccountMappingReference
  mappingScope?: AccountMappingScopePreview
  knowledge?: KnowledgeReference
}

export interface ClassificationReviewItem {
 modelVersion?: string
 typedValue?: DomainValue
 proposedTypedValue?: DomainValue
 evidence?: DecisionEvidence
 legalCitations?: LegalCitation[]
 validationResults?: ClassificationValidation[]
 proposalProvenance?: ProposalProvenance
 reviewReason?: string
 invoiceDateUsed?: string
 humanReviewed?: boolean
  id: string
  lineId: string
  lineLabel: string
  dimension: RuleCategory
  proposedValue: string
  confidence: string
  explanation: string
  legalBasis: string
  status: ReviewStatus
  effectiveSource?: string
  resolvedValue?: string
  rule?: RuleReference
  revision?: number
  source?: ClassificationSource
  mapping?: AccountMappingReference
  mappingScope?: AccountMappingScopePreview
  knowledge?: KnowledgeReference
}

export interface ValidationTask {
  id: string
  type: TaskType
  status: TaskStatus
  createdAt: string
  waitingSince?: string
  title: string
  reason: string
  contractCandidates?: ContractCandidate[]
  classificationItems?: ClassificationReviewItem[]
  contractRequested?: boolean
  revision?: number
  classificationRunId?:string
}

export interface ValidationTaskInboxItem {
  task: ValidationTask
  invoice: Invoice
  client?: Client
}

export interface InvoiceLine {
 sourceFacts?: LineSourceFacts
  id: string
  position: number
  description: string
  unit: string
  quantity: number
  unitPrice: Money
  netValue: Money
  vatLabel: string
  vatValue: Money
  grossValue: Money
  additionalInfo?: string
  classifications: LineClassification[]
}

export interface ActivityEvent {
  id: string
  label: string
  actor: string
  timestamp: string
  detail: string
  before?: string
  after?: string
}

export interface SagaExport {
	attemptId: string
	artifactStatus: 'GENERATED' | 'FAILED'
	filename?: string
	generatedAt: string
	downloadedAt?: string
	confirmedAt?: string
	confirmedBy?: string
	confirmationType?: 'HUMAN' | 'LOCAL_AGENT' | 'SAGA_API'
	invoiceRevision: number
}

export type DemoScenario = 'HAPPY_PATH' | 'MULTIPLE_CONTRACTS' | 'MISSING_CONTRACT' | 'UNCERTAIN_CLASSIFICATION' | 'PROCESSING' | 'DUPLICATE' | 'SAGA_FAILURE'

export interface Invoice {
 modelVersion?: string
 sourceFacts?: InvoiceSourceFacts
 readinessReason?: string
 currentClassificationRunId?: string
 accountingWorkflowStatus?: 'APPLYING_RULES'|'AI_ANALYSIS_PENDING'|'AI_ANALYSIS_RUNNING'|'REVIEW_REQUIRED'|'COMPLETED'
 classificationContext?: {runId:string;profileId?:string;profileVersion?:number;contextStale:boolean;staleReasons:string[];createdAt:string}
 accountingSnapshot?: { profile?: AccountingProfileSnapshot; pack?: { id: string; version: number; testOnly: boolean; mapping: { version: string; approved: boolean } } }
  id: string
  scenario: DemoScenario
  primaryDemo: boolean
  clientId: string
  supplierName: string
  supplierCui?: string
  documentNumber: string
  issueDate: string
  dueDate?: string
  total: Money
  spvReference: string
  pipelineStatus: PipelineStatus
  pipelinePath: PipelineStatus[]
  sagaStatus: SagaStatus
  confidence?: string
  autoRun: boolean
  selectedContractId?: string
  contract?: ContractSummary
  task?: ValidationTask
  lines: InvoiceLine[]
	activity: ActivityEvent[]
	sagaExport?: SagaExport
  revision?: number
  authority?: 'MOCK' | 'API'
}

export const PIPELINE_LABELS: Record<PipelineStatus, string> = {
  DOWNLOADED: 'Descărcată din SPV',
  ARCHIVED: 'Arhivată',
  MATCHING: 'Asociere contract',
  AWAITING_CONTRACT: 'Așteaptă contract',
  AWAITING_MATCH_CONFIRM: 'Așteaptă confirmare',
  DEDUPE_CHECKED: 'Duplicate verificate',
  HEADER_READ: 'Antet citit',
  LINES_READ: 'Linii citite',
  COMMERCIAL_VALIDATING: 'Validare comercială',
  AWAITING_COMMERCIAL_REVIEW: 'Așteaptă review comercial',
  COMMERCIALLY_VALIDATED: 'Validată comercial',
  CLASSIFIED: 'Clasificată',
  AWAITING_REVIEW: 'Așteaptă revizuirea',
  READY_FOR_SAGA: 'Pregătită pentru SAGA',
  EXPORTING: 'Export în curs',
  EXPORTED: 'Exportată în SAGA',
  DUPLICATE: 'Duplicat',
}

export const TASK_TYPE_LABELS: Record<TaskType, string> = {
  CONTRACT_MATCH: 'Verificare contract',
  MISSING_CONTRACT: 'Contract lipsă',
  COMMERCIAL_REVIEW: 'Review comercial',
  CLASSIFICATION: 'Revizuire clasificare',
}

export const TASK_STATUS_LABELS: Record<TaskStatus, string> = {
  OPEN: 'Deschis',
  WAITING: 'În așteptare',
  RESOLVED: 'Rezolvat',
}

export const CLASSIFICATION_DIMENSION_LABELS: Record<RuleCategory, string> = {
  ACCOUNT: 'Cont contabil',
  VAT: 'Confirmare istorică a cotei TVA',
  DEDUCTIBILITY: 'Instrucțiune SAGA istorică',
 VAT_TREATMENT: 'Tratament TVA',
 VAT_DEDUCTIBILITY: 'Drept de deducere TVA',
 EXPENSE_TAX_TREATMENT: 'Tratament fiscal al cheltuielii — impozit pe profit',
}

export const TERMINAL_STATES: PipelineStatus[] = ['EXPORTED', 'DUPLICATE']

export interface DomainValue {
 kind: string
 account?: string
 percentage?: string
 basis?: string
 category?: string
 reason?: string
 timing?: string
 sourceCategory?: string
 sourceRate?: string
}
export interface DecisionEvidence { modelVersion: string; profileId: string; profileVersion: number; policyId: string; packId?: string; packVersion?: number; dateBasis: string; date: string; sourceDocumentId: string; sourcePath: string; parserVersion: string; sourceHash: string; ruleVersionId?: string }
export interface LegalCitation { fragmentId:string;versionId:string;citationKey:string;contentHash:string;verified:boolean }
export interface ClassificationValidation { code:string;message:string;suggestedAccounts?:string[] }
export interface ProposalProvenance { analysisRunId:string;provider:string;model:string;schemaVersion:string;promptVersion:string;knowledgeId?:string;knowledgeVersion?:number;sourceInvoiceId?:string;sourceLineId?:string;sourceDecisionId?:string;promotedBy?:string;promotedAt?:string }
export interface LineSourceFacts { sourceId: string; path: string; code: string; rate?: string; scheme?: string; exemptionCode?: string; exemptionReason?: string; vatOrigin: 'DECLARED' | 'CALCULATED' | 'UNKNOWN' }
export interface InvoiceSourceFacts { parserVersion: string; sourceDocumentId?: string; sourceHash?: string; typeCode?: string; supplierCountry?: string; buyerCountry?: string; taxPointDate?: string; periodStart?: string; periodEnd?: string; taxPointCode?: string; supplierVatId?: string; supplierLegalId?: string; buyerVatId?: string; buyerLegalId?: string; taxCurrency?: string; cashAccounting: string }

export interface AccountingProfileSnapshot { id: string; clientId: string; version: number; chartPolicy: string; framework?: string; taxRegime?: string; vatRegistration?: string; deductionActivity?: string; cashAccounting?: string; proRata?: string; effectiveFrom?: string; effectiveTo?: string; testOnly?: boolean }
