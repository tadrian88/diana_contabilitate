import type { ClassificationRule, Client, ClientScope, Contract, Invoice, PipelineStatus, ValidationTaskInboxItem } from '../domain/invoice'

export interface SagaArtifactDownload { blob: Blob; filename: string }
export interface AccountCatalogEntry { code:string;name:string;accountType:string;synthetic:boolean;postable:boolean;active:boolean }

export type ContractIngestionStatus = 'UPLOADED' | 'EXTRACTING' | 'READY_FOR_REVIEW' | 'EXTRACTION_FAILED' | 'CONFIRMED'
export type ContractExtractionSafeErrorCategory = 'EXTRACTOR_CONFIGURATION'|'SOURCE_INTEGRITY'|'PROVIDER_AUTHENTICATION'|'PROVIDER_REJECTED'|'PROVIDER_RATE_LIMIT'|'PROVIDER_UNAVAILABLE'|'PROVIDER_NETWORK'|'TIMEOUT'|'INTERRUPTED'|'INVALID_PROVIDER_ENVELOPE'|'INVALID_STRUCTURED_OUTPUT'|'PROPOSAL_VALIDATION_FAILED'|'NO_CONTRACT_DATA'|'LEASE_EXPIRED'|'INVALID_OUTPUT'|'PROVIDER_TRANSIENT'
export interface ExtractedContractField { value: string | null; status: 'PRESENT' | 'MISSING' | 'AMBIGUOUS'; confidence: 'HIGH' | 'MEDIUM' | 'LOW' | 'UNKNOWN'; evidence: { page: number | null; snippet?: string }; alternatives: string[] }
export interface ProposedServiceTerm { serviceDescription:ExtractedContractField;pricingModel:ExtractedContractField;unitPrice:ExtractedContractField;currency:ExtractedContractField;unit:ExtractedContractField;quantitySource:ExtractedContractField;quantityValue:ExtractedContractField;quantityDriver:ExtractedContractField;billingFrequency:ExtractedContractField }
export interface CommercialExpression {op:'literal'|'variable'|'add'|'multiply'|'percent'|'tier'|'min'|'max'|'prorate'|'fx'|'round';value?:string;variable?:string;args?:CommercialExpression[];tiers?:Array<{upTo?:string|null;value:string}>;scale?:number}
export interface CommercialRule {id:string;kind:string;narrative:string;applicability:{serviceId?:string;skus?:string[];aliases?:string[];documentTypes?:string[];billingFrequency?:string;contractYear?:number|null;tranche?:number|null};dateBasis:string;currency?:string;expression?:CommercialExpression|null;requiredVariables?:string[];evidence:Array<{documentId:string;page?:number|null;snippet:string}>;blocking:boolean}
export interface ProposedCommercialClause {kind:ExtractedContractField;narrative:ExtractedContractField;rule:CommercialRule;evidence:{page:number|null;snippet?:string};confidence:'HIGH'|'MEDIUM'|'LOW'|'UNKNOWN'}
export interface ContractProposal { supplierName: ExtractedContractField; supplierCui: ExtractedContractField; reference: ExtractedContractField; effectiveFrom: ExtractedContractField; effectiveTo: ExtractedContractField; totalValue: ExtractedContractField; currency: ExtractedContractField; unitType: ExtractedContractField; paymentTerms: ExtractedContractField; buyerCui: ExtractedContractField;periodType:ExtractedContractField;documentRole?:ExtractedContractField;relatedReference?:ExtractedContractField;serviceTerms:ProposedServiceTerm[];commercialClauses?:ProposedCommercialClause[] }
export interface ContractDocument { id:string; clientId:string; clientCui?:string;originalFilename:string; mimeType:string; sizeBytes:number; sha256:string; status:ContractIngestionStatus; lifecycleState:string; revision:number; uploadedAt:string; uploadedBy?:string; confirmedAt?:string; confirmedBy?:string; confirmedContractId?:string; buyerMismatch:boolean; duplicate?:boolean; extraction?:{id:string;provider:string;model:string;schemaVersion:string;promptVersion:string;status:'STARTED'|'SUCCEEDED'|'FAILED';proposal?:ContractProposal;safeErrorCategory?:ContractExtractionSafeErrorCategory;startedAt:string;completedAt?:string} }
export interface ReviewedServiceTerm {serviceDescription:string;pricingModel:''|'FIXED_FEE'|'UNIT_RATE'|'FIXED_TOTAL';unitPrice:string;currency:string;unit:string;quantitySource:'CONTRACT_FIXED_QUANTITY'|'INVOICE_REPORTED_QUANTITY'|'USER_CONFIRMED_QUANTITY'|'EXTERNAL_SOURCE_FUTURE'|'UNKNOWN';quantityValue:string;quantityDriver:string;billingFrequency:'MONTHLY'|'QUARTERLY'|'ANNUAL'|'PER_OCCURRENCE'|'UNKNOWN';evidence:{page:number|null;snippet?:string}}
export interface ReviewedContract { supplierName:string;supplierCui:string;reference:string;effectiveFrom:string;effectiveTo:string;totalValue:string;currency:string;unitType:string;paymentTerms:string;buyerCui:string;periodType:'FIXED_TERM'|'INDEFINITE_TERM';documentRole:'BASE_CONTRACT'|'ANNEX'|'AMENDMENT'|'SOW'|'ORDER'|'PRICE_LIST'|'OTHER';relatedReference:string;coverage:'COMPLETE'|'PARTIAL'|'CONFLICTED';serviceTerms:ReviewedServiceTerm[];commercialRules:CommercialRule[] }
export type CommercialOutcome='CONFORM'|'NECONFORM'|'NEVERIFICABIL'
export interface CommercialFinding {id:string;ruleId:string;code:string;outcome:CommercialOutcome;lineId?:string;actual?:string;actualSource?:string;expected?:string;calculation?:string;reason:string;missingInputs?:string[];serviceCandidates?:Array<{ruleId:string;label:string}>;evidence?:Array<{documentId:string;page?:number;snippet:string}>;override?:{reason:string;actorDisplay?:string;at:string}}
export interface CommercialValidationRun {id:string;invoiceId:string;snapshotId?:string;dossierId?:string;invoiceRevision:number;snapshotVersion:number;engineVersion:string;outcome:CommercialOutcome;findings:CommercialFinding[];createdAt:string;completedAt:string}

export interface ContractDocument { attempts?:Array<NonNullable<ContractDocument['extraction']>>; confirmedValues?:ReviewedContract }

export interface CreateRuleVersionInput {
  criteria: string
  result: string
  effectiveFrom: string
  effectiveTo?: string
}

export interface CreateClientOverrideInput extends CreateRuleVersionInput {
  clientId: string
}

export type SPVConnectionStatus = 'NOT_CONNECTED' | 'CONNECTED' | 'NEEDS_REAUTHENTICATION' | 'ERROR' | 'DISABLED'
export type SPVSyncStatus = 'NEVER' | 'RUNNING' | 'SUCCEEDED' | 'FAILED'

export interface SPVConnection {
  status: SPVConnectionStatus
  environment?: 'TEST' | 'PRODUCTION'
  connectedAt?: string
  lastSyncAt?: string
  lastSuccessfulSyncAt?: string
  lastSyncStatus: SPVSyncStatus
  safeErrorCode?: string
  safeError?: string
  importAutomatic: boolean
  configurationReady: boolean
  identityValidation: 'NOT_AVAILABLE'
}

export interface InvoiceRepository {
  readonly runtimeAuthority: 'API' | 'MOCK'
  listClients(): Promise<Client[]>
  getClientDetail(clientId:string):Promise<import('../domain/client-management').ClientDetail>
  writeClient(input:import('../domain/client-management').ClientWrite,key:string):Promise<import('../domain/client-management').ClientDetail>
  listInvoices(scope: ClientScope): Promise<Invoice[]>
  getInvoice(id: string): Promise<Invoice | undefined>
  listValidationTasks(scope: ClientScope): Promise<ValidationTaskInboxItem[]>
  listContracts(scope: ClientScope): Promise<Contract[]>
  getContract(id: string): Promise<Contract | undefined>
  deleteContract(id:string,expectedRevision:number):Promise<{changed:boolean}>
  archiveContract(id:string,expectedRevision:number):Promise<{changed:boolean}>
  listContractInvoices(contractId: string): Promise<Invoice[]>
  listContractDocuments(clientId: string): Promise<ContractDocument[]>
  getContractDocument(clientId: string, documentId: string): Promise<ContractDocument | undefined>
  uploadContractDocument(clientId: string, file: File): Promise<ContractDocument>
  confirmContractDocument(clientId: string, document: ContractDocument, contract: ReviewedContract, key?:string): Promise<{contractId:string;changed:boolean}>
  confirmProposedCommercialRule(clientId:string,documentId:string,ruleId:string,rule?:CommercialRule,key?:string):Promise<{changed:boolean}>
  activateReviewedServicePrices(clientId:string,documentId:string,key?:string):Promise<{activated:number}>
  putCommercialDateFact(clientId:string,invoiceId:string,input:{kind:'REMITTANCE'|'RECEIPT'|'ACCEPTANCE';date:string;sourceReference:string},key?:string):Promise<{changed:boolean}>
  getContractDocumentFile(clientId:string,documentId:string):Promise<Blob>
  retryContractExtraction(clientId:string,documentId:string,revision:number):Promise<void>
  discardContractDocument(clientId:string,documentId:string,revision:number):Promise<void>
  getCommercialValidation(clientId:string,invoiceId:string):Promise<CommercialValidationRun|undefined>
  resolveCommercialValidation(clientId:string,invoiceId:string,input:{runId:string;findingId?:string;expectedInvoiceRevision:number;action:'ACCEPT_EXCEPTION'|'WAIT_FOR_CORRECTION'|'RERUN';reason?:string},key?:string):Promise<{changed:boolean}>
  putCommercialVariable(clientId:string,dossierId:string,input:{name:string;value:string;source:'MANUAL';sourceReference:string;periodStart?:string;periodEnd?:string},key?:string):Promise<{changed:boolean}>
  confirmCommercialServiceAlias(clientId:string,input:{invoiceId:string;lineId:string;serviceId:string;reuseForDossier:boolean},key?:string):Promise<{changed:boolean}>
  listRules(scope: ClientScope): Promise<ClassificationRule[]>
  searchAccounts(query:string):Promise<AccountCatalogEntry[]>
  getRule(id: string): Promise<ClassificationRule | undefined>
  createRuleVersion(id: string, input: CreateRuleVersionInput): Promise<ClassificationRule>
  createClientOverride(id: string, input: CreateClientOverrideInput): Promise<ClassificationRule>
  resolveContractMatch(id: string, contractId: string): Promise<Invoice>
  requestContract(id: string): Promise<Invoice>
  reviewClassification(id: string, itemId: string, value?: string, typedValue?: import('../domain/invoice').DomainValue, reason?: string, mappingAction?:import('../domain/invoice').AccountMappingAction, expectedMappingRevision?:number): Promise<Invoice>
  getSPVConnection(clientId: string): Promise<SPVConnection>
  startSPVOAuth(clientId: string): Promise<string>
  requestSPVSync(clientId: string): Promise<SPVConnection>
	disconnectSPV(clientId: string): Promise<SPVConnection>
	downloadSagaArtifact(clientId: string, invoiceId: string): Promise<SagaArtifactDownload>
	confirmSagaImport(clientId: string, invoiceId: string, attemptId: string, expectedInvoiceRevision: number, note?: string): Promise<Invoice>
}

export interface MockWorkflowRepository extends InvoiceRepository {
  readonly runtimeAuthority: 'MOCK'
  startAutomaticFlow(id: string): Promise<Invoice>
  transitionInvoice(id: string, status: PipelineStatus): Promise<Invoice>
}

export function isMockWorkflowRepository(repository: InvoiceRepository): repository is MockWorkflowRepository {
  return repository.runtimeAuthority === 'MOCK'
}
