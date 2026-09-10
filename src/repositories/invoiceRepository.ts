import type { ClassificationRule, Client, ClientScope, Contract, Invoice, PipelineStatus } from '../domain/invoice'

export interface CreateRuleVersionInput {
  criteria: string
  result: string
  effectiveFrom: string
  effectiveTo?: string
}

export interface CreateClientOverrideInput extends CreateRuleVersionInput {
  clientId: string
}

export interface InvoiceRepository {
  listClients(): Promise<Client[]>
  listInvoices(scope: ClientScope): Promise<Invoice[]>
  getInvoice(id: string): Promise<Invoice | undefined>
  listContracts(scope: ClientScope): Promise<Contract[]>
  getContract(id: string): Promise<Contract | undefined>
  listRules(scope: ClientScope): Promise<ClassificationRule[]>
  getRule(id: string): Promise<ClassificationRule | undefined>
  createRuleVersion(id: string, input: CreateRuleVersionInput): Promise<ClassificationRule>
  createClientOverride(id: string, input: CreateClientOverrideInput): Promise<ClassificationRule>
  startAutomaticFlow(id: string): Promise<Invoice>
  transitionInvoice(id: string, status: PipelineStatus): Promise<Invoice>
  resolveContractMatch(id: string, contractId: string): Promise<Invoice>
  requestContract(id: string): Promise<Invoice>
  reviewClassification(id: string, itemId: string, value?: string): Promise<Invoice>
}
