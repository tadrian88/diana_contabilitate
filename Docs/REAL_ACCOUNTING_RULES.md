# Real Accounting Rules V1

Status: **IMPLEMENTED — AWAITING USER-RUN TESTS / ACCOUNTING REVIEW; NOT FROZEN.**

Read [source discovery](ACCOUNTING_RULES_RESEARCH.md) and [test handoff](REAL_ACCOUNTING_RULES_TEST_HANDOFF.md) before evaluating accounting acceptance.

### Rule Architecture

Reuse Module 5 ClassificationRule / immutable RuleVersion / ClassificationPolicy. ACCOUNT, VAT, DEDUCTIBILITY; existing GLOBAL and direct CLIENT_OVERRIDE only. No new task or pipeline vocabulary. The default service and API/worker use `REAL_ACCOUNTING_RULES_V1`. Explicit `BaselinePolicy{}` remains confined to deterministic demo seeding and compatibility tests; no environment flag silently enables it in production.

### Production Eligibility

RuleVersion has immutable `production_eligible=false` by default, `rule_pack_version`, and optional structured `provenance`. All existing rows migrate to false without modifying their historical fields. Eligibility is explicit, never inferred from name, result, confidence, legal-basis text or enablement. The database rejects eligible rows with absent/invalid required provenance or the demo placeholder. The engine additionally validates exact allowed source hosts, source structure, match/value semantics, dimension, dates and policy requirements.

Public version/override creation remains compatible and creates NO_AUTOMATION, unverified versions. Filling in an accounting text cannot verify a rule. Production-reviewed configurations must be installed as reviewed immutable version data by a trusted maintenance/seeding process; this release exposes **no HTTP verification/promotion action**. Existing RequestActor/audit boundary is retained. Authorization for future verification is a production RBAC requirement, not a new invented role. Text and URL checks prove structure, not legal correctness; the independent accounting gate remains mandatory.

### Rule Pack Version

`RO_INCOMING_ACCOUNTING_V1_REVIEW_ONLY`: **zero production-seeded mappings**. `ProductionPack()` returns an explicit empty pack. No startup seeder loads it or substitutes demo mappings. Sources/data/policy gaps prohibit accepting speculative mappings. Future reviewed mappings fit declarative immutable version data. The name does not imply comprehensive Romanian law coverage.

### Effective Dating

`internal/accountingdate.Date` is an exact `YYYY-MM-DD` civil date with strict parsing. Existing timestamp DTO/domain boundaries preserve calendar components when converted; rule DB fields already use PostgreSQL date. Invoice-date-used is stored as PostgreSQL date, separately from processing timestamps. Runtime requires inclusive rule and source periods to contain the invoice issue date. Today's date, timestamp recency and timezone conversions never determine the applicable version.

Every version is loaded, rather than selecting versions[0]. Overlaps are intentionally allowed in history/configuration but cause review for a logical rule with multiple effective production versions; no newest-row winner. No version is edited to close a previous period. To introduce a disjoint period when the old immutable version was open-ended, the overlap safely remains review-only; a different lifecycle/amendment model is a future product decision, not an implicit history rewrite. Publish bounded periods when the period is authoritatively known; never fabricate expiry dates to make configuration convenient.

### Rule Precedence

Filter production/source/date eligibility first. Applicable CLIENT_OVERRIDE suppresses only its direct GLOBAL parent, as in Module 5. Future, expired or unverified overrides do not suppress a global production version. Description nonmatch of an otherwise eligible override follows existing parent-replacement semantics. Multiple same-level matches/overlapping logical versions produce AMBIGUOUS and review; no additional ranking hierarchy.

### Rule Evaluation

ACCOUNT supports an explicitly reviewed client policy, a narrow DESCRIPTION_CONTAINS condition and the partial account-628 vocabulary only; ALWAYS/global/default guesses are ineligible. This capability is not a shipped accounting mapping. Other legitimate manually chosen account codes keep the SAGA structural guard. VAT_SOURCE_RATE_EQUALS is a narrowly versioned condition over existing exact line VAT rate and present amount; output must numerically equal its positive match parameter and source percent. It confirms a supplier-declared rate only. No generic expressions, semantic matching or LLM calls. Zero tax categories, description VAT inference and automatic deductibility are unsupported.

### Legal Provenance

Immutable JSON: sourceType, sourceTitle, issuer, legalInstrument, reference, sourceURL, source effectiveFrom/effectiveTo, verifiedAt, verifiedBy, notes; optional accountingRegime/clientPolicyReference for account policy binding. Rule condition, output, creator, scope, rule period, pack and exact version remain separately modeled. Source URLs are provenance only; no runtime HTTP dependency. The API exposes historical metadata and safely rendered official external links.

### Audit

Decision persists exact rule-version FK, legal basis/explanation snapshot, proposed/final values, policy, immutable invoice-date-used and system activity. The joined immutable version provides source/period/pack; later insertions cannot change it. Manual correction preserves original proposal, rule FK, basis and date; reviewed-at/actor and correction activity remain authoritative. UI exposes evaluated date, source VAT context and historical rule link. Classification source stays RULE / NO_MATCH / AMBIGUOUS.

### SAGA Boundary

A final automatic result requires verified rule evidence, production policy, rule/source period agreement with its recorded date, and agreement with invoice issue date. Automatic DEDUCTIBILITY is refused. Demo/unverified/unattributed automatic results are rejected even if their output happens to be a valid number/account/code. Explicit accountant review (persisted reviewer and timestamp) can resolve these cases; it does not promote the originating rule. Numeric source VAT equality, exact arithmetic, account format and TipDeducere vocabulary guards remain. SAGA classification snapshots include rule, policy, date, pack, eligibility and legal-basis trace.

READY_FOR_SAGA follows existing all-dimension review completion. The default empty pack always creates CLASSIFICATION review. Fully automatic fiscal completion is intentionally unavailable while deductibility semantics are unresolved. The full-pipeline safety fixture therefore includes explicit accountant resolution; it does not pretend to prove an all-automatic Romanian tax treatment.

### Demo Isolation

Old fixture policy behavior is explicitly selected in unit/integration compatibility tests and guarded devseed. `devseed` rejects environments other than exact development/test. Synthetic SAGA fixtures now explicitly record fixture-accountant confirmation; they are not promoted to legal rules. Real API/worker default to production policy. Pre-existing demo decisions reaching READY in historical demo data still fail the real SAGA automatic evidence guard. Fake SAGA remains the already-approved development-only boundary.

### Supported Automatic Cases

The shipped production pack supports **none** pending accounting review. Architecture tests exercise positive supplier-source-rate confirmation and narrowly cited client account-628 policy under explicit TEST_ONLY assumptions. Configured production eligibility is necessary but not sufficient: engine predicate, provenance and temporal checks must all pass.

### Review-Only Cases

All deductions; unverified/demo rules; absent/ambiguous rules; overlapping active logical versions; missing date; unsupported document type; generic accounts; absent client policy; zero VAT, reverse charge, intra-community, import and exemptions; any percentage discrepancy. Accountant review is expected successful safety behavior.

### Idempotency and Concurrency

One repeatable-read read-only transaction loads invoice/header, ordered lines and all candidate versions. It establishes one MVCC snapshot before evaluation; no line×dimension×rule database queries. Evaluation runs locally on that snapshot even if a newer version is inserted before apply. Existing invoice-revision/status compare-and-swap and command/audit/task keys serialize writes and replay. Adding a version does not reclassify historical invoices. Rule updates are rejected by a database BEFORE UPDATE trigger in addition to Ent immutable fields. Existing FK restrictions protect referenced deletes; privileged fixture cleanup retains its prior behavior.

### Observability

Four counters expose only bounded ACCOUNT/VAT/DEDUCTIBILITY labels: classification_rule_evaluations_total, classification_rule_matches_total, classification_review_required_total, classification_production_rule_matches_total. Evaluations count applicable candidate predicates per dimension; production matches count accepted automatic decisions. No rule/client/account labels or invoice descriptions. No new sensitive-description logs; persisted activity identifies policy and retains exact per-decision trace.

### Migration and Frontend Contract

Additive `000013_production_accounting_rules.sql`; original migrations untouched, Atlas hash regenerated. Ent schema/generated code adds immutable metadata and invoice-date-used. Existing APIs only add optional metadata plus explicit boolean status; no parallel endpoints. Frontend edits are limited to eligibility/source/date/version display, official source links, source-VAT context and accurate human-confirmation labels. Existing effective-date editing and manual workflow remain. No frontend contract conflict identified by typecheck.

### Completion inventory

| Rule ID | Dimension | Condition | Output | Effective period | Authority | Automatic? | Reason |
| --- | --- | --- | --- | --- | --- | --- | --- |
| No production rule accepted | — | — | — | — | See source discovery | No | Sources do not establish deterministic mappings without missing policy/context |

## Superseding domain correction: Accounting V2 / Classification V1

The previous sections describe legacy three-dimensional rules and their historical SAGA boundary. They remain historical implementation records. Newly parsed V2 invoices now use four independent typed decisions: ACCOUNT, VAT_TREATMENT, VAT_DEDUCTIBILITY, EXPENSE_TAX_TREATMENT. Legacy VAT confirms the source rate; legacy DEDUCTIBILITY is a historical vendor import instruction. Neither is reinterpreted or backfilled.

Infrastructure is IMPLEMENTED — AWAITING USER-RUN TESTS. Real approved production rule pack remains EMPTY; no accountant-approved acquisition/chart/fiscal mapping or real SAGA acceptance was fabricated. Private reviewed operator tooling installs profile → immutable reviewed RuleVersions → exact approved pack/mapping. Ordinary Rule Administration cannot promote rules. Test fixtures are TEST_ONLY, isolated from production and not accounting evidence.

New-model readiness is authoritative and shared across automatic completion, human review and XML generation. Only an approved ordinary immediate FULL VAT / FULLY_DEDUCTIBLE expense mapping can omit TipDeducere. Unequal/limited/inapplicable/special/period-limit combinations stay AWAITING_REVIEW on the existing CLASSIFICATION task. Human confirmation cannot approve an unsupported mapping.

Classification V1: ENGINE IMPLEMENTED, REAL PACK / ACCEPTANCE REMAINS, NOT COMPLETE / NOT FROZEN. Classification V2: NOT STARTED. Approved accounting profile/chart/acquisition/supplier evidence, exact legally dated rules, expected four decisions and real SAGA import results are still required. Current definitions: [ACCOUNTING_DOMAIN_V2.md](ACCOUNTING_DOMAIN_V2.md), [CLASSIFICATION_ENGINE_V1.md](CLASSIFICATION_ENGINE_V1.md). User test commands: [CLASSIFICATION_V1_TEST_HANDOFF.md](CLASSIFICATION_V1_TEST_HANDOFF.md).
