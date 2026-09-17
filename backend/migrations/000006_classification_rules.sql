-- Add Backend Module 5 line classifications and immutable classification rules.
ALTER TABLE invoice_lines
    ADD CONSTRAINT invoice_lines_id_invoice_key UNIQUE (id, invoice_id);

CREATE TABLE classification_rules (
    id text PRIMARY KEY,
    reference text NOT NULL UNIQUE CHECK (length(reference) > 0),
    name text NOT NULL CHECK (length(name) > 0),
    category text NOT NULL CHECK (category IN ('ACCOUNT', 'VAT', 'DEDUCTIBILITY')),
    scope text NOT NULL CHECK (scope IN ('GLOBAL', 'CLIENT_OVERRIDE')),
    client_id text REFERENCES clients(id),
    parent_rule_id text,
    parent_scope text,
    creation_key text NOT NULL UNIQUE CHECK (length(creation_key) > 0),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT classification_rules_scope_shape CHECK (
        (scope = 'GLOBAL' AND client_id IS NULL AND parent_rule_id IS NULL AND parent_scope IS NULL)
        OR
        (scope = 'CLIENT_OVERRIDE' AND client_id IS NOT NULL AND parent_rule_id IS NOT NULL AND parent_scope = 'GLOBAL')
    ),
    CONSTRAINT classification_rules_identity_category_scope_key UNIQUE (id, category, scope),
    CONSTRAINT classification_rules_parent_global_fk
        FOREIGN KEY (parent_rule_id, category, parent_scope)
        REFERENCES classification_rules(id, category, scope)
);
CREATE INDEX classification_rules_scope_category_idx ON classification_rules (scope, category);
CREATE UNIQUE INDEX classification_rules_one_override_per_client
    ON classification_rules (parent_rule_id, client_id)
    WHERE scope = 'CLIENT_OVERRIDE';

CREATE TABLE rule_versions (
    id text PRIMARY KEY,
    rule_id text NOT NULL REFERENCES classification_rules(id),
    version integer NOT NULL CHECK (version > 0),
    criteria text NOT NULL CHECK (length(criteria) > 0),
    result text NOT NULL CHECK (length(result) > 0),
    explanation text NOT NULL CHECK (length(explanation) > 0),
    legal_basis text NOT NULL CHECK (length(legal_basis) > 0),
    match_kind text NOT NULL CHECK (match_kind IN ('DESCRIPTION_CONTAINS', 'ALWAYS', 'NO_AUTOMATION')),
    match_value text,
    effective_from date NOT NULL,
    effective_to date,
    created_by_id text,
    created_by_display text NOT NULL CHECK (length(created_by_display) > 0),
    command_key text NOT NULL UNIQUE CHECK (length(command_key) > 0),
    created_at timestamptz NOT NULL,
    CONSTRAINT rule_versions_rule_version_key UNIQUE (rule_id, version),
    CONSTRAINT rule_versions_effective_period CHECK (effective_to IS NULL OR effective_from <= effective_to),
    CONSTRAINT rule_versions_match_shape CHECK (
        (match_kind = 'DESCRIPTION_CONTAINS' AND match_value IS NOT NULL AND length(btrim(match_value)) > 0)
        OR (match_kind IN ('ALWAYS', 'NO_AUTOMATION') AND match_value IS NULL)
    )
);

CREATE TABLE line_classifications (
    id text PRIMARY KEY,
    client_id text NOT NULL REFERENCES clients(id),
    invoice_id text NOT NULL,
    invoice_line_id text NOT NULL,
    dimension text NOT NULL CHECK (dimension IN ('ACCOUNT', 'VAT', 'DEDUCTIBILITY')),
    proposed_value text NOT NULL CHECK (length(proposed_value) > 0),
    effective_value text,
    confidence_display text NOT NULL CHECK (length(confidence_display) > 0),
    explanation text NOT NULL CHECK (length(explanation) > 0),
    legal_basis text NOT NULL CHECK (length(legal_basis) > 0),
    required_review boolean NOT NULL,
    review_status text NOT NULL CHECK (review_status IN ('PENDING', 'ACCEPTED', 'CORRECTED')),
    source text NOT NULL CHECK (source IN ('RULE', 'NO_MATCH', 'AMBIGUOUS')),
    rule_version_id text REFERENCES rule_versions(id),
    policy_version text NOT NULL CHECK (length(policy_version) > 0),
    reviewed_by_id text,
    reviewed_by_display text,
    reviewed_at timestamptz,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT line_classifications_invoice_client_fk
        FOREIGN KEY (invoice_id, client_id) REFERENCES invoices(id, client_id),
    CONSTRAINT line_classifications_line_invoice_fk
        FOREIGN KEY (invoice_line_id, invoice_id) REFERENCES invoice_lines(id, invoice_id),
    CONSTRAINT line_classifications_line_dimension_key UNIQUE (invoice_line_id, dimension),
    CONSTRAINT line_classifications_review_shape CHECK (
        (NOT required_review AND review_status = 'ACCEPTED' AND effective_value IS NOT NULL AND reviewed_at IS NULL)
        OR (required_review AND review_status = 'PENDING' AND effective_value IS NULL AND reviewed_at IS NULL)
        OR (required_review AND review_status IN ('ACCEPTED', 'CORRECTED') AND effective_value IS NOT NULL AND reviewed_at IS NOT NULL AND reviewed_by_display IS NOT NULL AND length(btrim(reviewed_by_display)) > 0)
    ),
    CONSTRAINT line_classifications_rule_shape CHECK (
        (source = 'RULE' AND rule_version_id IS NOT NULL)
        OR (source IN ('NO_MATCH', 'AMBIGUOUS') AND rule_version_id IS NULL)
    )
);
CREATE INDEX line_classifications_invoice_review_idx ON line_classifications (invoice_id, review_status);

ALTER TABLE activity_events ALTER COLUMN client_id DROP NOT NULL;
