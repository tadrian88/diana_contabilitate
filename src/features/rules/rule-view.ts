import { CLASSIFICATION_DIMENSION_LABELS } from '../../domain/invoice'
import type { RuleScope } from '../../domain/invoice'

export const RULE_CATEGORY_LABELS = CLASSIFICATION_DIMENSION_LABELS

export const RULE_SCOPE_LABELS: Record<RuleScope, string> = {
  GLOBAL: 'Regulă globală',
  CLIENT_OVERRIDE: 'Override client',
}
