import type { ClassificationRule, Client, ClientScope, Contract, Invoice, PipelineStatus, ValidationTaskInboxItem } from '../domain/invoice'

export interface SagaArtifactDownload { blob: Blob; filename: string }

export type ContractIngestionStatus = 'UPLOADED' | 'EXTRACTING' | 'READY_FOR_REVIEW' | 'EXTRACTION_FAILED' | 'CONFIRMED'
export interface ExtractedContractField { value: string | null; status: 'PRESENT' | 'MISSING' | 'AMBIGUOUS'; confidence: 'HIGH' | 'MEDIUM' | 'LOW' | 'UNKNOWN'; evidence: { page: number | null; snippet?: string }; alternatives: string[] }
export interface ContractProposal { supplierName: ExtractedContractField; supplierCui: ExtractedContractField; reference: ExtractedContractField; effectiveFrom: ExtractedContractField; effectiveTo: ExtractedContractField; totalValue: ExtractedContractField; currency: ExtractedContractField; unitType: ExtractedContractField; paymentTerms: ExtractedContractField; buyerCui: ExtractedContractField }
export interface ContractDocument { id:string; clientId:string; originalFilename:string; mimeType:string; sizeBytes:number; sha256:string; status:ContractIngestionStatus; lifecycleState:string; revision:number; uploadedAt:string; uploadedBy?:string; confirmedAt?:string; confirmedBy?:string; confirmedContractId?:string; buyerMismatch:boolean; duplicate?:boolean; extraction?:{id:string;provider:string;model:string;schemaVersion:string;promptVersion:string;status:'STARTED'|'SUCCEEDED'|'FAILED';proposal?:ContractProposal;safeErrorCategory?:string;startedAt:string;completedAt?:string} }
export interface ReviewedContract { supplierName:string;supplierCui:string;reference:string;effectiveFrom:string;effectiveTo:string;totalValue:string;currency:string;unitType:string;paymentTerms:string }

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
  listContractInvoices(contractId: string): Promise<Invoice[]>
  listContractDocuments(clientId: string): Promise<ContractDocument[]>
  getContractDocument(clientId: string, documentId: string): Promise<ContractDocument | undefined>
  uploadContractDocument(clientId: string, file: File): Promise<ContractDocument>
  confirmContractDocument(clientId: string, document: ContractDocument, contract: ReviewedContract, key?:string): Promise<{contractId:string;changed:boolean}>
  getContractDocumentFile(clientId:string,documentId:string):Promise<Blob>
  retryContractExtraction(clientId:string,documentId:string,revision:number):Promise<void>
  listRules(scope: ClientScope): Promise<ClassificationRule[]>
  getRule(id: string): Promise<ClassificationRule | undefined>
  createRuleVersion(id: string, input: CreateRuleVersionInput): Promise<ClassificationRule>
  createClientOverride(id: string, input: CreateClientOverrideInput): Promise<ClassificationRule>
  resolveContractMatch(id: string, contractId: string): Promise<Invoice>
  requestContract(id: string): Promise<Invoice>
  reviewClassification(id: string, itemId: string, value?: string, typedValue?: import('../domain/invoice').DomainValue, reason?: string): Promise<Invoice>
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
