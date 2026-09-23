import {emptyCompany,type ClientDetail,type ClientWrite,type ReadinessSection} from '../domain/client-management'
import type { ClientScope, Invoice, PipelineStatus } from '../domain/invoice'
import type { ContractDocument, InvoiceRepository, ReviewedContract, SPVConnection } from '../repositories/invoiceRepository'
import { mockClients, mockContracts, mockInvoices, mockRules } from './scenarios'
import type { CreateClientOverrideInput, CreateRuleVersionInput } from '../repositories/invoiceRepository'

const clone = <T,>(value: T): T => structuredClone(value)

const activityFor = (invoice: Invoice, label: string, detail: string, before?: string, after?: string): Invoice['activity'][number] => ({
  id: `activity-${invoice.id}-${invoice.activity.length + 1}`,
  label,
  actor: 'Contabil demo',
  timestamp: `2026-09-${String(10 + invoice.activity.length).padStart(2, '0')}T09:00:00.000Z`,
  detail,
  before,
  after,
})

export class MockInvoiceRepository implements InvoiceRepository {
  readonly runtimeAuthority = 'MOCK' as const
  private invoices = new Map(mockInvoices.map((invoice) => [invoice.id, clone(invoice)]))
  private contracts = new Map(mockContracts.map((contract) => [contract.id, clone(contract)]))
  private rules = new Map(mockRules.map((rule) => [rule.id, clone(rule)]))
  private spvConnections = new Map<string, SPVConnection>([
    ['client-alfa', { status: 'NOT_CONNECTED', lastSyncStatus: 'NEVER', importAutomatic: false, configurationReady: true, identityValidation: 'NOT_AVAILABLE' }],
    ['client-beta', { status: 'CONNECTED', environment: 'TEST', connectedAt: '2026-09-14T09:00:00.000Z', lastSyncAt: '2026-09-14T10:00:00.000Z', lastSuccessfulSyncAt: '2026-09-14T10:00:00.000Z', lastSyncStatus: 'SUCCEEDED', importAutomatic: true, configurationReady: true, identityValidation: 'NOT_AVAILABLE' }],
  ])
  private clientDetails = new Map<string,ClientDetail>()
  private clientCommands = new Map<string,{payload:string;clientId:string}>()
  private contractDocuments = new Map<string, ContractDocument>()
  private contractDocumentFiles = new Map<string,File>()

  async listClients() {
    return clone([...mockClients.filter(c=>!this.clientDetails.has(c.id)),...this.clientDetails.values()].map(c=>'client' in c?c.client:c))
  }

  async getClientDetail(clientId:string):Promise<ClientDetail>{
    let d=this.clientDetails.get(clientId)
    if(!d){const client=mockClients.find(c=>c.id===clientId);if(!client)throw new Error('Client inexistent');d={client:{...client,company:{...emptyCompany,name:client.name,cui:client.cui},status:'ACTIVE',revision:1},profiles:[],sagaEnabled:false,history:[],onboarding:{} as ClientDetail['onboarding']};this.clientDetails.set(clientId,d)}
    const section=(status:ReadinessSection['status'],explanation:string,target:string):ReadinessSection=>({status,explanation,target,nextAction:'Vezi configurarea'})
    const date=new Date().toISOString().slice(0,10);const current=d.profiles.find(p=>p.effectiveFrom<=date&&(!p.effectiveTo||p.effectiveTo>=date));const spv=await this.getSPVConnection(clientId)
    d.onboarding={company:section('READY','Identitatea companiei este salvată.','company'),accountingProfile:section(current?.approval?.actor?'READY':d.profiles.length?'INCOMPLETE':'NOT_STARTED',current?.approval?.actor?'Profil aplicabil aprobat.':'Profil configurat sau aprobare necesară.','accounting-profile'),anaf:section(spv.status==='CONNECTED'?'READY':spv.status==='NOT_CONNECTED'?'NOT_STARTED':'ACTION_REQUIRED','Certificatul rămâne în browser/token; OAuth nu confirmă acoperirea CUI.','anaf-spv'),saga:section(d.sagaEnabled?'READY':'NOT_STARTED',d.sagaEnabled?'Export SAGA configurat; validare în așteptare.':'Export XML neconfigurat.','saga-setup'),classification:section('NOT_STARTED','Reguli de producție neconfigurate. Facturile pot fi importate și trimise la revizuire.','classification-status'),overall:'CONFIGURATION_INCOMPLETE',blockers:['Reguli de producție neconfigurate'],currentProfileId:current?.id,sagaConfigurationReady:d.sagaEnabled,sagaMappingApproved:false,sagaValidation:'VALIDATION_PENDING'}
    if(current?.approval?.actor&&spv.status==='CONNECTED'&&d.sagaEnabled)d.onboarding.overall='CORE_CONFIGURED_AUTOMATION_PENDING'
    if(d.client.status==='INACTIVE')d.onboarding.overall='INACTIVE'
    return clone(d)
  }
  private clientWriteQueue:Promise<unknown>=Promise.resolve()
  writeClient(input:ClientWrite,key:string):Promise<ClientDetail>{const pending=this.clientWriteQueue.then(()=>this.executeClientWrite(input,key));this.clientWriteQueue=pending.catch(()=>undefined);return pending}
  private async executeClientWrite(input:ClientWrite,key:string):Promise<ClientDetail>{
    const payload=JSON.stringify(input);const prior=this.clientCommands.get(key);if(prior){if(prior.payload!==payload)throw new Error('Comandă reutilizată');return this.getClientDetail(prior.clientId)}
    const id=input.kind==='create'?`client-created-${this.clientDetails.size+1}`:input.clientId
    if(input.kind==='create'||input.kind==='company'){
      const c=input.company;const normalized=c.country==='RO'?c.cui.toUpperCase().replace(/^RO\s*/,'').trim():c.cui.toUpperCase().trim()
      const existing=await this.listClients()
      if(!c.name.trim()||!c.cui.trim()||(c.country==='RO'&&!/^[1-9][0-9]{1,9}$/.test(normalized)))throw new Error('Identitate invalidă')
      if(existing.some(v=>v.id!==id&&(v.company?.country??'RO')===c.country&&v.cui.toUpperCase().replace(/^RO\s*/,'').trim()===normalized))throw new Error('Companie duplicată')
      if(input.kind==='create')this.clientDetails.set(id,{client:{id,name:c.name,cui:c.cui,company:clone(c),status:'ONBOARDING',revision:1,normalizedIdentifier:normalized},profiles:[],history:[],sagaEnabled:false,onboarding:{} as ClientDetail['onboarding']})
    }
    const d=await this.getClientDetail(id)
    if(input.kind!=='create'&&input.expectedRevision!==d.client.revision)throw new Error('Revizie învechită')
    let label='Client creat'
    if(input.kind==='company'){
      if(input.company.cui!==d.client.cui&&(d.profiles.length||this.invoices.size&&[...this.invoices.values()].some(i=>i.clientId===id)||d.client.cui.startsWith('RO-DEMO')))throw new Error('Identitate protejată')
      d.client={...d.client,name:input.company.name,cui:input.company.cui,company:clone(input.company)};label='Date companie actualizate'
    }
    if(input.kind==='lifecycle'){d.client.status=input.status;label=input.status==='INACTIVE'?'Client dezactivat':'Client reactivat'}
    if(input.kind==='saga-configuration'){d.sagaEnabled=input.sagaEnabled;label='Configurație export SAGA actualizată'}
    if(input.kind==='accounting-profiles'){
      if(input.expectedProfileVersion!==(d.profiles[0]?.version??0)||input.profile.effectiveTo&&input.profile.effectiveTo<input.profile.effectiveFrom)throw new Error('Versiune sau perioadă invalidă')
      if(input.approve&&(!input.evidence.length||d.profiles.some(p=>p.approval?.actor&&(!p.effectiveTo||input.profile.effectiveFrom<=p.effectiveTo)&&(!input.profile.effectiveTo||p.effectiveFrom<=input.profile.effectiveTo))))throw new Error('Dovezi sau suprapunere invalidă')
      d.profiles.unshift({...clone(input.profile),id:`profile-${id}-${d.profiles.length+1}`,clientId:id,version:d.profiles.length+1,testOnly:false,approval:input.approve?{actor:'Contabil demo',at:new Date().toISOString(),evidence:input.evidence}:undefined});label=input.approve?'Versiune profil aprobată':'Versiune profil configurată'
    }
    if(input.kind!=='create')d.client.revision=(d.client.revision??1)+1
    d.history.unshift({id:`history-${id}-${d.history.length+1}`,label,actor:'Contabil demo',timestamp:new Date().toISOString(),detail:label});this.clientDetails.set(id,d)
    const detail=await this.getClientDetail(id);this.clientCommands.set(key,{payload,clientId:id});return detail
  }

  async listInvoices(scope: ClientScope) {
    const invoices = [...this.invoices.values()]
    return clone(scope === 'all' ? invoices : invoices.filter((invoice) => invoice.clientId === scope))
  }

  async getCommercialValidation(){return undefined}
  async resolveCommercialValidation(){return {changed:false}}
  async putCommercialVariable(){return {changed:false}}
  async confirmCommercialServiceAlias(){return {changed:false}}
  async activateReviewedServicePrices(){return {activated:0}}
  async putCommercialDateFact(){return {changed:false}}

  async getInvoice(id: string) {
    const invoice = this.invoices.get(id)
    return invoice ? clone(invoice) : undefined
  }

  async listValidationTasks(scope: ClientScope) {
    const invoices = [...this.invoices.values()].filter((invoice) => invoice.task && (scope === 'all' || invoice.clientId === scope))
    return clone(invoices.map((invoice) => ({
      task: invoice.task!,
      invoice,
      client: mockClients.find((client) => client.id === invoice.clientId),
    })))
  }

  async listContracts(scope: ClientScope) {
    const contracts = [...this.contracts.values()].filter(contract=>contract.lifecycleState!=='ARCHIVED')
    return clone(scope === 'all' ? contracts : contracts.filter((contract) => contract.clientId === scope))
  }

  async getContract(id: string) {
    const contract = this.contracts.get(id)
    return contract ? clone(contract) : undefined
  }

  async deleteContract(id:string,expectedRevision:number){const contract=this.contracts.get(id);if(!contract)throw new Error('Contract inexistent');if([...this.invoices.values()].some(invoice=>invoice.selectedContractId===id))throw new Error('Contract folosit de facturi');if((contract.revision??1)!==expectedRevision)throw new Error('Revizie învechită');if(contract.lifecycleState==='ARCHIVED'&&contract.sourceDocumentId===undefined)return {changed:false};this.contracts.set(id,{...contract,lifecycleState:'ARCHIVED',revision:expectedRevision+1});if(contract.sourceDocumentId){const document=this.contractDocuments.get(contract.sourceDocumentId);if(document){document.lifecycleState='DISCARDED';document.revision++}}return {changed:true}}
  async archiveContract(id:string,expectedRevision:number){const contract=this.contracts.get(id);if(!contract)throw new Error('Contract inexistent');if(contract.lifecycleState==='ARCHIVED')return {changed:false};if((contract.revision??1)!==expectedRevision)throw new Error('Revizie învechită');this.contracts.set(id,{...contract,lifecycleState:'ARCHIVED',revision:expectedRevision+1});return {changed:true}}

  async listContractInvoices(contractId: string) {
    return clone([...this.invoices.values()].filter((invoice) => invoice.selectedContractId === contractId))
  }

  async listContractDocuments(clientId:string){return clone([...this.contractDocuments.values()].filter(item=>item.clientId===clientId&&item.lifecycleState!=='DISCARDED'))}
  async getContractDocument(clientId:string,documentId:string){const item=this.contractDocuments.get(documentId);return item?.clientId===clientId?clone(item):undefined}
async uploadContractDocument(clientId:string,file:File){const id=`contract-document-${this.contractDocuments.size+1}`;const field=(value:string|null,page:number|null=1)=>({value,status:value?'PRESENT' as const:'MISSING' as const,confidence:value?'HIGH' as const:'UNKNOWN' as const,evidence:{page,snippet:value??''},alternatives:[]});const item:ContractDocument={id,clientId,clientCui:'RO10000000',originalFilename:file.name,mimeType:'application/pdf',sizeBytes:file.size,sha256:`mock-${id}`,status:'READY_FOR_REVIEW',lifecycleState:'ACTIVE',revision:2,uploadedAt:new Date().toISOString(),uploadedBy:'Contabil demo',buyerMismatch:false,extraction:{id:`extraction-${id}`,provider:'DETERMINISTIC_DEMO',model:'fake-contract-extractor',schemaVersion:'CONTRACT_EXTRACTION_V2',promptVersion:'CONTRACT_EXTRACTION_PROMPT_V2',status:'SUCCEEDED',startedAt:new Date().toISOString(),completedAt:new Date().toISOString(),proposal:{supplierName:field('Furnizor extras SRL'),supplierCui:field('RO12345678'),reference:field('CTR-2026-01'),effectiveFrom:field('2026-01-01'),effectiveTo:field('2027-12-31',2),totalValue:field('125000.00',3),currency:field('RON',3),unitType:field('servicii'),paymentTerms:field('30 zile'),buyerCui:field('RO10000000'),periodType:field('FIXED_TERM'),serviceTerms:[]}}};this.contractDocumentFiles.set(id,file);this.contractDocuments.set(id,item);return clone(item)}
  async confirmContractDocument(clientId:string,document:ContractDocument,input:ReviewedContract){const item=this.contractDocuments.get(document.id);if(!item||item.clientId!==clientId||item.revision!==document.revision)throw new Error('Extragerea s-a modificat.');item.status='CONFIRMED';item.revision++;item.confirmedValues=clone(input);item.confirmedContractId=`contract-confirmed-${document.id}`;item.confirmedAt=new Date().toISOString();item.confirmedBy='Contabil demo';this.contracts.set(item.confirmedContractId,{id:item.confirmedContractId,clientId,reference:input.reference,supplierName:input.supplierName,period:`${input.effectiveFrom} — ${input.effectiveTo}`,value:{amount:Number(input.totalValue),currency:input.currency},currency:input.currency,unitType:input.unitType,paymentTerms:input.paymentTerms,sourceReference:item.originalFilename,sourceMetadata:item.extraction?.schemaVersion,sourceDocumentId:item.id});return {contractId:item.confirmedContractId,changed:true}}
  async confirmProposedCommercialRule(){return {changed:false}}
  async getContractDocumentFile(clientId:string,documentId:string){const item=this.contractDocuments.get(documentId);if(item?.clientId!==clientId)throw new Error('Documentul nu există.');return this.contractDocumentFiles.get(documentId)??new Blob(['%PDF-1.4\n%%EOF'],{type:'application/pdf'})}
  async retryContractExtraction(clientId:string,documentId:string,revision:number){const item=this.contractDocuments.get(documentId);if(!item||item.clientId!==clientId||item.revision!==revision)throw new Error('Extragerea s-a modificat.');item.status='READY_FOR_REVIEW';item.revision++}
  async discardContractDocument(clientId:string,documentId:string,revision:number){const item=this.contractDocuments.get(documentId);if(!item||item.clientId!==clientId||item.revision!==revision||item.status==='CONFIRMED')throw new Error('Documentul nu poate fi șters.');this.contractDocuments.delete(documentId);this.contractDocumentFiles.delete(documentId)}

  async listRules(scope: ClientScope) {
    const rules = [...this.rules.values()]
    return clone(scope === 'all' ? rules : rules.filter((rule) => rule.scope === 'GLOBAL' || rule.clientId === scope))
  }

  async searchAccounts(query:string) {
    const items=[{code:'6281',name:'Cheltuieli cu serviciile IT',accountType:'expense',synthetic:false,postable:true,active:true},{code:'6262',name:'Cheltuieli cu telecomunicațiile',accountType:'expense',synthetic:false,postable:true,active:true}]
    const needle=query.trim().toLocaleLowerCase('ro')
    return items.filter(item=>!needle||item.code.includes(needle)||item.name.toLocaleLowerCase('ro').includes(needle))
  }

  async getRule(id: string) {
    const rule = this.rules.get(id)
    return rule ? clone(rule) : undefined
  }

  async createRuleVersion(id: string, input: CreateRuleVersionInput) {
    const rule = this.requireRule(id)
    const version = Math.max(...rule.versions.map((candidate) => candidate.version)) + 1
    rule.versions.push({
      version,
      ...clone(input),
      legalBasis: 'Exemplu demonstrativ — bază legală nevalidată',
      createdAt: `2026-09-${String(12 + version).padStart(2, '0')}T09:00:00.000Z`,
      actor: 'Contabil demo',
    })
    return clone(rule)
  }

  async createClientOverride(id: string, input: CreateClientOverrideInput) {
    const parent = this.requireRule(id)
    if (parent.scope !== 'GLOBAL') throw new Error('Override-ul poate porni numai dintr-o regulă globală.')
    const client = mockClients.find((candidate) => candidate.id === input.clientId)
    if (!client) throw new Error('Clientul nu există.')
    const sequence = [...this.rules.values()].filter((rule) => rule.parentRuleId === parent.id && rule.clientId === input.clientId).length + 1
    const override = {
      id: `${parent.id}-override-${input.clientId}-${sequence}`,
      reference: `${parent.reference}-OVR-${input.clientId === 'client-alfa' ? 'ALFA' : 'BETA'}-${sequence}`,
      name: `${parent.name} — variație ${client.name}`,
      category: parent.category,
      scope: 'CLIENT_OVERRIDE' as const,
      clientId: input.clientId,
      parentRuleId: parent.id,
      versions: [{
        version: 1,
        criteria: input.criteria,
        result: input.result,
        effectiveFrom: input.effectiveFrom,
        effectiveTo: input.effectiveTo,
        legalBasis: 'Exemplu demonstrativ — bază legală nevalidată',
        createdAt: '2026-09-15T09:00:00.000Z',
        actor: 'Contabil demo',
      }],
    }
    this.rules.set(override.id, override)
    return clone(override)
  }

  async startAutomaticFlow(id: string) {
    const invoice = this.requireInvoice(id)
    invoice.autoRun = true
    return clone(invoice)
  }

  async transitionInvoice(id: string, status: PipelineStatus) {
    const invoice = this.requireInvoice(id)
    const previousStatus = invoice.pipelineStatus
    invoice.pipelineStatus = status
    invoice.sagaStatus = status === 'READY_FOR_SAGA' ? 'READY' : status === 'EXPORTING' ? 'EXPORTING' : status === 'EXPORTED' ? 'EXPORTED' : invoice.sagaStatus
    invoice.activity.push(activityFor(invoice, 'Pipeline actualizat', `Stare simulată: ${status}.`, previousStatus, status))
    return clone(invoice)
  }

  async resolveContractMatch(id: string, contractId: string) {
    const invoice = this.requireInvoice(id)
    if (!invoice.task || invoice.task.type !== 'CONTRACT_MATCH') throw new Error('Task-ul de contract nu există.')
    invoice.selectedContractId = contractId
    const candidate = invoice.task.contractCandidates?.find((contractCandidate) => contractCandidate.id === contractId)
    const selectedContract = this.contracts.get(contractId)
    if (selectedContract) invoice.contract = clone(selectedContract)
    invoice.task.status = 'RESOLVED'
    invoice.pipelineStatus = 'DEDUPE_CHECKED'
    invoice.autoRun = true
    invoice.activity.push(activityFor(invoice, 'Contract confirmat', `Contract demonstrativ selectat: ${contractId}.`, 'Contract neconfirmat', candidate?.reference ?? contractId))
    return clone(invoice)
  }

  async requestContract(id: string) {
    const invoice = this.requireInvoice(id)
    if (!invoice.task || invoice.task.type !== 'MISSING_CONTRACT') throw new Error('Task-ul de contract lipsă nu există.')
    invoice.task.status = 'WAITING'
    invoice.task.contractRequested = true
    invoice.task.waitingSince = '2026-09-11T09:00:00.000Z'
    invoice.pipelineStatus = 'AWAITING_CONTRACT'
    invoice.activity.push(activityFor(invoice, 'Contract solicitat', 'Factura rămâne blocată până la primirea contractului.'))
    return clone(invoice)
  }

  async reviewClassification(id: string, itemId: string, value?: string) {
    const invoice = this.requireInvoice(id)
    const task = invoice.task
    if (!task || task.type !== 'CLASSIFICATION' || !task.classificationItems) throw new Error('Task-ul de clasificare nu există.')
    const item = task.classificationItems.find((candidate) => candidate.id === itemId)
    if (!item) throw new Error('Elementul de revizuire nu există.')
    item.status = value ? 'CORRECTED' : 'ACCEPTED'
    item.resolvedValue = value || item.proposedValue
    invoice.activity.push(activityFor(invoice, value ? 'Clasificare corectată' : 'Propunere acceptată', item.lineLabel, item.proposedValue, item.resolvedValue))
    if (task.classificationItems.every((candidate) => candidate.status !== 'PENDING')) {
      task.status = 'RESOLVED'
      invoice.pipelineStatus = 'READY_FOR_SAGA'
      invoice.sagaStatus = 'READY'
      invoice.autoRun = true
    }
    return clone(invoice)
  }

  async getSPVConnection(clientId: string): Promise<SPVConnection> {
    const fallback: SPVConnection = { status: 'NOT_CONNECTED', lastSyncStatus: 'NEVER', importAutomatic: false, configurationReady: true, identityValidation: 'NOT_AVAILABLE' }
    return clone(this.spvConnections.get(clientId) ?? fallback)
  }

  async startSPVOAuth(clientId: string) {
    return `https://anaf.example/authorize?state=demo-${encodeURIComponent(clientId)}`
  }

  async requestSPVSync(clientId: string) {
    const connection = this.spvConnections.get(clientId)
    if (!connection || connection.status !== 'CONNECTED') throw new Error('Conexiunea ANAF necesită reconectare.')
    connection.lastSyncStatus = 'RUNNING'
    return clone(connection)
  }

	async disconnectSPV(clientId: string) {
    const connection = this.spvConnections.get(clientId)
    if (!connection || connection.status === 'NOT_CONNECTED') throw new Error('Conexiunea ANAF nu există.')
    connection.status = 'DISABLED'; connection.importAutomatic = false
    return clone(connection)
	}

	async downloadSagaArtifact(_clientId: string, invoiceId: string) {
		const invoice = this.requireInvoice(invoiceId)
		if (invoice.sagaExport?.artifactStatus !== 'GENERATED') throw new Error('Fișierul SAGA nu este disponibil.')
		return { blob: new Blob(['<Facturi/>'], { type: 'application/xml' }), filename: invoice.sagaExport.filename ?? `saga-${invoiceId}.xml` }
	}

	async confirmSagaImport(clientId: string, invoiceId: string, attemptId: string) {
		const invoice = this.requireInvoice(invoiceId)
		if (invoice.clientId !== clientId || invoice.sagaExport?.attemptId !== attemptId || invoice.pipelineStatus !== 'EXPORTING') throw new Error('Exportul SAGA nu poate fi confirmat.')
		invoice.pipelineStatus = 'EXPORTED'; invoice.sagaStatus = 'EXPORTED'; invoice.autoRun = false
		invoice.sagaExport.confirmedAt = '2026-09-14T12:00:00.000Z'; invoice.sagaExport.confirmedBy = 'Contabil demo'; invoice.sagaExport.confirmationType = 'HUMAN'
		invoice.activity.push(activityFor(invoice, 'Import SAGA confirmat manual', 'Importul fișierului în SAGA a fost confirmat de contabil.', 'EXPORTING', 'EXPORTED'))
		return clone(invoice)
	}

  private requireInvoice(id: string) {
    const invoice = this.invoices.get(id)
    if (!invoice) throw new Error(`Factura ${id} nu există.`)
    return invoice
  }

  private requireRule(id: string) {
    const rule = this.rules.get(id)
    if (!rule) throw new Error(`Regula ${id} nu există.`)
    return rule
  }
}

export const mockInvoiceRepository = new MockInvoiceRepository()
