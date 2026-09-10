import type { ClientScope, Invoice, PipelineStatus } from '../domain/invoice'
import type { InvoiceRepository } from '../repositories/invoiceRepository'
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
  private invoices = new Map(mockInvoices.map((invoice) => [invoice.id, clone(invoice)]))
  private contracts = new Map(mockContracts.map((contract) => [contract.id, clone(contract)]))
  private rules = new Map(mockRules.map((rule) => [rule.id, clone(rule)]))

  async listClients() {
    return clone(mockClients)
  }

  async listInvoices(scope: ClientScope) {
    const invoices = [...this.invoices.values()]
    return clone(scope === 'all' ? invoices : invoices.filter((invoice) => invoice.clientId === scope))
  }

  async getInvoice(id: string) {
    const invoice = this.invoices.get(id)
    return invoice ? clone(invoice) : undefined
  }

  async listContracts(scope: ClientScope) {
    const contracts = [...this.contracts.values()]
    return clone(scope === 'all' ? contracts : contracts.filter((contract) => contract.clientId === scope))
  }

  async getContract(id: string) {
    const contract = this.contracts.get(id)
    return contract ? clone(contract) : undefined
  }

  async listRules(scope: ClientScope) {
    const rules = [...this.rules.values()]
    return clone(scope === 'all' ? rules : rules.filter((rule) => rule.scope === 'GLOBAL' || rule.clientId === scope))
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
