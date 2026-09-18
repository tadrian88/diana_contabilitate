import type { AxiosInstance } from 'axios'
import { apiClient } from '../../features/auth/auth-api'
import type { ActivityEvent, ClassificationRule, Client, ClientScope, Contract, InvoiceLine, Invoice, LineClassification, PipelineStatus, SagaExport, SagaStatus, ValidationTask, ValidationTaskInboxItem } from '../../domain/invoice'
import type { ContractDocument, ReviewedContract, CreateClientOverrideInput, CreateRuleVersionInput, InvoiceRepository, SPVConnection } from '../invoiceRepository'

interface InvoiceReadDto {
 modelVersion?: string
 sourceFacts?: Invoice['sourceFacts']
 accountingSnapshot?: Invoice['accountingSnapshot']
 readinessReason?: string
  id: string
  clientId: string
  supplierName: string
  supplierCui?: string
  documentNumber: string
  issueDate: string
  dueDate?: string
  total: { amount: number; currency: string }
  spvReference: string
  pipelineStatus: PipelineStatus
  sagaStatus: SagaStatus
  revision: number
  activity: ActivityEvent[]
  task?: ValidationTask
  selectedContractId?: string
  contract?: Invoice['contract']
	lines?: Array<{
    sourceFacts?: InvoiceLine['sourceFacts']
    id: string
    position: number
    description: string
    unit: string
    vatRate: number
    vatValue: { amount: number; currency: string }
    quantity: number
    unitPrice: { amount: number; currency: string }
    netValue: { amount: number; currency: string }
    totalValue: { amount: number; currency: string }
    additionalInfo?: string
    classifications?: LineClassification[]
	}>
	sagaExport?: SagaExport
}

interface ValidationTaskInboxDto {
  task: ValidationTask
  invoice: {
    id: string
    clientId: string
    supplierName: string
    supplierCui?: string
    documentNumber: string
    issueDate: string
    total: { amount: number; currency: string }
    spvReference: string
    pipelineStatus: PipelineStatus
    sagaStatus: SagaStatus
  }
  client: Client
}

interface ContractDto extends Contract {}

interface ContractInvoiceDto {
  id: string
  clientId: string
  supplierName: string
  documentNumber: string
  issueDate: string
  total: { amount: number; currency: string }
  spvReference: string
  pipelineStatus: PipelineStatus
  sagaStatus: SagaStatus
}

export class ApiInvoiceReadRepository implements InvoiceRepository {
  readonly runtimeAuthority = 'API' as const

  constructor(private readonly http: Pick<AxiosInstance, 'get' | 'post'> = apiClient) {}

  async getClientDetail(clientId:string):Promise<import('../../domain/client-management').ClientDetail>{
    return (await this.http.get<import('../../domain/client-management').ClientDetail>(`/clients/${encodeURIComponent(clientId)}`)).data
  }
  async writeClient(input:import('../../domain/client-management').ClientWrite,key:string):Promise<import('../../domain/client-management').ClientDetail>{
    const {kind,...body}=input
    const route=kind==='create'?'/clients':`/clients/${encodeURIComponent('clientId' in input?input.clientId:'')}/${kind}`
    const payload:Record<string,unknown>={...body};delete payload.clientId
    return (await this.http.post<import('../../domain/client-management').ClientDetail>(route,payload,{headers:{'Idempotency-Key':key}})).data
  }

  async listClients(): Promise<Client[]> {
    const response = await this.http.get<Client[]>('/clients')
    return response.data
  }

  async listInvoices(scope: ClientScope): Promise<Invoice[]> {
    const response = await this.http.get<InvoiceReadDto[]>('/invoices', {
      params: scope === 'all' ? undefined : { clientId: scope },
    })
    return response.data.map(mapInvoice)
  }

  async getInvoice(id: string): Promise<Invoice | undefined> {
    const response = await this.http.get<InvoiceReadDto>(`/invoices/${encodeURIComponent(id)}`, {
      validateStatus: (status) => status === 200 || status === 404,
    })
    if (response.status === 404) return undefined
    return mapInvoice(response.data)
  }

  async listValidationTasks(scope: ClientScope): Promise<ValidationTaskInboxItem[]> {
    const response = await this.http.get<ValidationTaskInboxDto[]>('/validation-tasks', {
      params: scope === 'all' ? undefined : { clientId: scope },
    })
    return response.data.map((item) => ({
      task: item.task,
      client: item.client,
      invoice: mapTaskInvoice(item),
    }))
  }

  async requestContract(id: string): Promise<Invoice> {
    const invoice = await this.requireInvoice(id)
    if (!invoice.task?.revision) throw new Error('Task revision is required for a contract request.')
    const response = await this.http.post<InvoiceReadDto>(`/invoices/${encodeURIComponent(invoice.id)}/contract-requests`, {
      taskId: invoice.task.id,
      expectedRevision: invoice.task.revision,
    }, { headers: { 'Idempotency-Key': crypto.randomUUID() } })
    return mapInvoice(response.data)
  }

  async listContracts(scope: ClientScope): Promise<Contract[]> {
    const response = await this.http.get<ContractDto[]>('/contracts', { params: scope === 'all' ? undefined : { clientId: scope } })
    return response.data
  }

  async getContract(id: string): Promise<Contract | undefined> {
    const response = await this.http.get<ContractDto>(`/contracts/${encodeURIComponent(id)}`, {
      validateStatus: (status) => status === 200 || status === 404,
    })
    return response.status === 404 ? undefined : response.data
  }

  async listContractInvoices(contractId: string): Promise<Invoice[]> {
    const response = await this.http.get<ContractInvoiceDto[]>(`/contracts/${encodeURIComponent(contractId)}/invoices`)
    return response.data.map((invoice) => mapContractInvoice(invoice, contractId))
  }

  async listContractDocuments(clientId: string) { const response=await this.http.get<ContractDocument[]>(`/clients/${encodeURIComponent(clientId)}/contract-documents`);return response.data }
  async getContractDocument(clientId:string,documentId:string){const response=await this.http.get<ContractDocument>(`/clients/${encodeURIComponent(clientId)}/contract-documents/${encodeURIComponent(documentId)}`,{validateStatus:(status)=>status===200||status===404});return response.status===404?undefined:response.data}
  async uploadContractDocument(clientId:string,file:File){const form=new FormData();form.append('file',file);const response=await this.http.post<ContractDocument>(`/clients/${encodeURIComponent(clientId)}/contract-documents`,form,{headers:{'Content-Type':'multipart/form-data'}});return response.data}
  async confirmContractDocument(clientId:string,document:ContractDocument,contract:ReviewedContract,key=crypto.randomUUID()){if(!document.extraction?.id)throw new Error('Extraction attempt is required.');const response=await this.http.post<{contractId:string;changed:boolean}>(`/clients/${encodeURIComponent(clientId)}/contract-documents/${encodeURIComponent(document.id)}/confirm`,{extractionAttemptId:document.extraction.id,expectedDocumentRevision:document.revision,contract},{headers:{'Idempotency-Key':key}});return response.data}
  async getContractDocumentFile(clientId:string,documentId:string){const response=await this.http.get<Blob>(`/clients/${encodeURIComponent(clientId)}/contract-documents/${encodeURIComponent(documentId)}/file`,{responseType:'blob'});return response.data}
  async retryContractExtraction(clientId:string,documentId:string,revision:number){await this.http.post(`/clients/${encodeURIComponent(clientId)}/contract-documents/${encodeURIComponent(documentId)}/reextract`,{expectedDocumentRevision:revision})}
  async discardContractDocument(clientId:string,documentId:string,revision:number){await this.http.post(`/clients/${encodeURIComponent(clientId)}/contract-documents/${encodeURIComponent(documentId)}/discard`,{expectedDocumentRevision:revision})}

  async resolveContractMatch(id: string, contractId: string): Promise<Invoice> {
    const invoice = await this.requireInvoice(id)
    if (!invoice.revision || !invoice.task?.revision) throw new Error('Invoice and task revisions are required for contract confirmation.')
    const response = await this.http.post<InvoiceReadDto>(`/invoices/${encodeURIComponent(invoice.id)}/contract-confirmations`, {
      taskId: invoice.task.id,
      contractId,
      expectedInvoiceRevision: invoice.revision,
      expectedTaskRevision: invoice.task.revision,
    }, { headers: { 'Idempotency-Key': crypto.randomUUID() } })
    return mapInvoice(response.data)
  }

  async listRules(scope: ClientScope): Promise<ClassificationRule[]> {
    const response = await this.http.get<ClassificationRule[]>('/rules', { params: scope === 'all' ? undefined : { clientId: scope } })
    return response.data
  }

  async getRule(id: string): Promise<ClassificationRule | undefined> {
    const response = await this.http.get<ClassificationRule>(`/rules/${encodeURIComponent(id)}`, { validateStatus: (status) => status === 200 || status === 404 })
    return response.status === 404 ? undefined : response.data
  }

  async createRuleVersion(id: string, input: CreateRuleVersionInput): Promise<ClassificationRule> {
    const current = await this.getRule(id)
    if (!current?.revision) throw new Error('Rule revision is required for a new version.')
    const response = await this.http.post<ClassificationRule>(`/rules/${encodeURIComponent(id)}/versions`, { ...input, expectedRevision: current.revision }, { headers: { 'Idempotency-Key': crypto.randomUUID() } })
    return response.data
  }

  async createClientOverride(id: string, input: CreateClientOverrideInput): Promise<ClassificationRule> {
    const response = await this.http.post<ClassificationRule>(`/rules/${encodeURIComponent(id)}/client-overrides`, input, { headers: { 'Idempotency-Key': crypto.randomUUID() } })
    return response.data
  }

  async reviewClassification(id: string, itemId: string, correctedValue?: string, typedValue?: import('../../domain/invoice').DomainValue, reason?: string): Promise<Invoice> {
    const invoice = await this.requireInvoice(id)
    const task = invoice.task
    const item = task?.classificationItems?.find((candidate) => candidate.id === itemId)
    if (!invoice.revision || !task?.revision || !item?.revision) throw new Error('Invoice, task, and classification revisions are required for review.')
    const response = await this.http.post<InvoiceReadDto>(`/invoices/${encodeURIComponent(invoice.id)}/classification-decisions`, {
      taskId: task.id,
      classificationId: item.id,
      expectedInvoiceRevision: invoice.revision,
      expectedTaskRevision: task.revision,
      expectedClassificationRevision: item.revision,
      ...(correctedValue === undefined ? {} : { correctedValue }),
 ...(typedValue === undefined ? {} : { typedValue }),
 ...(reason === undefined ? {} : { reason }),
    }, { headers: { 'Idempotency-Key': crypto.randomUUID() } })
    return mapInvoice(response.data)
  }

  async getSPVConnection(clientId: string): Promise<SPVConnection> {
    const response = await this.http.get<SPVConnection>(`/clients/${encodeURIComponent(clientId)}/spv`)
    return response.data
  }

  async startSPVOAuth(clientId: string): Promise<string> {
    const response = await this.http.post<{ authorizationUrl: string }>(`/clients/${encodeURIComponent(clientId)}/spv/oauth/start`,undefined,{headers:{'Idempotency-Key':crypto.randomUUID()}})
    return response.data.authorizationUrl
  }

  async requestSPVSync(clientId: string): Promise<SPVConnection> {
    const response = await this.http.post<SPVConnection>(`/clients/${encodeURIComponent(clientId)}/spv/sync`, undefined, { headers: { 'Idempotency-Key': crypto.randomUUID() } })
    return response.data
  }

	async disconnectSPV(clientId: string): Promise<SPVConnection> {
    const response = await this.http.post<SPVConnection>(`/clients/${encodeURIComponent(clientId)}/spv/disconnect`, undefined, { headers: { 'Idempotency-Key': crypto.randomUUID() } })
    return response.data
	}

	async downloadSagaArtifact(clientId: string, invoiceId: string) {
		const response = await this.http.get<Blob>(`/clients/${encodeURIComponent(clientId)}/invoices/${encodeURIComponent(invoiceId)}/saga-export/artifact`, { responseType: 'blob' })
		const disposition = String(response.headers?.['content-disposition'] ?? '')
		const encoded = disposition.match(/filename\*=UTF-8''([^;]+)/i)?.[1]
		const quoted = disposition.match(/filename="([^"]+)"/i)?.[1]
		const filename = encoded ? decodeURIComponent(encoded) : quoted ?? `saga-${invoiceId}.xml`
		return { blob: response.data, filename }
	}

	async confirmSagaImport(clientId: string, invoiceId: string, attemptId: string, expectedInvoiceRevision: number, note?: string) {
		const response = await this.http.post<{ invoice: InvoiceReadDto }>(`/clients/${encodeURIComponent(clientId)}/invoices/${encodeURIComponent(invoiceId)}/saga-export/confirm-import`, {
			attemptId, expectedInvoiceRevision, ...(note ? { note } : {}),
		}, { headers: { 'Idempotency-Key': crypto.randomUUID() } })
		return mapInvoice(response.data.invoice)
	}

  private async requireInvoice(id: string): Promise<Invoice> {
    const invoice = await this.getInvoice(id)
    if (!invoice) throw new Error('Invoice was not found.')
    return invoice
  }
}

function mapInvoice(value: InvoiceReadDto): Invoice {
  return {
    id: value.id,
    modelVersion: value.modelVersion, sourceFacts: value.sourceFacts, accountingSnapshot: value.accountingSnapshot, readinessReason: value.readinessReason,
    scenario: 'PROCESSING',
    primaryDemo: false,
    clientId: value.clientId,
    supplierName: value.supplierName,
    supplierCui: value.supplierCui,
    documentNumber: value.documentNumber,
    issueDate: value.issueDate,
    dueDate: value.dueDate,
    total: value.total,
    spvReference: value.spvReference,
    pipelineStatus: value.pipelineStatus,
    pipelinePath: pipelinePath(value.pipelineStatus),
    sagaStatus: value.sagaStatus,
    autoRun: false,
    lines: (value.lines ?? []).map((line): InvoiceLine => ({
      id: line.id, position: line.position, description: line.description, unit: line.unit,
      sourceFacts: line.sourceFacts, vatLabel: line.sourceFacts && line.sourceFacts.rate === undefined ? 'Cotă absentă' : `${line.vatRate}%`, vatValue: line.vatValue, quantity: line.quantity,
      unitPrice: line.unitPrice, netValue: line.netValue, grossValue: line.totalValue,
      additionalInfo: line.additionalInfo, classifications: line.classifications ?? [],
    })),
    activity: value.activity,
    task: value.task,
    selectedContractId: value.selectedContractId,
    contract: value.contract,
    revision: value.revision,
		authority: 'API',
		sagaExport: value.sagaExport,
	}
}

function mapTaskInvoice(item: ValidationTaskInboxDto): Invoice {
  const value = item.invoice
  return {
    id: value.id, scenario: 'PROCESSING', primaryDemo: false, clientId: value.clientId,
    supplierName: value.supplierName, supplierCui: value.supplierCui, documentNumber: value.documentNumber,
    issueDate: value.issueDate, total: value.total, spvReference: value.spvReference,
    pipelineStatus: value.pipelineStatus, pipelinePath: pipelinePath(value.pipelineStatus), sagaStatus: value.sagaStatus,
    autoRun: false, task: item.task, lines: [], activity: [], authority: 'API',
  }
}

function mapContractInvoice(value: ContractInvoiceDto, contractId: string): Invoice {
  return {
    id: value.id, scenario: 'PROCESSING', primaryDemo: false, clientId: value.clientId,
    supplierName: value.supplierName, documentNumber: value.documentNumber, issueDate: value.issueDate,
    total: value.total, spvReference: value.spvReference, pipelineStatus: value.pipelineStatus,
    pipelinePath: pipelinePath(value.pipelineStatus), sagaStatus: value.sagaStatus,
    autoRun: false, lines: [], activity: [], selectedContractId: contractId, authority: 'API',
  }
}

function pipelinePath(status: PipelineStatus): PipelineStatus[] {
  if (status === 'DUPLICATE') return ['DOWNLOADED', 'ARCHIVED', 'MATCHING', 'DEDUPE_CHECKED', 'DUPLICATE']
  if (status === 'AWAITING_CONTRACT') return ['DOWNLOADED', 'ARCHIVED', 'MATCHING', 'AWAITING_CONTRACT']
  const path: PipelineStatus[] = ['DOWNLOADED', 'ARCHIVED', 'MATCHING']
  if (status === 'AWAITING_MATCH_CONFIRM') path.push('AWAITING_MATCH_CONFIRM')
  path.push('DEDUPE_CHECKED', 'HEADER_READ', 'LINES_READ', 'CLASSIFIED')
  if (status === 'AWAITING_REVIEW') path.push('AWAITING_REVIEW')
  path.push('READY_FOR_SAGA', 'EXPORTING', 'EXPORTED')
  return path
}
