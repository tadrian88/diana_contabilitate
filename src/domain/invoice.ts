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
  | 'CLASSIFIED'
  | 'AWAITING_REVIEW'
  | 'READY_FOR_SAGA'
  | 'EXPORTING'
  | 'EXPORTED'
  | 'DUPLICATE'

export type SagaStatus = 'NOT_READY' | 'READY' | 'EXPORTING' | 'EXPORTED' | 'FAILED'
export type TaskType = 'CONTRACT_MATCH' | 'MISSING_CONTRACT' | 'CLASSIFICATION'
export type TaskStatus = 'OPEN' | 'WAITING' | 'RESOLVED'
export type ReviewStatus = 'PENDING' | 'ACCEPTED' | 'CORRECTED'
export type RuleCategory = 'ACCOUNT' | 'VAT' | 'DEDUCTIBILITY'
export type RuleScope = 'GLOBAL' | 'CLIENT_OVERRIDE'

export interface Client {
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
}

export interface Contract extends ContractSummary {
  clientId: string
  sourceReference?: string
  sourceMetadata?: string
}

export interface RuleReference {
  ruleId: string
  reference: string
  version: number
  origin: RuleScope
}

export interface RuleVersion {
  version: number
  effectiveFrom: string
  effectiveTo?: string
  criteria: string
  result: string
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
}

export interface LineClassification {
  id: string
  dimension: 'ACCOUNT' | 'VAT' | 'DEDUCTIBILITY'
  value: string
  confidence?: string
  explanation: string
  legalBasis: string
  status: ReviewStatus
  rule?: RuleReference
}

export interface ClassificationReviewItem {
  id: string
  lineId: string
  lineLabel: string
  dimension: 'ACCOUNT' | 'VAT' | 'DEDUCTIBILITY'
  proposedValue: string
  confidence: string
  explanation: string
  legalBasis: string
  status: ReviewStatus
  resolvedValue?: string
  rule?: RuleReference
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
}

export interface InvoiceLine {
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

export type DemoScenario = 'HAPPY_PATH' | 'MULTIPLE_CONTRACTS' | 'MISSING_CONTRACT' | 'UNCERTAIN_CLASSIFICATION' | 'PROCESSING' | 'DUPLICATE' | 'SAGA_FAILURE'

export interface Invoice {
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
  CLASSIFICATION: 'Revizuire clasificare',
}

export const TASK_STATUS_LABELS: Record<TaskStatus, string> = {
  OPEN: 'Deschis',
  WAITING: 'În așteptare',
  RESOLVED: 'Rezolvat',
}

export const CLASSIFICATION_DIMENSION_LABELS: Record<RuleCategory, string> = {
  ACCOUNT: 'Cont contabil',
  VAT: 'Cotă TVA',
  DEDUCTIBILITY: 'Deductibilitate',
}

export const TERMINAL_STATES: PipelineStatus[] = ['EXPORTED', 'DUPLICATE']
