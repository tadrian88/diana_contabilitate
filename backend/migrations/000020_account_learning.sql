-- Diana-owned account catalogue and deterministic accountant-confirmed mappings.
-- The catalogue is application configuration. It is not legal evidence and no
-- individual analytic account is asserted here to be mandated by OMFP 1802/2014.
CREATE TABLE accounts (
 id text PRIMARY KEY,
 code text NOT NULL UNIQUE CHECK(length(btrim(code))>0),
 name text NOT NULL CHECK(length(btrim(name))>0),
 name_en text,
 account_class integer NOT NULL CHECK(account_class BETWEEN 1 AND 9),
 account_type text NOT NULL CHECK(length(btrim(account_type))>0),
 normal_side text,
 parent_code text,
 level integer NOT NULL CHECK(level>0),
 is_synthetic boolean NOT NULL,
 -- Narrow technical rule supported by the supplied hierarchy: leaf entries
 -- are selectable. Operation-specific accounting suitability remains a human decision.
 postable boolean GENERATED ALWAYS AS (NOT is_synthetic) STORED,
 is_active boolean NOT NULL DEFAULT true,
 description text,
 description_en text,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(parent_code) REFERENCES accounts(code)
);
CREATE INDEX accounts_active_postable_code_idx ON accounts(is_active,postable,code);
CREATE INDEX accounts_name_lower_idx ON accounts(lower(name));

CREATE TABLE account_mappings (
 id text PRIMARY KEY,
 client_id text NOT NULL REFERENCES clients(id),
 normalized_supplier_id text NOT NULL CHECK(length(btrim(normalized_supplier_id))>0),
 service_identity_kind text NOT NULL CHECK(service_identity_kind IN ('SELLER_ITEM_ID','STANDARD_ITEM_ID','NORMALIZED_DESCRIPTION')),
 service_identity_value text NOT NULL CHECK(length(btrim(service_identity_value))>0),
 normalizer_version text NOT NULL CHECK(length(btrim(normalizer_version))>0),
 current_version integer NOT NULL CHECK(current_version>0),
 status text NOT NULL CHECK(status IN ('ACTIVE','INACTIVE')),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 UNIQUE(client_id,normalized_supplier_id,service_identity_kind,service_identity_value,normalizer_version),
 UNIQUE(id,current_version)
);
CREATE INDEX account_mappings_lookup_idx ON account_mappings(client_id,normalized_supplier_id,status);

CREATE TABLE account_mapping_versions (
 id text PRIMARY KEY,
 mapping_id text NOT NULL REFERENCES account_mappings(id),
 version integer NOT NULL CHECK(version>0),
 account_code text NOT NULL REFERENCES accounts(code),
 change_kind text NOT NULL CHECK(change_kind IN ('CREATION','CORRECTION','POLICY_CHANGE')),
 source_classification_id text NOT NULL REFERENCES line_classifications(id),
 source_invoice_line_id text NOT NULL REFERENCES invoice_lines(id),
 raw_description_snapshot text NOT NULL CHECK(length(btrim(raw_description_snapshot))>0),
 actor_id text,
 actor_display text NOT NULL CHECK(length(btrim(actor_display))>0),
 reason text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL,
 command_key text NOT NULL UNIQUE CHECK(length(btrim(command_key))>0),
 UNIQUE(mapping_id,version)
);
ALTER TABLE account_mappings ADD CONSTRAINT account_mappings_current_version_fk
 FOREIGN KEY(id,current_version) REFERENCES account_mapping_versions(mapping_id,version) DEFERRABLE INITIALLY DEFERRED;

CREATE FUNCTION reject_account_mapping_version_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Account mapping versions are immutable'; END; $$;
CREATE TRIGGER account_mapping_version_immutable BEFORE UPDATE OR DELETE ON account_mapping_versions
 FOR EACH ROW EXECUTE FUNCTION reject_account_mapping_version_update();

ALTER TABLE line_classifications ADD COLUMN account_mapping_id text,
 ADD COLUMN account_mapping_version integer;
ALTER TABLE line_classifications DROP CONSTRAINT line_classifications_source_check;
ALTER TABLE line_classifications ADD CONSTRAINT line_classifications_source_check
 CHECK(source IN ('RULE','NO_MATCH','AMBIGUOUS','LEARNED_MAPPING'));
ALTER TABLE line_classifications DROP CONSTRAINT line_classifications_rule_shape;
ALTER TABLE line_classifications ADD CONSTRAINT line_classifications_rule_shape CHECK (
 (source='RULE' AND rule_version_id IS NOT NULL AND account_mapping_id IS NULL)
 OR (source='LEARNED_MAPPING' AND account_mapping_id IS NOT NULL AND account_mapping_version IS NOT NULL)
 OR (source='NO_MATCH' AND rule_version_id IS NULL AND account_mapping_id IS NULL)
 OR (source='AMBIGUOUS')
);
ALTER TABLE line_classifications ADD CONSTRAINT line_classifications_mapping_version_fk
 FOREIGN KEY(account_mapping_id,account_mapping_version)
 REFERENCES account_mapping_versions(mapping_id,version);

-- Diana initial configured chart of accounts (Plan de Conturi).
-- Names/codes are configuration input; this seed does not independently
-- certify the normative status of its analytic subaccounts.
-- Generated at: 2026-02-17T20:23:48.526135

INSERT INTO accounts (
    id, code, name, name_en, account_class, account_type, normal_side,
    parent_code, level, is_synthetic, is_active,
    description, description_en, created_at, updated_at
) VALUES
    ('6edd4c98-98cf-4c58-a3e2-7171c9c06b9f', '1', 'Conturi de capitaluri', 'Capital accounts', 1, 'equity', 'credit', NULL, 1, TRUE, TRUE, NULL, 'Summary account for capital', NOW(), NOW()),
    ('0eb970e5-cb19-46e6-92d7-fb3bcd916620', '10', 'Capital și rezerve', 'Capital and reserves', 1, 'equity', 'credit', '1', 2, TRUE, TRUE, NULL, 'Capital and reserves summary', NOW(), NOW()),
    ('7ec19181-6433-45a7-a361-2f126ae76666', '101', 'Capital social', 'Share capital', 1, 'equity', 'credit', '10', 3, FALSE, TRUE, NULL, 'Subscribed and paid-in capital', NOW(), NOW()),
    ('ba19fd67-b266-48ac-8d9a-97fb142a8bae', '1011', 'Capital subscris nevărsat', 'Subscribed capital unpaid', 1, 'equity', 'credit', '101', 4, FALSE, TRUE, NULL, 'Capital subscribed but not yet paid', NOW(), NOW()),
    ('93d668b0-b473-47fe-b372-cbeee601689e', '1012', 'Capital subscris vărsat', 'Subscribed capital paid', 1, 'equity', 'credit', '101', 4, FALSE, TRUE, NULL, 'Capital subscribed and paid', NOW(), NOW()),
    ('d89d919e-a965-44a7-98b4-ad4b3f39c3f2', '104', 'Prime de capital', 'Share premium', 1, 'equity', 'credit', '10', 3, FALSE, TRUE, NULL, 'Premium received on share issuance', NOW(), NOW()),
    ('b6da9b06-fd6f-4c73-b009-9a4ee518180a', '105', 'Rezerve din reevaluare', 'Revaluation reserves', 1, 'equity', 'credit', '10', 3, FALSE, TRUE, NULL, 'Reserves from asset revaluation', NOW(), NOW()),
    ('4faa6ce0-cc00-4ee7-a48d-56afd425478d', '106', 'Rezerve', 'Reserves', 1, 'equity', 'credit', '10', 3, FALSE, TRUE, NULL, 'Legal and other reserves', NOW(), NOW()),
    ('22bf1a46-033d-48a4-8596-e963d3d4d76d', '1061', 'Rezerve legale', 'Legal reserves', 1, 'equity', 'credit', '106', 4, FALSE, TRUE, NULL, 'Mandatory legal reserves', NOW(), NOW()),
    ('e13b66aa-b3a9-4714-9f2d-e1771433e50c', '1068', 'Alte rezerve', 'Other reserves', 1, 'equity', 'credit', '106', 4, FALSE, TRUE, NULL, 'Other voluntary reserves', NOW(), NOW()),
    ('0cbd9c3c-9c45-4601-8002-7a8a7f74c593', '107', 'Rezultatul reportat', 'Retained earnings', 1, 'equity', 'credit', '10', 3, FALSE, TRUE, NULL, 'Accumulated profits/losses from prior years', NOW(), NOW()),
    ('d53b8b0c-1d08-45cc-97e1-a775561031c3', '1071', 'Rezultatul reportat reprezentând profitul nerepartizat', 'Retained earnings - undistributed profit', 1, 'equity', 'credit', '107', 4, FALSE, TRUE, NULL, 'Undistributed profit from prior years', NOW(), NOW()),
    ('34b38960-5870-420d-a336-66d6c15048f4', '1074', 'Rezultatul reportat reprezentând pierderea neacoperită', 'Retained earnings - uncovered loss', 1, 'equity', 'debit', '107', 4, FALSE, TRUE, NULL, 'Uncovered losses from prior years', NOW(), NOW()),
    ('1ad076d8-a28f-4959-8ae9-143d01240465', '12', 'Rezultatul exercițiului', 'Current year result', 1, 'equity', 'credit', '1', 2, TRUE, TRUE, NULL, 'Profit or loss for current year', NOW(), NOW()),
    ('2cee9032-be68-450b-90fa-73f482e6faf6', '121', 'Profit sau pierdere', 'Profit or loss', 1, 'equity', 'credit', '12', 3, FALSE, TRUE, NULL, 'Net profit or loss for current year', NOW(), NOW()),
    ('3da9b653-5e59-4a4a-9678-71b4a94005c4', '129', 'Repartizarea profitului', 'Profit distribution', 1, 'equity', 'debit', '12', 3, FALSE, TRUE, NULL, 'Distribution of current year profit', NOW(), NOW()),
    ('f4192bba-3a70-441e-896b-53ee467b3300', '15', 'Provizioane', 'Provisions', 1, 'liability', 'credit', '1', 2, TRUE, TRUE, NULL, 'Provisions summary', NOW(), NOW()),
    ('74a4f383-96e5-4eb3-a866-505f4e5efa09', '151', 'Provizioane', 'Provisions', 1, 'liability', 'credit', '15', 3, FALSE, TRUE, NULL, 'Provisions for risks and charges', NOW(), NOW()),
    ('cc49a9cf-a702-4684-a202-9f7e93be627b', '1511', 'Provizioane pentru litigii', 'Provisions for litigation', 1, 'liability', 'credit', '151', 4, FALSE, TRUE, NULL, 'Provisions for pending lawsuits', NOW(), NOW()),
    ('e1916137-d920-468f-b879-f35cb46c0145', '1512', 'Provizioane pentru garanții acordate clienților', 'Provisions for warranties', 1, 'liability', 'credit', '151', 4, FALSE, TRUE, NULL, 'Warranty provisions', NOW(), NOW()),
    ('8c7947ae-a72d-4034-8684-6bc027036ea9', '1518', 'Alte provizioane', 'Other provisions', 1, 'liability', 'credit', '151', 4, FALSE, TRUE, NULL, 'Other provisions', NOW(), NOW()),
    ('37c518ba-bda2-42ae-9be3-b3165859f5ba', '16', 'Împrumuturi și datorii asimilate', 'Loans and similar liabilities', 1, 'liability', 'credit', '1', 2, TRUE, TRUE, NULL, 'Long-term loans summary', NOW(), NOW()),
    ('3f8e31dc-6b5a-4eac-bccc-3f6c22033db5', '161', 'Împrumuturi din emisiuni de obligațiuni', 'Bond loans', 1, 'liability', 'credit', '16', 3, FALSE, TRUE, NULL, 'Loans from bond issuances', NOW(), NOW()),
    ('1ddb2527-f6ae-4bf1-905f-9c388cf909ae', '162', 'Credite bancare pe termen lung', 'Long-term bank loans', 1, 'liability', 'credit', '16', 3, FALSE, TRUE, NULL, 'Bank loans over 1 year', NOW(), NOW()),
    ('240ee0b0-f6ac-4e36-aeea-f1ed96acbba7', '1621', 'Credite bancare pe termen lung', 'Long-term bank credits', 1, 'liability', 'credit', '162', 4, FALSE, TRUE, NULL, 'Long-term bank credits', NOW(), NOW()),
    ('819f6467-e232-450e-8138-eb1d9ecaf937', '1622', 'Credite bancare pe termen lung nerambursate la scadență', 'Overdue long-term bank loans', 1, 'liability', 'credit', '162', 4, FALSE, TRUE, NULL, 'Long-term bank loans not repaid at maturity', NOW(), NOW()),
    ('3ee978c6-eeff-44ea-8b71-0504b897d9d0', '166', 'Datorii către entitățile afiliate', 'Liabilities to affiliates', 1, 'liability', 'credit', '16', 3, FALSE, TRUE, NULL, 'Amounts owed to affiliated entities', NOW(), NOW()),
    ('3745f5e0-c2e4-4a23-9fae-422cfb752065', '167', 'Alte împrumuturi și datorii asimilate', 'Other loans and liabilities', 1, 'liability', 'credit', '16', 3, FALSE, TRUE, NULL, 'Other long-term loans', NOW(), NOW()),
    ('d23f557f-4c38-436a-8319-726de10827e3', '168', 'Dobânzi aferente împrumuturilor', 'Interest on loans', 1, 'liability', 'credit', '16', 3, FALSE, TRUE, NULL, 'Accrued interest on loans', NOW(), NOW()),
    ('c320562d-3636-4c31-a1b1-701ed633f321', '2', 'Conturi de imobilizări', 'Fixed assets accounts', 2, 'asset', 'debit', NULL, 1, TRUE, TRUE, NULL, 'Summary account for fixed assets', NOW(), NOW()),
    ('784b3461-2d8d-41d8-9fed-6b1628977336', '20', 'Imobilizări necorporale', 'Intangible assets', 2, 'asset', 'debit', '2', 2, TRUE, TRUE, NULL, 'Intangible fixed assets', NOW(), NOW()),
    ('44b6a4cb-7157-40e6-8c46-9519b8745f34', '201', 'Cheltuieli de constituire', 'Formation expenses', 2, 'asset', 'debit', '20', 3, FALSE, TRUE, NULL, 'Company formation costs', NOW(), NOW()),
    ('8672f209-ef50-4a41-9f46-9ae47d09b882', '203', 'Cheltuieli de dezvoltare', 'Development costs', 2, 'asset', 'debit', '20', 3, FALSE, TRUE, NULL, 'Capitalized development costs', NOW(), NOW()),
    ('1dbe4f7f-6bbc-41f0-ab47-896585dc3144', '205', 'Concesiuni, brevete, licențe', 'Concessions, patents, licenses', 2, 'asset', 'debit', '20', 3, FALSE, TRUE, NULL, 'Rights and licenses', NOW(), NOW()),
    ('aa43237a-a4ee-461e-abe3-deb3e96f570c', '207', 'Fond comercial', 'Goodwill', 2, 'asset', 'debit', '20', 3, FALSE, TRUE, NULL, 'Purchased goodwill', NOW(), NOW()),
    ('ccbd31d1-3c8e-4bc8-83c2-e389e53af27b', '208', 'Alte imobilizări necorporale', 'Other intangible assets', 2, 'asset', 'debit', '20', 3, FALSE, TRUE, NULL, 'Other intangible fixed assets', NOW(), NOW()),
    ('4f73bb13-ab0d-42a2-b932-451c33eee3aa', '21', 'Imobilizări corporale', 'Tangible assets', 2, 'asset', 'debit', '2', 2, TRUE, TRUE, NULL, 'Tangible fixed assets', NOW(), NOW()),
    ('5aee015b-23c8-45be-bde0-82095365249f', '211', 'Terenuri și amenajări de terenuri', 'Land and land improvements', 2, 'asset', 'debit', '21', 3, FALSE, TRUE, NULL, 'Land and improvements', NOW(), NOW()),
    ('46dcff0c-ec8b-43a0-9010-44de47a00f87', '2111', 'Terenuri', 'Land', 2, 'asset', 'debit', '211', 4, FALSE, TRUE, NULL, 'Land', NOW(), NOW()),
    ('8a46fde5-8c66-41f9-8752-25a80ceca321', '2112', 'Amenajări de terenuri', 'Land improvements', 2, 'asset', 'debit', '211', 4, FALSE, TRUE, NULL, 'Land improvements', NOW(), NOW()),
    ('110dc231-af15-411c-83f5-13d2655a9f62', '212', 'Construcții', 'Buildings', 2, 'asset', 'debit', '21', 3, FALSE, TRUE, NULL, 'Buildings and structures', NOW(), NOW()),
    ('b47edc24-d934-4e78-b9d5-f7451d86d1cf', '213', 'Instalații tehnice și mașini', 'Technical equipment and machinery', 2, 'asset', 'debit', '21', 3, FALSE, TRUE, NULL, 'Technical installations and machines', NOW(), NOW()),
    ('4edd9c85-3444-4322-84f5-74529ba6753e', '214', 'Mobilier, aparatură birotică', 'Furniture and office equipment', 2, 'asset', 'debit', '21', 3, FALSE, TRUE, NULL, 'Furniture and office equipment', NOW(), NOW()),
    ('a9ab9197-cf21-4237-870c-14802164e8aa', '215', 'Investiții imobiliare', 'Investment property', 2, 'asset', 'debit', '21', 3, FALSE, TRUE, NULL, 'Property held for investment', NOW(), NOW()),
    ('fd494911-054e-43a0-b8be-41caa38c1575', '23', 'Imobilizări în curs', 'Assets under construction', 2, 'asset', 'debit', '2', 2, TRUE, TRUE, NULL, 'Fixed assets in progress', NOW(), NOW()),
    ('200f6e9f-bc5d-4f41-8589-018e9ce377f4', '231', 'Imobilizări corporale în curs', 'Tangible assets under construction', 2, 'asset', 'debit', '23', 3, FALSE, TRUE, NULL, 'Tangible assets being constructed', NOW(), NOW()),
    ('0a39df79-5b83-4110-a8b6-1ab67823af92', '232', 'Avansuri acordate pentru imobilizări corporale', 'Advances for tangible assets', 2, 'asset', 'debit', '23', 3, FALSE, TRUE, NULL, 'Advances paid for fixed assets', NOW(), NOW()),
    ('fddb390c-3354-43e1-b784-81a39abed0f3', '26', 'Imobilizări financiare', 'Financial assets', 2, 'asset', 'debit', '2', 2, TRUE, TRUE, NULL, 'Long-term financial assets', NOW(), NOW()),
    ('2d885e74-8329-4fd7-b49b-803295a5494b', '261', 'Acțiuni deținute la entitățile afiliate', 'Shares in affiliates', 2, 'asset', 'debit', '26', 3, FALSE, TRUE, NULL, 'Shares in affiliated companies', NOW(), NOW()),
    ('7fc9010f-b895-4aab-a159-37995f7dd518', '263', 'Interese de participare', 'Participating interests', 2, 'asset', 'debit', '26', 3, FALSE, TRUE, NULL, 'Other equity investments', NOW(), NOW()),
    ('e3adfba1-ad36-4cf0-8bf6-cf1dd2d967b0', '265', 'Alte titluri imobilizate', 'Other fixed securities', 2, 'asset', 'debit', '26', 3, FALSE, TRUE, NULL, 'Other long-term securities', NOW(), NOW()),
    ('e17b0e5a-d6b8-4bbb-9a1a-e96b51398283', '267', 'Creanțe imobilizate', 'Fixed receivables', 2, 'asset', 'debit', '26', 3, FALSE, TRUE, NULL, 'Long-term receivables', NOW(), NOW()),
    ('a8a1d185-3a12-4b8f-b50f-c226dcd907dc', '28', 'Amortizări privind imobilizările', 'Depreciation of fixed assets', 2, 'asset', 'credit', '2', 2, TRUE, TRUE, NULL, 'Accumulated depreciation', NOW(), NOW()),
    ('8b6e159a-3bf1-4bcb-9f5f-4b7c2bab1524', '280', 'Amortizări privind imobilizările necorporale', 'Depreciation of intangible assets', 2, 'asset', 'credit', '28', 3, FALSE, TRUE, NULL, 'Accumulated amortization - intangibles', NOW(), NOW()),
    ('d2eaaf00-e589-48bb-addb-4e9d6fb9ef9c', '281', 'Amortizări privind imobilizările corporale', 'Depreciation of tangible assets', 2, 'asset', 'credit', '28', 3, FALSE, TRUE, NULL, 'Accumulated depreciation - tangibles', NOW(), NOW()),
    ('4173883f-e672-451e-8c18-6b22c12272c3', '2811', 'Amortizarea amenajărilor de terenuri', 'Depreciation of land improvements', 2, 'asset', 'credit', '281', 4, FALSE, TRUE, NULL, 'Depreciation of land improvements', NOW(), NOW()),
    ('705722f2-504a-4d26-8081-0567c41c4a28', '2812', 'Amortizarea construcțiilor', 'Depreciation of buildings', 2, 'asset', 'credit', '281', 4, FALSE, TRUE, NULL, 'Depreciation of buildings', NOW(), NOW()),
    ('69d665a1-e8a9-46ca-adb7-a6aebe6682b2', '2813', 'Amortizarea instalațiilor și mașinilor', 'Depreciation of equipment', 2, 'asset', 'credit', '281', 4, FALSE, TRUE, NULL, 'Depreciation of technical equipment', NOW(), NOW()),
    ('7f0734b7-d344-4af6-8107-0c7a85dca508', '2814', 'Amortizarea altor imobilizări corporale', 'Depreciation of other tangibles', 2, 'asset', 'credit', '281', 4, FALSE, TRUE, NULL, 'Depreciation of other tangible assets', NOW(), NOW()),
    ('a3b8acfd-ca48-4ee7-aceb-86788c973f52', '3', 'Conturi de stocuri și producție în curs', 'Inventory accounts', 3, 'asset', 'debit', NULL, 1, TRUE, TRUE, NULL, 'Summary account for inventory', NOW(), NOW()),
    ('debd8aa3-6bf3-4f2f-a2e1-6084fc6f7d9b', '30', 'Stocuri de materii prime și materiale', 'Raw materials and supplies', 3, 'asset', 'debit', '3', 2, TRUE, TRUE, NULL, 'Raw materials inventory', NOW(), NOW()),
    ('47db28a9-1c2f-421b-80f5-e5b49d1a9496', '301', 'Materii prime', 'Raw materials', 3, 'asset', 'debit', '30', 3, FALSE, TRUE, NULL, 'Raw materials inventory', NOW(), NOW()),
    ('c77f735c-4e1c-4cb3-bc66-6e11793e88d3', '302', 'Materiale consumabile', 'Consumable materials', 3, 'asset', 'debit', '30', 3, FALSE, TRUE, NULL, 'Consumable supplies', NOW(), NOW()),
    ('6bf72193-ea8f-47c7-ae4c-a22ab2a0c9ce', '303', 'Materiale de natura obiectelor de inventar', 'Low-value items', 3, 'asset', 'debit', '30', 3, FALSE, TRUE, NULL, 'Small tools and equipment', NOW(), NOW()),
    ('da1a6741-645b-492c-87c6-fb370a1b948f', '308', 'Diferențe de preț la materii prime și materiale', 'Price differences - materials', 3, 'asset', 'debit', '30', 3, FALSE, TRUE, NULL, 'Price variances on materials', NOW(), NOW()),
    ('3840db8d-d0c1-40c5-8d6a-47a1bb8cb0b4', '33', 'Producție în curs de execuție', 'Work in progress', 3, 'asset', 'debit', '3', 2, TRUE, TRUE, NULL, 'Work in progress inventory', NOW(), NOW()),
    ('5a57d3cf-fbdd-464a-84c0-65cc374ac68b', '331', 'Produse în curs de execuție', 'Products in progress', 3, 'asset', 'debit', '33', 3, FALSE, TRUE, NULL, 'Products being manufactured', NOW(), NOW()),
    ('a8ff2030-e5bc-4058-afc2-723c0622f259', '332', 'Servicii în curs de execuție', 'Services in progress', 3, 'asset', 'debit', '33', 3, FALSE, TRUE, NULL, 'Services being rendered', NOW(), NOW()),
    ('55e9ebbc-4e07-4b18-9727-43ac38e1dfe8', '34', 'Produse', 'Finished products', 3, 'asset', 'debit', '3', 2, TRUE, TRUE, NULL, 'Finished goods inventory', NOW(), NOW()),
    ('94c8319a-0d98-4a6e-a424-daefbfd8b722', '341', 'Semifabricate', 'Semi-finished products', 3, 'asset', 'debit', '34', 3, FALSE, TRUE, NULL, 'Semi-finished goods', NOW(), NOW()),
    ('a2a116cd-ec32-40db-b58c-eaee68c7ffe9', '345', 'Produse finite', 'Finished products', 3, 'asset', 'debit', '34', 3, FALSE, TRUE, NULL, 'Completed goods ready for sale', NOW(), NOW()),
    ('1983d7a2-48d0-439c-b891-7ed747cfb809', '346', 'Produse reziduale', 'By-products', 3, 'asset', 'debit', '34', 3, FALSE, TRUE, NULL, 'By-products and residuals', NOW(), NOW()),
    ('e438cc91-39db-449b-84a4-2d839781ca49', '35', 'Stocuri aflate la terți', 'Inventory held by third parties', 3, 'asset', 'debit', '3', 2, TRUE, TRUE, NULL, 'Inventory at third party locations', NOW(), NOW()),
    ('0ab02fe3-f697-4a11-88ef-0f592e564597', '351', 'Materii și materiale aflate la terți', 'Materials at third parties', 3, 'asset', 'debit', '35', 3, FALSE, TRUE, NULL, 'Raw materials held by others', NOW(), NOW()),
    ('9357a7ae-8b81-4f41-8fad-dc0e1f276cfc', '354', 'Produse aflate la terți', 'Products at third parties', 3, 'asset', 'debit', '35', 3, FALSE, TRUE, NULL, 'Products held by others', NOW(), NOW()),
    ('4e572800-5e91-49a1-8f50-a49b3c2794e8', '356', 'Animale aflate la terți', 'Animals at third parties', 3, 'asset', 'debit', '35', 3, FALSE, TRUE, NULL, 'Livestock held by others', NOW(), NOW()),
    ('88f7bd53-122e-46a3-974f-e96d17ce72a7', '357', 'Mărfuri aflate la terți', 'Goods at third parties', 3, 'asset', 'debit', '35', 3, FALSE, TRUE, NULL, 'Merchandise held by others', NOW(), NOW()),
    ('08175b9f-598e-43b3-b555-1d9b660fc469', '358', 'Ambalaje aflate la terți', 'Packaging at third parties', 3, 'asset', 'debit', '35', 3, FALSE, TRUE, NULL, 'Packaging held by others', NOW(), NOW()),
    ('acf7432c-7e4b-44a5-a886-02e6b3f77e9c', '37', 'Mărfuri', 'Merchandise', 3, 'asset', 'debit', '3', 2, TRUE, TRUE, NULL, 'Merchandise inventory', NOW(), NOW()),
    ('9132e667-c499-4b85-bbce-e1ef22596b5c', '371', 'Mărfuri', 'Merchandise', 3, 'asset', 'debit', '37', 3, FALSE, TRUE, NULL, 'Goods purchased for resale', NOW(), NOW()),
    ('b78e3ffd-5bd3-4265-8cf4-bf066e796f99', '378', 'Diferențe de preț la mărfuri', 'Price differences - merchandise', 3, 'asset', 'debit', '37', 3, FALSE, TRUE, NULL, 'Price variances on merchandise', NOW(), NOW()),
    ('e64455d2-2c74-40d7-82e6-04c6f251883e', '38', 'Ambalaje', 'Packaging', 3, 'asset', 'debit', '3', 2, TRUE, TRUE, NULL, 'Packaging inventory', NOW(), NOW()),
    ('4555b036-ea39-4c13-9535-539c5204506a', '381', 'Ambalaje', 'Packaging', 3, 'asset', 'debit', '38', 3, FALSE, TRUE, NULL, 'Packaging materials', NOW(), NOW()),
    ('5319b3d6-3371-4bc3-8c53-887e29dd8539', '388', 'Diferențe de preț la ambalaje', 'Price differences - packaging', 3, 'asset', 'debit', '38', 3, FALSE, TRUE, NULL, 'Price variances on packaging', NOW(), NOW()),
    ('a073cf68-b820-44a9-a557-1a4691fd5348', '39', 'Ajustări pentru deprecierea stocurilor', 'Inventory provisions', 3, 'asset', 'credit', '3', 2, TRUE, TRUE, NULL, 'Provisions for inventory obsolescence', NOW(), NOW()),
    ('3a7b2d3a-4792-4f7f-b86f-4fa7a601e26c', '391', 'Ajustări pentru deprecierea materiilor prime', 'Provisions - raw materials', 3, 'asset', 'credit', '39', 3, FALSE, TRUE, NULL, 'Provisions for raw materials', NOW(), NOW()),
    ('d53fe4e1-3f22-48eb-af06-65fd69da40ca', '392', 'Ajustări pentru deprecierea materialelor', 'Provisions - materials', 3, 'asset', 'credit', '39', 3, FALSE, TRUE, NULL, 'Provisions for materials', NOW(), NOW()),
    ('593fd8f2-58a3-4379-8c22-4909d4813dbf', '394', 'Ajustări pentru deprecierea produselor', 'Provisions - products', 3, 'asset', 'credit', '39', 3, FALSE, TRUE, NULL, 'Provisions for finished goods', NOW(), NOW()),
    ('628a444e-39f0-4b13-8032-476ae16c1308', '397', 'Ajustări pentru deprecierea mărfurilor', 'Provisions - merchandise', 3, 'asset', 'credit', '39', 3, FALSE, TRUE, NULL, 'Provisions for merchandise', NOW(), NOW()),
    ('dbc1ce32-2469-4e5e-8c8e-89470552e29a', '4', 'Conturi de terți', 'Third party accounts', 4, 'asset', 'debit', NULL, 1, TRUE, TRUE, NULL, 'Summary account for receivables/payables', NOW(), NOW()),
    ('043ef32d-8738-4ebc-8aa9-df2cc11cd509', '40', 'Furnizori și conturi asimilate', 'Suppliers and similar', 4, 'liability', 'credit', '4', 2, TRUE, TRUE, NULL, 'Trade payables', NOW(), NOW()),
    ('bb631ea7-14ea-4114-8f7e-c08c5882a195', '401', 'Furnizori', 'Suppliers', 4, 'liability', 'credit', '40', 3, FALSE, TRUE, NULL, 'Trade accounts payable', NOW(), NOW()),
    ('c2860227-252f-4d08-b366-7088526d1799', '403', 'Efecte de plătit', 'Notes payable', 4, 'liability', 'credit', '40', 3, FALSE, TRUE, NULL, 'Bills of exchange payable', NOW(), NOW()),
    ('119b975e-2f1d-4287-b2c2-6af7b5a4bc01', '404', 'Furnizori de imobilizări', 'Suppliers of fixed assets', 4, 'liability', 'credit', '40', 3, FALSE, TRUE, NULL, 'Payables for fixed assets', NOW(), NOW()),
    ('82089076-449e-41b7-8a92-714822aba3dd', '405', 'Efecte de plătit pentru imobilizări', 'Notes payable - fixed assets', 4, 'liability', 'credit', '40', 3, FALSE, TRUE, NULL, 'Bills payable for fixed assets', NOW(), NOW()),
    ('fde8dee9-fa5f-40ce-9e42-ed2a34d50ab4', '408', 'Furnizori - facturi nesosite', 'Suppliers - invoices not received', 4, 'liability', 'credit', '40', 3, FALSE, TRUE, NULL, 'Accrued expenses - goods received', NOW(), NOW()),
    ('e7ea784c-cb8f-4811-9d36-03f24b05c3cd', '409', 'Furnizori - debitori', 'Suppliers - debit balances', 4, 'asset', 'debit', '40', 3, FALSE, TRUE, NULL, 'Advances to suppliers', NOW(), NOW()),
    ('2d1a59f7-467f-418f-9f8e-43dc8a23febd', '41', 'Clienți și conturi asimilate', 'Customers and similar', 4, 'asset', 'debit', '4', 2, TRUE, TRUE, NULL, 'Trade receivables', NOW(), NOW()),
    ('f2620301-684a-4c9a-95f3-34ed2c592f0f', '411', 'Clienți', 'Customers', 4, 'asset', 'debit', '41', 3, FALSE, TRUE, NULL, 'Trade accounts receivable', NOW(), NOW()),
    ('17e7ee91-5bd5-4215-a4b3-53c640f9755f', '4111', 'Clienți', 'Customers', 4, 'asset', 'debit', '411', 4, FALSE, TRUE, NULL, 'Customer receivables', NOW(), NOW()),
    ('51e56531-f617-4f0e-aa50-11c3f3383f61', '4118', 'Clienți incerți sau în litigiu', 'Doubtful customers', 4, 'asset', 'debit', '411', 4, FALSE, TRUE, NULL, 'Doubtful accounts receivable', NOW(), NOW()),
    ('1bed27d8-13c8-4e4b-b007-56dd96526023', '413', 'Efecte de primit', 'Notes receivable', 4, 'asset', 'debit', '41', 3, FALSE, TRUE, NULL, 'Bills of exchange receivable', NOW(), NOW()),
    ('00fb11bd-0933-46c1-a1da-9580359f616f', '418', 'Clienți - facturi de întocmit', 'Customers - invoices to issue', 4, 'asset', 'debit', '41', 3, FALSE, TRUE, NULL, 'Accrued revenue - services rendered', NOW(), NOW()),
    ('61fcd0db-828d-4e82-83a4-dee51829d5c5', '419', 'Clienți - creditori', 'Customers - credit balances', 4, 'liability', 'credit', '41', 3, FALSE, TRUE, NULL, 'Advances from customers', NOW(), NOW()),
    ('7a43b6fb-a178-4981-9ebf-6ce7c3845c8c', '42', 'Personal și conturi asimilate', 'Personnel accounts', 4, 'liability', 'credit', '4', 2, TRUE, TRUE, NULL, 'Payroll accounts', NOW(), NOW()),
    ('7960010b-311d-45ef-ab97-1f18f8311713', '421', 'Personal - salarii datorate', 'Salaries payable', 4, 'liability', 'credit', '42', 3, FALSE, TRUE, NULL, 'Salaries owed to employees', NOW(), NOW()),
    ('663bdcf8-883e-48a6-a217-167775b2385e', '423', 'Personal - ajutoare materiale datorate', 'Benefits payable', 4, 'liability', 'credit', '42', 3, FALSE, TRUE, NULL, 'Other employee benefits owed', NOW(), NOW()),
    ('00fb0610-779c-44ec-8bbb-e98733151989', '424', 'Prime reprezentând participarea la profit', 'Profit sharing', 4, 'liability', 'credit', '42', 3, FALSE, TRUE, NULL, 'Employee profit sharing', NOW(), NOW()),
    ('c8e35027-f46b-4af2-9904-3f08dd21f93e', '425', 'Avansuri acordate personalului', 'Advances to personnel', 4, 'asset', 'debit', '42', 3, FALSE, TRUE, NULL, 'Salary advances', NOW(), NOW()),
    ('03e3498a-831d-4b3f-abc6-7b253bc3edcd', '426', 'Drepturi de personal neridicate', 'Unclaimed wages', 4, 'liability', 'credit', '42', 3, FALSE, TRUE, NULL, 'Salaries not collected', NOW(), NOW()),
    ('93c4edac-8f11-4cde-8b51-b56007caecfd', '427', 'Rețineri din salarii datorate terților', 'Salary deductions payable', 4, 'liability', 'credit', '42', 3, FALSE, TRUE, NULL, 'Withheld amounts payable', NOW(), NOW()),
    ('1cf63d46-58b8-4a6a-991e-017cd2539727', '428', 'Alte datorii și creanțe în legătură cu personalul', 'Other personnel accounts', 4, 'liability', 'credit', '42', 3, FALSE, TRUE, NULL, 'Other personnel-related accounts', NOW(), NOW()),
    ('6b6cab03-6392-4918-bae2-08bb4e25e5cc', '43', 'Asigurări sociale, protecție socială', 'Social security', 4, 'liability', 'credit', '4', 2, TRUE, TRUE, NULL, 'Social security contributions', NOW(), NOW()),
    ('fa9579a3-af80-40cb-b5e5-cbc0231f125d', '431', 'Asigurări sociale', 'Social security', 4, 'liability', 'credit', '43', 3, FALSE, TRUE, NULL, 'Social security contributions payable', NOW(), NOW()),
    ('634eb87f-55da-4c97-9cfd-615625b01b8d', '4311', 'Contribuția la asigurările sociale', 'Social security contribution', 4, 'liability', 'credit', '431', 4, FALSE, TRUE, NULL, 'Pension contributions', NOW(), NOW()),
    ('fcbf50b8-0b20-493b-99fb-47824cd14e19', '4312', 'Contribuția angajatorului pentru asigurări sociale de sănătate', 'Health insurance - employer', 4, 'liability', 'credit', '431', 4, FALSE, TRUE, NULL, 'Employer health contributions', NOW(), NOW()),
    ('a00550fe-be8d-4ccc-b91c-69caeeae1824', '4313', 'Contribuția angajatului pentru asigurări sociale de sănătate', 'Health insurance - employee', 4, 'liability', 'credit', '431', 4, FALSE, TRUE, NULL, 'Employee health contributions', NOW(), NOW()),
    ('7834d171-3806-4555-a2c2-10d268f29125', '437', 'Ajutor de șomaj', 'Unemployment insurance', 4, 'liability', 'credit', '43', 3, FALSE, TRUE, NULL, 'Unemployment contributions', NOW(), NOW()),
    ('a71aa08d-dab8-4365-b799-0568956125a6', '438', 'Alte datorii și creanțe sociale', 'Other social contributions', 4, 'liability', 'credit', '43', 3, FALSE, TRUE, NULL, 'Other social security accounts', NOW(), NOW()),
    ('82e45761-e4ca-4db8-8f16-5bf2d6b46d3e', '44', 'Bugetul statului, fonduri speciale', 'State budget accounts', 4, 'liability', 'credit', '4', 2, TRUE, TRUE, NULL, 'Tax accounts', NOW(), NOW()),
    ('ad038e87-c831-40e4-83fa-27c9d0ee391f', '441', 'Impozitul pe profit', 'Corporate income tax', 4, 'liability', 'credit', '44', 3, FALSE, TRUE, NULL, 'Income tax payable', NOW(), NOW()),
    ('386c5577-82ce-4856-afa4-099840d72fcc', '4411', 'Impozitul pe profit curent', 'Current income tax', 4, 'liability', 'credit', '441', 4, FALSE, TRUE, NULL, 'Current period income tax', NOW(), NOW()),
    ('1ccecd23-700e-43c5-8989-c1f7a927b372', '4412', 'Impozitul pe profit amânat', 'Deferred income tax', 4, 'liability', 'credit', '441', 4, FALSE, TRUE, NULL, 'Deferred tax liability', NOW(), NOW()),
    ('cced859e-2fbf-4375-9975-c94313024b96', '442', 'Taxa pe valoarea adăugată', 'Value added tax', 4, 'liability', 'credit', '44', 3, TRUE, TRUE, NULL, 'VAT accounts', NOW(), NOW()),
    ('fcc0107d-794d-434b-aedc-b59a7b03f435', '4423', 'TVA de plată', 'VAT payable', 4, 'liability', 'credit', '442', 4, FALSE, TRUE, NULL, 'Net VAT payable to state', NOW(), NOW()),
    ('a3da9ec3-4d69-41e1-95d7-d74d4887efe1', '4424', 'TVA de recuperat', 'VAT receivable', 4, 'asset', 'debit', '442', 4, FALSE, TRUE, NULL, 'Net VAT receivable from state', NOW(), NOW()),
    ('58ad5662-bd50-4f6d-811a-70a471597a8c', '4426', 'TVA deductibilă', 'Input VAT', 4, 'asset', 'debit', '442', 4, FALSE, TRUE, NULL, 'VAT on purchases (deductible)', NOW(), NOW()),
    ('e1a792b9-0ec3-4170-885a-c7190bf938c2', '4427', 'TVA colectată', 'Output VAT', 4, 'liability', 'credit', '442', 4, FALSE, TRUE, NULL, 'VAT on sales (collected)', NOW(), NOW()),
    ('8cd57d20-d35e-456b-bb1b-5a1a3408728f', '4428', 'TVA neexigibilă', 'VAT not yet due', 4, 'liability', 'credit', '442', 4, FALSE, TRUE, NULL, 'VAT not yet payable/receivable', NOW(), NOW()),
    ('16058cbb-7413-44b2-89f3-2e778f72902f', '444', 'Impozitul pe venituri de natura salariilor', 'Payroll tax', 4, 'liability', 'credit', '44', 3, FALSE, TRUE, NULL, 'Withholding tax on salaries', NOW(), NOW()),
    ('9ef68fe8-d1d7-46b5-8aec-e516548941a3', '445', 'Subvenții', 'Subsidies', 4, 'asset', 'debit', '44', 3, FALSE, TRUE, NULL, 'Government subsidies receivable', NOW(), NOW()),
    ('6483faca-94f1-4b32-885f-d9b01dad059d', '446', 'Alte impozite, taxe și vărsăminte asimilate', 'Other taxes', 4, 'liability', 'credit', '44', 3, FALSE, TRUE, NULL, 'Other taxes and duties', NOW(), NOW()),
    ('35830364-c805-4060-bfe0-ab7621263d7a', '447', 'Fonduri speciale - taxe și vărsăminte asimilate', 'Special funds', 4, 'liability', 'credit', '44', 3, FALSE, TRUE, NULL, 'Special fund contributions', NOW(), NOW()),
    ('34a66a9c-b584-4c06-9cd2-41cc75ae0a6a', '448', 'Alte datorii și creanțe cu bugetul statului', 'Other state budget', 4, 'liability', 'credit', '44', 3, FALSE, TRUE, NULL, 'Other amounts due to/from state', NOW(), NOW()),
    ('e6248371-afc4-454d-8c94-841f0bee55c6', '45', 'Grup și acționari/asociați', 'Group and shareholders', 4, 'asset', 'debit', '4', 2, TRUE, TRUE, NULL, 'Intercompany and shareholder accounts', NOW(), NOW()),
    ('64c6ce99-a458-4417-9bc7-0535a8d5b936', '451', 'Decontări între entitățile afiliate', 'Intercompany accounts', 4, 'asset', 'debit', '45', 3, FALSE, TRUE, NULL, 'Amounts with affiliated entities', NOW(), NOW()),
    ('0d57ad78-bfee-4c7a-a6ce-8db7f327e9f9', '453', 'Decontări privind interesele de participare', 'Participation interests', 4, 'asset', 'debit', '45', 3, FALSE, TRUE, NULL, 'Amounts with equity investments', NOW(), NOW()),
    ('8b907cb5-0e6f-482b-9134-d3b522facc23', '455', 'Sume datorate acționarilor/asociaților', 'Amounts due to shareholders', 4, 'liability', 'credit', '45', 3, FALSE, TRUE, NULL, 'Dividends and other payables to owners', NOW(), NOW()),
    ('b971e0c7-5813-452a-85e4-b6d1033a3423', '456', 'Decontări cu acționarii/asociații', 'Shareholder accounts', 4, 'asset', 'debit', '45', 3, FALSE, TRUE, NULL, 'Current accounts with shareholders', NOW(), NOW()),
    ('b8e717c3-5819-420a-a426-cd77bc1854b3', '457', 'Dividende de plată', 'Dividends payable', 4, 'liability', 'credit', '45', 3, FALSE, TRUE, NULL, 'Declared dividends not yet paid', NOW(), NOW()),
    ('34e24797-987a-4ec8-b8a1-794824f40785', '46', 'Debitori și creditori diverși', 'Sundry debtors and creditors', 4, 'asset', 'debit', '4', 2, TRUE, TRUE, NULL, 'Other receivables/payables', NOW(), NOW()),
    ('e688b7ae-9c32-4c64-94f5-5ef4ab83d1ee', '461', 'Debitori diverși', 'Sundry debtors', 4, 'asset', 'debit', '46', 3, FALSE, TRUE, NULL, 'Miscellaneous receivables', NOW(), NOW()),
    ('457fb7e5-cb34-4bc5-af1a-bac961480cca', '462', 'Creditori diverși', 'Sundry creditors', 4, 'liability', 'credit', '46', 3, FALSE, TRUE, NULL, 'Miscellaneous payables', NOW(), NOW()),
    ('63430277-3748-4492-a589-9f9254abf084', '47', 'Conturi de regularizare și asimilate', 'Accruals and deferrals', 4, 'asset', 'debit', '4', 2, TRUE, TRUE, NULL, 'Prepayments and accruals', NOW(), NOW()),
    ('6dfa9206-af04-43b7-9f0e-d75f15ce0431', '471', 'Cheltuieli înregistrate în avans', 'Prepaid expenses', 4, 'asset', 'debit', '47', 3, FALSE, TRUE, NULL, 'Expenses paid in advance', NOW(), NOW()),
    ('9a5173c2-bfa4-4f2a-a04d-cdbfbe1a8b80', '472', 'Venituri înregistrate în avans', 'Deferred revenue', 4, 'liability', 'credit', '47', 3, FALSE, TRUE, NULL, 'Revenue received in advance', NOW(), NOW()),
    ('4f5fdc67-9436-411e-87aa-6e7f8bfd0010', '473', 'Decontări din operații în curs de clarificare', 'Suspense accounts', 4, 'asset', 'debit', '47', 3, FALSE, TRUE, NULL, 'Pending transactions', NOW(), NOW()),
    ('4142c454-c4f3-4f22-9016-2966eafce115', '48', 'Decontări în cadrul unității', 'Intra-entity accounts', 4, 'asset', 'debit', '4', 2, TRUE, TRUE, NULL, 'Head office and branch accounts', NOW(), NOW()),
    ('1871237a-968a-4305-b768-335a22c5e65b', '481', 'Decontări între unitate și subunități', 'Branch accounts', 4, 'asset', 'debit', '48', 3, FALSE, TRUE, NULL, 'Transactions with branches', NOW(), NOW()),
    ('1e85f125-e124-4f6f-b760-c3285a22fc71', '482', 'Decontări între subunități', 'Inter-branch accounts', 4, 'asset', 'debit', '48', 3, FALSE, TRUE, NULL, 'Transactions between branches', NOW(), NOW()),
    ('eadf53b4-863a-46a0-91b7-5d2b2d14364e', '49', 'Ajustări pentru deprecierea creanțelor', 'Provisions for receivables', 4, 'asset', 'credit', '4', 2, TRUE, TRUE, NULL, 'Bad debt provisions', NOW(), NOW()),
    ('962ea672-ea9e-4d23-9fd8-071470e1b801', '491', 'Ajustări pentru deprecierea creanțelor - clienți', 'Provisions - customers', 4, 'asset', 'credit', '49', 3, FALSE, TRUE, NULL, 'Provisions for doubtful customers', NOW(), NOW()),
    ('a651ee89-d27b-488e-a1f5-647149079aab', '495', 'Ajustări pentru deprecierea creanțelor - grup', 'Provisions - group', 4, 'asset', 'credit', '49', 3, FALSE, TRUE, NULL, 'Provisions for group receivables', NOW(), NOW()),
    ('9c26a0de-680b-4b9b-87cf-99ee561e5d2b', '496', 'Ajustări pentru deprecierea creanțelor - debitori diverși', 'Provisions - sundry debtors', 4, 'asset', 'credit', '49', 3, FALSE, TRUE, NULL, 'Provisions for other receivables', NOW(), NOW()),
    ('25b66631-4344-4c9e-9f6c-c2e8639d346d', '5', 'Conturi de trezorerie', 'Cash accounts', 5, 'asset', 'debit', NULL, 1, TRUE, TRUE, NULL, 'Summary account for cash and bank', NOW(), NOW()),
    ('bbdf2527-f5f2-4047-bbcf-65a05612d332', '50', 'Investiții pe termen scurt', 'Short-term investments', 5, 'asset', 'debit', '5', 2, TRUE, TRUE, NULL, 'Short-term financial investments', NOW(), NOW()),
    ('06cc132a-d03d-4e5e-a7bf-69752864e240', '501', 'Acțiuni deținute la entitățile afiliate', 'Shares in affiliates', 5, 'asset', 'debit', '50', 3, FALSE, TRUE, NULL, 'Short-term shares in affiliates', NOW(), NOW()),
    ('d9d89413-c17c-44b9-9c74-e8594d427ec2', '505', 'Obligațiuni emise și răscumpărate', 'Own bonds repurchased', 5, 'asset', 'debit', '50', 3, FALSE, TRUE, NULL, 'Own bonds bought back', NOW(), NOW()),
    ('7ca48583-2a8d-4467-ae26-3c559a33c447', '506', 'Obligațiuni', 'Bonds', 5, 'asset', 'debit', '50', 3, FALSE, TRUE, NULL, 'Bond investments', NOW(), NOW()),
    ('9ad113cc-8839-40a8-8913-54b010d27360', '508', 'Alte investiții pe termen scurt', 'Other short-term investments', 5, 'asset', 'debit', '50', 3, FALSE, TRUE, NULL, 'Other temporary investments', NOW(), NOW()),
    ('2e299d58-1472-4d49-90cb-3e4369381a95', '509', 'Vărsăminte de efectuat pentru investiții', 'Amounts due for investments', 5, 'liability', 'credit', '50', 3, FALSE, TRUE, NULL, 'Amounts owed for investments', NOW(), NOW()),
    ('674dda18-a41b-49de-b65c-11dc14ce5553', '51', 'Conturi la bănci', 'Bank accounts', 5, 'asset', 'debit', '5', 2, TRUE, TRUE, NULL, 'Bank accounts', NOW(), NOW()),
    ('b7d189bf-63d4-45e8-ae2a-eecfa073deb2', '511', 'Valori de încasat', 'Items for collection', 5, 'asset', 'debit', '51', 3, FALSE, TRUE, NULL, 'Checks and items for collection', NOW(), NOW()),
    ('c974b1be-4f23-49cd-9ac3-eb22a1a6b980', '512', 'Conturi curente la bănci', 'Current bank accounts', 5, 'asset', 'debit', '51', 3, FALSE, TRUE, NULL, 'Current accounts at banks', NOW(), NOW()),
    ('0332725a-501b-4d2a-8e7f-b3dc9643d90f', '5121', 'Conturi la bănci în lei', 'Bank accounts in RON', 5, 'asset', 'debit', '512', 4, FALSE, TRUE, NULL, 'Bank accounts in local currency', NOW(), NOW()),
    ('b05ae7b2-997a-45be-9e5b-36190a79b03e', '5124', 'Conturi la bănci în valută', 'Bank accounts in foreign currency', 5, 'asset', 'debit', '512', 4, FALSE, TRUE, NULL, 'Bank accounts in foreign currency', NOW(), NOW()),
    ('f6ce0556-78cb-4758-b18c-7ac8c86a8008', '518', 'Dobânzi', 'Interest', 5, 'asset', 'debit', '51', 3, FALSE, TRUE, NULL, 'Interest receivable/payable', NOW(), NOW()),
    ('296da131-11b0-46b3-b364-5192312a60c3', '5186', 'Dobânzi de plătit', 'Interest payable', 5, 'liability', 'credit', '518', 4, FALSE, TRUE, NULL, 'Bank interest payable', NOW(), NOW()),
    ('c9a91042-4161-4dfa-b2bc-32b703285123', '5187', 'Dobânzi de încasat', 'Interest receivable', 5, 'asset', 'debit', '518', 4, FALSE, TRUE, NULL, 'Bank interest receivable', NOW(), NOW()),
    ('82c14003-7ef2-4176-aba8-9a52a4fdbb3d', '519', 'Credite bancare pe termen scurt', 'Short-term bank loans', 5, 'liability', 'credit', '51', 3, FALSE, TRUE, NULL, 'Bank overdrafts and short-term loans', NOW(), NOW()),
    ('664885d5-4ca9-41c6-b50b-5ecc70b699e3', '5191', 'Credite bancare pe termen scurt', 'Short-term bank credits', 5, 'liability', 'credit', '519', 4, FALSE, TRUE, NULL, 'Short-term bank credits', NOW(), NOW()),
    ('b997e174-9e98-44f3-819e-28f515f9946e', '5192', 'Credite bancare pe termen scurt nerambursate la scadență', 'Overdue short-term bank loans', 5, 'liability', 'credit', '519', 4, FALSE, TRUE, NULL, 'Short-term loans not repaid at maturity', NOW(), NOW()),
    ('b25d0264-c8e6-44fc-9369-d1c1d15b179c', '53', 'Casa', 'Cash on hand', 5, 'asset', 'debit', '5', 2, TRUE, TRUE, NULL, 'Petty cash', NOW(), NOW()),
    ('9a48abe4-96c5-476b-814e-0eadaed73d11', '531', 'Casa', 'Cash', 5, 'asset', 'debit', '53', 3, FALSE, TRUE, NULL, 'Cash on hand', NOW(), NOW()),
    ('964e8dbe-8f81-4e82-98ed-f02a99116157', '5311', 'Casa în lei', 'Cash in RON', 5, 'asset', 'debit', '531', 4, FALSE, TRUE, NULL, 'Cash in local currency', NOW(), NOW()),
    ('aa191326-30c6-4284-a74c-2f6148f5cadf', '5314', 'Casa în valută', 'Cash in foreign currency', 5, 'asset', 'debit', '531', 4, FALSE, TRUE, NULL, 'Cash in foreign currency', NOW(), NOW()),
    ('019437e5-8411-40ac-ac0e-10dec06e4fbb', '532', 'Alte valori', 'Other cash items', 5, 'asset', 'debit', '53', 3, FALSE, TRUE, NULL, 'Stamps, vouchers, etc.', NOW(), NOW()),
    ('c26a8a09-9cc9-4bad-8bc3-ae2cf5f3e00b', '54', 'Acreditive', 'Letters of credit', 5, 'asset', 'debit', '5', 2, TRUE, TRUE, NULL, 'Letters of credit', NOW(), NOW()),
    ('ff05e70c-72be-41f3-b3f3-7f5acf92d6ef', '541', 'Acreditive', 'Letters of credit', 5, 'asset', 'debit', '54', 3, FALSE, TRUE, NULL, 'Documentary credits', NOW(), NOW()),
    ('82c182d0-cd1a-40ff-960a-88e5860ebd17', '542', 'Avansuri de trezorerie', 'Cash advances', 5, 'asset', 'debit', '54', 3, FALSE, TRUE, NULL, 'Petty cash advances', NOW(), NOW()),
    ('53a8b693-2bc2-4c5e-ad32-e504bb22135b', '58', 'Viramente interne', 'Internal transfers', 5, 'asset', 'debit', '5', 2, TRUE, TRUE, NULL, 'Transfers between cash accounts', NOW(), NOW()),
    ('e2bd4b5b-ec93-483d-9f80-df9dcfdd8758', '581', 'Viramente interne', 'Internal transfers', 5, 'asset', 'debit', '58', 3, FALSE, TRUE, NULL, 'Cash transfers in transit', NOW(), NOW()),
    ('4f3cae68-c902-427f-beaf-2a172643e282', '59', 'Ajustări pentru pierderea de valoare', 'Provisions for investments', 5, 'asset', 'credit', '5', 2, TRUE, TRUE, NULL, 'Provisions for investment impairment', NOW(), NOW()),
    ('ef1b0d59-fe1f-4e1f-b749-8d0630b883c8', '591', 'Ajustări pentru pierderea de valoare a acțiunilor', 'Provisions - shares', 5, 'asset', 'credit', '59', 3, FALSE, TRUE, NULL, 'Provisions for share impairment', NOW(), NOW()),
    ('70a74a7e-5d2b-4dfa-9fb6-77df356b6454', '595', 'Ajustări pentru pierderea de valoare a obligațiunilor', 'Provisions - bonds', 5, 'asset', 'credit', '59', 3, FALSE, TRUE, NULL, 'Provisions for bond impairment', NOW(), NOW()),
    ('18748077-3293-4a5d-b056-57612e239c82', '6', 'Conturi de cheltuieli', 'Expense accounts', 6, 'expense', 'debit', NULL, 1, TRUE, TRUE, NULL, 'Summary account for expenses', NOW(), NOW()),
    ('fc624015-4e23-4e90-82fa-61865e7ff99b', '60', 'Cheltuieli privind stocurile', 'Inventory expenses', 6, 'expense', 'debit', '6', 2, TRUE, TRUE, NULL, 'Cost of materials used', NOW(), NOW()),
    ('7b224cd6-eb5d-427b-b676-7d60bd524807', '601', 'Cheltuieli cu materiile prime', 'Raw materials expense', 6, 'expense', 'debit', '60', 3, FALSE, TRUE, NULL, 'Cost of raw materials consumed', NOW(), NOW()),
    ('90dae92d-d0d2-4b50-856b-e7a53e5965fb', '602', 'Cheltuieli cu materialele consumabile', 'Consumables expense', 6, 'expense', 'debit', '60', 3, FALSE, TRUE, NULL, 'Cost of consumables used', NOW(), NOW()),
    ('2b158158-9caa-4e8e-8cdf-05e63bd867d1', '603', 'Cheltuieli privind materialele de natura obiectelor de inventar', 'Low-value items expense', 6, 'expense', 'debit', '60', 3, FALSE, TRUE, NULL, 'Cost of small tools consumed', NOW(), NOW()),
    ('4b13a725-6a84-4a4b-b24e-e5f801d4e58c', '604', 'Cheltuieli privind materialele nestocate', 'Non-stocked materials expense', 6, 'expense', 'debit', '60', 3, FALSE, TRUE, NULL, 'Materials expensed directly', NOW(), NOW()),
    ('c797621b-f844-4b38-bfff-16335a9f2c2c', '605', 'Cheltuieli privind energia și apa', 'Utilities expense', 6, 'expense', 'debit', '60', 3, FALSE, TRUE, NULL, 'Electricity, gas, water costs', NOW(), NOW()),
    ('7ca25a6b-a83c-4ef9-8106-3a352aed556d', '606', 'Cheltuieli privind animalele și păsările', 'Livestock expense', 6, 'expense', 'debit', '60', 3, FALSE, TRUE, NULL, 'Cost of animals consumed', NOW(), NOW()),
    ('1aff08b9-6dba-4fcb-ae7a-280559a576df', '607', 'Cheltuieli privind mărfurile', 'Cost of goods sold', 6, 'expense', 'debit', '60', 3, FALSE, TRUE, NULL, 'Cost of merchandise sold', NOW(), NOW()),
    ('a92495b3-c2e9-4648-a78d-bd354c38f97e', '608', 'Cheltuieli privind ambalajele', 'Packaging expense', 6, 'expense', 'debit', '60', 3, FALSE, TRUE, NULL, 'Cost of packaging used', NOW(), NOW()),
    ('f15e8af4-b099-4a02-a073-e076f78bf34d', '61', 'Cheltuieli cu serviciile executate de terți', 'Third-party services', 6, 'expense', 'debit', '6', 2, TRUE, TRUE, NULL, 'External services expense', NOW(), NOW()),
    ('3531c1ec-8022-403b-94f5-eb50804f5cda', '611', 'Cheltuieli cu întreținerea și reparațiile', 'Maintenance and repairs', 6, 'expense', 'debit', '61', 3, FALSE, TRUE, NULL, 'Maintenance and repair costs', NOW(), NOW()),
    ('c0102960-dbfd-4fbd-91aa-a10ec9ffd3d1', '612', 'Cheltuieli cu redevențele, locațiile de gestiune și chiriile', 'Rent and leasing', 6, 'expense', 'debit', '61', 3, FALSE, TRUE, NULL, 'Rental and lease payments', NOW(), NOW()),
    ('f5a4be40-b701-4f02-aef6-d8353bd7fb37', '613', 'Cheltuieli cu primele de asigurare', 'Insurance expense', 6, 'expense', 'debit', '61', 3, FALSE, TRUE, NULL, 'Insurance premiums', NOW(), NOW()),
    ('501b930f-fa4a-4640-bce3-62315d5b82a2', '614', 'Cheltuieli cu studiile și cercetările', 'Research expense', 6, 'expense', 'debit', '61', 3, FALSE, TRUE, NULL, 'Research and studies costs', NOW(), NOW()),
    ('f30f5e02-d595-43b0-8775-f1a45506cedc', '62', 'Cheltuieli cu alte servicii executate de terți', 'Other third-party services', 6, 'expense', 'debit', '6', 2, TRUE, TRUE, NULL, 'Other external services', NOW(), NOW()),
    ('85ca59cf-6644-40ab-b2e8-12a72bd75646', '621', 'Cheltuieli cu colaboratorii', 'Contractors expense', 6, 'expense', 'debit', '62', 3, FALSE, TRUE, NULL, 'Independent contractor costs', NOW(), NOW()),
    ('b70eeef0-17f9-4fcb-b7b7-0fe47c7d52af', '622', 'Cheltuieli privind comisioanele și onorariile', 'Commissions and fees', 6, 'expense', 'debit', '62', 3, FALSE, TRUE, NULL, 'Commission and professional fees', NOW(), NOW()),
    ('e09404c3-c19d-41cf-9935-127f28cf98d2', '623', 'Cheltuieli de protocol, reclamă și publicitate', 'Advertising expense', 6, 'expense', 'debit', '62', 3, FALSE, TRUE, NULL, 'Entertainment and advertising', NOW(), NOW()),
    ('33d0f0d9-cc90-4781-8404-8a9e12d1aec7', '624', 'Cheltuieli cu transportul de bunuri și personal', 'Transport expense', 6, 'expense', 'debit', '62', 3, FALSE, TRUE, NULL, 'Freight and employee transport', NOW(), NOW()),
    ('a579fc6d-8588-4284-b8c3-5f2f5d39a753', '625', 'Cheltuieli cu deplasări, detașări și transferări', 'Travel expense', 6, 'expense', 'debit', '62', 3, FALSE, TRUE, NULL, 'Business travel costs', NOW(), NOW()),
    ('309e095c-b65d-4a7f-ab35-295e1ee1b9ca', '626', 'Cheltuieli poștale și taxe de telecomunicații', 'Postal and telecom', 6, 'expense', 'debit', '62', 3, FALSE, TRUE, NULL, 'Postage and telecommunications', NOW(), NOW()),
    ('9dc93472-a8fe-4b2a-be95-4c78189ffde3', '627', 'Cheltuieli cu serviciile bancare și asimilate', 'Bank charges', 6, 'expense', 'debit', '62', 3, FALSE, TRUE, NULL, 'Banking fees and charges', NOW(), NOW()),
    ('459a67f6-7821-4e05-9aef-56a26892105a', '628', 'Alte cheltuieli cu serviciile executate de terți', 'Other services expense', 6, 'expense', 'debit', '62', 3, FALSE, TRUE, NULL, 'Other external service costs', NOW(), NOW()),
    ('e43fa913-d898-4bcd-969f-ef35a40aeda3', '63', 'Cheltuieli cu alte impozite, taxe', 'Other taxes expense', 6, 'expense', 'debit', '6', 2, TRUE, TRUE, NULL, 'Non-income taxes', NOW(), NOW()),
    ('6c7036de-5955-47f7-b7b1-db3adae73452', '635', 'Cheltuieli cu alte impozite, taxe și vărsăminte asimilate', 'Other taxes and duties', 6, 'expense', 'debit', '63', 3, FALSE, TRUE, NULL, 'Property tax, vehicle tax, etc.', NOW(), NOW()),
    ('45b77235-b29c-4258-98d6-77f491b39ebb', '64', 'Cheltuieli cu personalul', 'Personnel expense', 6, 'expense', 'debit', '6', 2, TRUE, TRUE, NULL, 'Payroll costs', NOW(), NOW()),
    ('c6273d5d-67e5-4533-9375-a6b342720eca', '641', 'Cheltuieli cu salariile personalului', 'Salaries expense', 6, 'expense', 'debit', '64', 3, FALSE, TRUE, NULL, 'Gross wages and salaries', NOW(), NOW()),
    ('f219a948-270f-4061-be08-009d9f23db7e', '642', 'Cheltuieli cu tichetele de masă acordate salariaților', 'Meal vouchers expense', 6, 'expense', 'debit', '64', 3, FALSE, TRUE, NULL, 'Meal ticket costs', NOW(), NOW()),
    ('46f7fb99-c168-4411-95be-9bf8fc27997e', '643', 'Cheltuieli cu primele reprezentând participarea personalului la profit', 'Profit sharing expense', 6, 'expense', 'debit', '64', 3, FALSE, TRUE, NULL, 'Employee profit sharing costs', NOW(), NOW()),
    ('bacf34c1-2f82-45b1-8768-e4aa0f21e244', '644', 'Cheltuieli cu remunerarea în instrumente de capitaluri proprii', 'Share-based payments', 6, 'expense', 'debit', '64', 3, FALSE, TRUE, NULL, 'Stock option expense', NOW(), NOW()),
    ('e5dc00c6-179f-4278-adf5-4c579cf69eab', '645', 'Cheltuieli privind asigurările și protecția socială', 'Social security expense', 6, 'expense', 'debit', '64', 3, FALSE, TRUE, NULL, 'Employer social contributions', NOW(), NOW()),
    ('ff348987-47bb-444c-9966-5b4f189c71c6', '6451', 'Contribuția unității la asigurările sociale', 'Pension contributions - employer', 6, 'expense', 'debit', '645', 4, FALSE, TRUE, NULL, 'Employer pension contribution', NOW(), NOW()),
    ('18144eab-3389-4015-90de-b04e8c9f5409', '6452', 'Contribuția unității pentru ajutorul de șomaj', 'Unemployment - employer', 6, 'expense', 'debit', '645', 4, FALSE, TRUE, NULL, 'Employer unemployment contribution', NOW(), NOW()),
    ('321e0bf7-47d6-4894-929b-456bca6b0003', '6453', 'Contribuția angajatorului pentru asigurări sociale de sănătate', 'Health insurance - employer', 6, 'expense', 'debit', '645', 4, FALSE, TRUE, NULL, 'Employer health insurance contribution', NOW(), NOW()),
    ('c2b6012d-0bcf-42cd-be05-46593ec39a23', '6458', 'Alte cheltuieli privind asigurările și protecția socială', 'Other social expense', 6, 'expense', 'debit', '645', 4, FALSE, TRUE, NULL, 'Other social security costs', NOW(), NOW()),
    ('5ff68fc7-658d-4966-a164-82e54c61b474', '65', 'Alte cheltuieli de exploatare', 'Other operating expenses', 6, 'expense', 'debit', '6', 2, TRUE, TRUE, NULL, 'Other operating costs', NOW(), NOW()),
    ('0e72f7b7-b4fe-4ff0-993d-cf109f867330', '654', 'Pierderi din creanțe și debitori diverși', 'Bad debt expense', 6, 'expense', 'debit', '65', 3, FALSE, TRUE, NULL, 'Losses from uncollectable receivables', NOW(), NOW()),
    ('e0612293-7482-445c-88d2-44d7410db778', '658', 'Alte cheltuieli de exploatare', 'Other operating expenses', 6, 'expense', 'debit', '65', 3, FALSE, TRUE, NULL, 'Miscellaneous operating costs', NOW(), NOW()),
    ('12f84dfb-3f5a-45b6-8565-b9d3370a18e3', '66', 'Cheltuieli financiare', 'Financial expenses', 6, 'expense', 'debit', '6', 2, TRUE, TRUE, NULL, 'Financial costs', NOW(), NOW()),
    ('ac780066-d3ae-4a88-afc6-82f3ddfc459f', '663', 'Pierderi din creanțe legate de participații', 'Losses from participations', 6, 'expense', 'debit', '66', 3, FALSE, TRUE, NULL, 'Losses on equity investments', NOW(), NOW()),
    ('6bcffb7c-ef63-47bb-bccf-c7a0fc663eb3', '664', 'Cheltuieli privind investițiile financiare cedate', 'Investment disposal losses', 6, 'expense', 'debit', '66', 3, FALSE, TRUE, NULL, 'Losses on investment disposals', NOW(), NOW()),
    ('67691cac-6d4d-4d15-86b8-e3de6ad9fc9f', '665', 'Cheltuieli din diferențe de curs valutar', 'Foreign exchange losses', 6, 'expense', 'debit', '66', 3, FALSE, TRUE, NULL, 'Currency exchange losses', NOW(), NOW()),
    ('5a72c3f6-099c-4c22-81a5-3afc254229aa', '666', 'Cheltuieli privind dobânzile', 'Interest expense', 6, 'expense', 'debit', '66', 3, FALSE, TRUE, NULL, 'Interest costs', NOW(), NOW()),
    ('1d177fd1-9ef0-460e-9823-d3221661e936', '667', 'Cheltuieli privind sconturile acordate', 'Discount expense', 6, 'expense', 'debit', '66', 3, FALSE, TRUE, NULL, 'Cash discounts given', NOW(), NOW()),
    ('53f5b035-f624-4659-b19a-af5f1759eadb', '668', 'Alte cheltuieli financiare', 'Other financial expenses', 6, 'expense', 'debit', '66', 3, FALSE, TRUE, NULL, 'Other financial costs', NOW(), NOW()),
    ('0dd56787-3326-498b-8dbc-5a9f60d0424f', '68', 'Cheltuieli cu amortizările, provizioanele și ajustările', 'Depreciation and provisions', 6, 'expense', 'debit', '6', 2, TRUE, TRUE, NULL, 'Non-cash expenses', NOW(), NOW()),
    ('538338a7-82e2-4433-8594-5b1372c27a65', '681', 'Cheltuieli de exploatare privind amortizările', 'Depreciation expense', 6, 'expense', 'debit', '68', 3, FALSE, TRUE, NULL, 'Depreciation of fixed assets', NOW(), NOW()),
    ('93c153b1-3538-4632-b70b-3a5f708ab421', '6811', 'Cheltuieli de exploatare privind amortizarea imobilizărilor', 'Depreciation - fixed assets', 6, 'expense', 'debit', '681', 4, FALSE, TRUE, NULL, 'Depreciation expense', NOW(), NOW()),
    ('3bfb55cc-7baa-4f20-901d-287da4e866e5', '686', 'Cheltuieli financiare privind amortizările și ajustările', 'Financial amortization', 6, 'expense', 'debit', '68', 3, FALSE, TRUE, NULL, 'Amortization of financial items', NOW(), NOW()),
    ('34a020b3-3e08-4991-b3ec-a2b932d42347', '69', 'Cheltuieli cu impozitul pe profit', 'Income tax expense', 6, 'expense', 'debit', '6', 2, TRUE, TRUE, NULL, 'Corporate income tax', NOW(), NOW()),
    ('ed970192-113e-4203-9a6f-3123ae3a3bb5', '691', 'Cheltuieli cu impozitul pe profit curent', 'Current income tax', 6, 'expense', 'debit', '69', 3, FALSE, TRUE, NULL, 'Current period income tax', NOW(), NOW()),
    ('2d1a74a9-0b44-4f6f-8c09-db692b4aa8c5', '692', 'Cheltuieli cu impozitul pe profit amânat', 'Deferred income tax', 6, 'expense', 'debit', '69', 3, FALSE, TRUE, NULL, 'Deferred tax expense', NOW(), NOW()),
    ('2d377f2b-c484-4f89-8f97-bb4f7057433d', '7', 'Conturi de venituri', 'Revenue accounts', 7, 'revenue', 'credit', NULL, 1, TRUE, TRUE, NULL, 'Summary account for revenue', NOW(), NOW()),
    ('5815a653-ae7a-4a00-a73d-3dff84a83eb4', '70', 'Cifra de afaceri netă', 'Net turnover', 7, 'revenue', 'credit', '7', 2, TRUE, TRUE, NULL, 'Sales revenue', NOW(), NOW()),
    ('b93838a8-641b-4b6b-9a19-e7277c6bb109', '701', 'Venituri din vânzarea produselor finite', 'Revenue - finished products', 7, 'revenue', 'credit', '70', 3, FALSE, TRUE, NULL, 'Sales of manufactured goods', NOW(), NOW()),
    ('b4122a07-585a-4b47-8771-00f89750305f', '702', 'Venituri din vânzarea semifabricatelor', 'Revenue - semi-finished products', 7, 'revenue', 'credit', '70', 3, FALSE, TRUE, NULL, 'Sales of semi-finished goods', NOW(), NOW()),
    ('9d341197-a7f7-4191-b72b-b66923b45081', '703', 'Venituri din vânzarea produselor reziduale', 'Revenue - by-products', 7, 'revenue', 'credit', '70', 3, FALSE, TRUE, NULL, 'Sales of by-products', NOW(), NOW()),
    ('37982819-9920-4bfe-881d-36f8af822c0c', '704', 'Venituri din servicii prestate', 'Revenue - services', 7, 'revenue', 'credit', '70', 3, FALSE, TRUE, NULL, 'Revenue from services rendered', NOW(), NOW()),
    ('050a1a6d-56fc-4fd0-a12c-8208c4eaabc4', '705', 'Venituri din studii și cercetări', 'Revenue - research', 7, 'revenue', 'credit', '70', 3, FALSE, TRUE, NULL, 'Revenue from research services', NOW(), NOW()),
    ('fc8e4f26-be75-4357-9192-4de6a0bf71df', '706', 'Venituri din redevențe, locații de gestiune și chirii', 'Revenue - rentals', 7, 'revenue', 'credit', '70', 3, FALSE, TRUE, NULL, 'Rental and royalty income', NOW(), NOW()),
    ('82c20180-bd53-4af4-9a0f-e020ed131bac', '707', 'Venituri din vânzarea mărfurilor', 'Revenue - merchandise', 7, 'revenue', 'credit', '70', 3, FALSE, TRUE, NULL, 'Sales of merchandise', NOW(), NOW()),
    ('9ca4aabe-ac82-49ed-a04a-f856784f148d', '708', 'Venituri din activități diverse', 'Revenue - other activities', 7, 'revenue', 'credit', '70', 3, FALSE, TRUE, NULL, 'Other operating revenue', NOW(), NOW()),
    ('caf81338-71e2-4670-b18e-ae33d3d58821', '71', 'Venituri aferente costurilor stocurilor de produse', 'Change in inventory', 7, 'revenue', 'credit', '7', 2, TRUE, TRUE, NULL, 'Inventory production adjustments', NOW(), NOW()),
    ('a2aac19d-e516-4861-bcc1-70082c3c9481', '711', 'Venituri aferente costurilor stocurilor de produse', 'Inventory production', 7, 'revenue', 'credit', '71', 3, FALSE, TRUE, NULL, 'Change in finished goods inventory', NOW(), NOW()),
    ('790d9378-7f28-43ec-a847-07cc9c694492', '72', 'Venituri din producția de imobilizări', 'Own-work capitalized', 7, 'revenue', 'credit', '7', 2, TRUE, TRUE, NULL, 'Self-constructed assets', NOW(), NOW()),
    ('fbad088b-a754-4419-905c-4b859cd63aee', '721', 'Venituri din producția de imobilizări necorporale', 'Own-work - intangibles', 7, 'revenue', 'credit', '72', 3, FALSE, TRUE, NULL, 'Self-produced intangible assets', NOW(), NOW()),
    ('0f7b1eb8-2d38-4723-9022-b05c9f9db89b', '722', 'Venituri din producția de imobilizări corporale', 'Own-work - tangibles', 7, 'revenue', 'credit', '72', 3, FALSE, TRUE, NULL, 'Self-constructed tangible assets', NOW(), NOW()),
    ('255bbb5a-cbea-4fe8-8149-26de08c52384', '74', 'Venituri din subvenții de exploatare', 'Operating subsidies', 7, 'revenue', 'credit', '7', 2, TRUE, TRUE, NULL, 'Government grants - operating', NOW(), NOW()),
    ('d98ba6f7-549e-46f8-a3e5-878a19d6aa83', '741', 'Venituri din subvenții de exploatare', 'Operating subsidies', 7, 'revenue', 'credit', '74', 3, FALSE, TRUE, NULL, 'Operating grants received', NOW(), NOW()),
    ('fc0e0c56-9235-40be-a655-69a64718a3a7', '75', 'Alte venituri din exploatare', 'Other operating revenue', 7, 'revenue', 'credit', '7', 2, TRUE, TRUE, NULL, 'Other operating income', NOW(), NOW()),
    ('62c916b6-3032-4232-a985-e9a7e96f7a6f', '754', 'Venituri din creanțe reactivate și debitori diverși', 'Recovered receivables', 7, 'revenue', 'credit', '75', 3, FALSE, TRUE, NULL, 'Previously written-off amounts recovered', NOW(), NOW()),
    ('597e32c7-4115-4372-94ed-7358e733f758', '758', 'Alte venituri din exploatare', 'Other operating revenue', 7, 'revenue', 'credit', '75', 3, FALSE, TRUE, NULL, 'Miscellaneous operating income', NOW(), NOW()),
    ('0a4f4f9a-a07c-45ff-8d09-f68c1de1f75f', '76', 'Venituri financiare', 'Financial revenue', 7, 'revenue', 'credit', '7', 2, TRUE, TRUE, NULL, 'Financial income', NOW(), NOW()),
    ('0bd738ce-ddfd-4977-a58f-089e2d430ede', '761', 'Venituri din imobilizări financiare', 'Revenue - financial assets', 7, 'revenue', 'credit', '76', 3, FALSE, TRUE, NULL, 'Income from long-term investments', NOW(), NOW()),
    ('f2b7e733-778d-418d-877b-aa5266208a38', '762', 'Venituri din investiții pe termen scurt', 'Revenue - short-term investments', 7, 'revenue', 'credit', '76', 3, FALSE, TRUE, NULL, 'Income from short-term investments', NOW(), NOW()),
    ('44b097cf-d65a-4e1a-b5c3-9c007c95fdf4', '763', 'Venituri din creanțe imobilizate', 'Revenue - fixed receivables', 7, 'revenue', 'credit', '76', 3, FALSE, TRUE, NULL, 'Interest on long-term receivables', NOW(), NOW()),
    ('e35ed8cc-4fde-481a-854b-a9f33ed071bc', '764', 'Venituri din investiții financiare cedate', 'Gains on investment disposal', 7, 'revenue', 'credit', '76', 3, FALSE, TRUE, NULL, 'Gains on sale of investments', NOW(), NOW()),
    ('74f63bee-1094-421c-ad0c-0bd449a32486', '765', 'Venituri din diferențe de curs valutar', 'Foreign exchange gains', 7, 'revenue', 'credit', '76', 3, FALSE, TRUE, NULL, 'Currency exchange gains', NOW(), NOW()),
    ('6dc3b9c1-a940-428d-b6f2-d7b44ab9d495', '766', 'Venituri din dobânzi', 'Interest revenue', 7, 'revenue', 'credit', '76', 3, FALSE, TRUE, NULL, 'Interest income', NOW(), NOW()),
    ('30c41571-9131-4724-a3af-ea45a00503f1', '767', 'Venituri din sconturi obținute', 'Discount revenue', 7, 'revenue', 'credit', '76', 3, FALSE, TRUE, NULL, 'Cash discounts received', NOW(), NOW()),
    ('12aa0e6e-5e61-4ba2-94f8-f61ce3a57f31', '768', 'Alte venituri financiare', 'Other financial revenue', 7, 'revenue', 'credit', '76', 3, FALSE, TRUE, NULL, 'Other financial income', NOW(), NOW()),
    ('2f97d543-eb7f-4ed4-b551-67621693109b', '78', 'Venituri din provizioane și ajustări', 'Provision reversals', 7, 'revenue', 'credit', '7', 2, TRUE, TRUE, NULL, 'Provision and depreciation reversals', NOW(), NOW()),
    ('2c8bd043-8fb5-4fe3-83b1-a020fc4f36d9', '781', 'Venituri din provizioane și ajustări de exploatare', 'Operating provision reversals', 7, 'revenue', 'credit', '78', 3, FALSE, TRUE, NULL, 'Operating provision write-backs', NOW(), NOW()),
    ('859507a8-2c43-4616-a56d-e5e51d1e1a7d', '786', 'Venituri financiare din ajustări', 'Financial adjustment reversals', 7, 'revenue', 'credit', '78', 3, FALSE, TRUE, NULL, 'Financial provision write-backs', NOW(), NOW())
ON CONFLICT (code) DO NOTHING;

-- Configured 4-digit analytic subaccounts supplied for Diana.
-- These rows are application configuration, not legal evidence.

-- First, update 3-digit parent accounts to be synthetic where they have sub-accounts
UPDATE accounts SET is_synthetic = true WHERE code IN (
    -- Class 1
    '101', '106', '107', '151', '162',
    -- Class 2
    '211', '281',
    -- Class 3
    '391',
    -- Class 4
    '411', '431', '441', '442', '519', '531',
    -- Class 5
    '512', '518',
    -- Class 6
    '601', '602', '603', '604', '605', '607', '611', '612', '613', '614',
    '621', '622', '623', '624', '625', '626', '627', '628', '635', '645', '681',
    -- Class 7
    '761', '762'
);

INSERT INTO accounts (id, created_at, updated_at, code, name, name_en, account_class, account_type, normal_side, parent_code, level, is_synthetic, is_active, description, description_en) VALUES

-- ============================================
-- CLASS 6 - Expenses (detailed sub-accounts)
-- ============================================

-- 601 - Raw materials (Materii prime)
(gen_random_uuid()::text, NOW(), NOW(), '6011', 'Cheltuieli cu materiile prime', 'Raw materials expense - general', 6, 'expense', 'debit', '601', 4, false, true, NULL, 'General raw materials'),
(gen_random_uuid()::text, NOW(), NOW(), '6012', 'Cheltuieli cu materiale pentru producție', 'Production materials expense', 6, 'expense', 'debit', '601', 4, false, true, NULL, 'Materials for production'),
(gen_random_uuid()::text, NOW(), NOW(), '6013', 'Cheltuieli cu metale și aliaje', 'Metals and alloys expense', 6, 'expense', 'debit', '601', 4, false, true, NULL, 'Metals and metal products'),
(gen_random_uuid()::text, NOW(), NOW(), '6014', 'Cheltuieli cu materiale de construcție', 'Construction materials expense', 6, 'expense', 'debit', '601', 4, false, true, NULL, 'Construction materials'),
(gen_random_uuid()::text, NOW(), NOW(), '6015', 'Cheltuieli cu produse chimice', 'Chemicals expense', 6, 'expense', 'debit', '601', 4, false, true, NULL, 'Chemical products'),
(gen_random_uuid()::text, NOW(), NOW(), '6018', 'Alte cheltuieli cu materiile prime', 'Other raw materials expense', 6, 'expense', 'debit', '601', 4, false, true, NULL, 'Other raw materials'),

-- 602 - Consumables (Materiale consumabile)
(gen_random_uuid()::text, NOW(), NOW(), '6021', 'Cheltuieli cu materialele auxiliare', 'Auxiliary materials expense', 6, 'expense', 'debit', '602', 4, false, true, NULL, 'Auxiliary materials'),
(gen_random_uuid()::text, NOW(), NOW(), '6022', 'Cheltuieli cu combustibilii', 'Fuel expense', 6, 'expense', 'debit', '602', 4, false, true, NULL, 'Fuel costs'),
(gen_random_uuid()::text, NOW(), NOW(), '6023', 'Cheltuieli cu piesele de schimb', 'Spare parts expense', 6, 'expense', 'debit', '602', 4, false, true, NULL, 'Spare parts'),
(gen_random_uuid()::text, NOW(), NOW(), '6024', 'Cheltuieli cu materialele de ambalat', 'Packaging materials expense', 6, 'expense', 'debit', '602', 4, false, true, NULL, 'Packaging materials'),
(gen_random_uuid()::text, NOW(), NOW(), '6025', 'Cheltuieli cu semințe și materiale de plantat', 'Seeds and planting materials', 6, 'expense', 'debit', '602', 4, false, true, NULL, 'Seeds and planting materials'),
(gen_random_uuid()::text, NOW(), NOW(), '6026', 'Cheltuieli cu furaje', 'Animal feed expense', 6, 'expense', 'debit', '602', 4, false, true, NULL, 'Animal feed'),
(gen_random_uuid()::text, NOW(), NOW(), '6027', 'Cheltuieli cu produse alimentare', 'Food products expense', 6, 'expense', 'debit', '602', 4, false, true, NULL, 'Food and beverage products'),
(gen_random_uuid()::text, NOW(), NOW(), '6028', 'Alte cheltuieli cu materialele consumabile', 'Other consumables expense', 6, 'expense', 'debit', '602', 4, false, true, NULL, 'Other consumables'),

-- 603 - Low-value items (Obiecte de inventar)
(gen_random_uuid()::text, NOW(), NOW(), '6031', 'Cheltuieli cu obiectele de inventar', 'Low-value inventory items', 6, 'expense', 'debit', '603', 4, false, true, NULL, 'Small tools and equipment'),
(gen_random_uuid()::text, NOW(), NOW(), '6032', 'Cheltuieli cu echipamentul de protecție', 'Protective equipment expense', 6, 'expense', 'debit', '603', 4, false, true, NULL, 'Safety equipment'),
(gen_random_uuid()::text, NOW(), NOW(), '6033', 'Cheltuieli cu echipamente IT mici', 'Small IT equipment expense', 6, 'expense', 'debit', '603', 4, false, true, NULL, 'Small IT equipment'),
(gen_random_uuid()::text, NOW(), NOW(), '6034', 'Cheltuieli cu mobilier de birou', 'Office furniture expense', 6, 'expense', 'debit', '603', 4, false, true, NULL, 'Office furniture'),

-- 604 - Non-stocked materials
(gen_random_uuid()::text, NOW(), NOW(), '6041', 'Cheltuieli cu materialele nestocate', 'Non-stocked materials', 6, 'expense', 'debit', '604', 4, false, true, NULL, 'Materials expensed directly'),

-- 605 - Utilities (Energie și apă)
(gen_random_uuid()::text, NOW(), NOW(), '6051', 'Cheltuieli cu energia electrică', 'Electricity expense', 6, 'expense', 'debit', '605', 4, false, true, NULL, 'Electricity costs'),
(gen_random_uuid()::text, NOW(), NOW(), '6052', 'Cheltuieli cu gazele naturale', 'Natural gas expense', 6, 'expense', 'debit', '605', 4, false, true, NULL, 'Natural gas costs'),
(gen_random_uuid()::text, NOW(), NOW(), '6053', 'Cheltuieli cu apa', 'Water expense', 6, 'expense', 'debit', '605', 4, false, true, NULL, 'Water supply costs'),
(gen_random_uuid()::text, NOW(), NOW(), '6054', 'Cheltuieli cu încălzirea', 'Heating expense', 6, 'expense', 'debit', '605', 4, false, true, NULL, 'District heating costs'),
(gen_random_uuid()::text, NOW(), NOW(), '6055', 'Cheltuieli cu canalizarea', 'Sewerage expense', 6, 'expense', 'debit', '605', 4, false, true, NULL, 'Sewerage costs'),

-- 607 - Cost of goods sold (Mărfuri)
(gen_random_uuid()::text, NOW(), NOW(), '6071', 'Cheltuieli privind mărfurile', 'Cost of goods sold - general', 6, 'expense', 'debit', '607', 4, false, true, NULL, 'General merchandise costs'),
(gen_random_uuid()::text, NOW(), NOW(), '6072', 'Cheltuieli privind mărfurile en-gros', 'Wholesale goods expense', 6, 'expense', 'debit', '607', 4, false, true, NULL, 'Wholesale merchandise'),
(gen_random_uuid()::text, NOW(), NOW(), '6073', 'Cheltuieli privind mărfurile cu amănuntul', 'Retail goods expense', 6, 'expense', 'debit', '607', 4, false, true, NULL, 'Retail merchandise'),

-- 611 - Maintenance and repairs (Întreținere și reparații)
(gen_random_uuid()::text, NOW(), NOW(), '6111', 'Cheltuieli cu întreținerea imobilizărilor', 'Fixed assets maintenance', 6, 'expense', 'debit', '611', 4, false, true, NULL, 'Maintenance of fixed assets'),
(gen_random_uuid()::text, NOW(), NOW(), '6112', 'Cheltuieli cu reparațiile clădirilor', 'Building repairs', 6, 'expense', 'debit', '611', 4, false, true, NULL, 'Building repairs'),
(gen_random_uuid()::text, NOW(), NOW(), '6113', 'Cheltuieli cu reparațiile echipamentelor', 'Equipment repairs', 6, 'expense', 'debit', '611', 4, false, true, NULL, 'Equipment repairs'),
(gen_random_uuid()::text, NOW(), NOW(), '6114', 'Cheltuieli cu reparațiile vehiculelor', 'Vehicle repairs', 6, 'expense', 'debit', '611', 4, false, true, NULL, 'Vehicle repairs'),
(gen_random_uuid()::text, NOW(), NOW(), '6115', 'Cheltuieli cu reparațiile IT', 'IT repairs', 6, 'expense', 'debit', '611', 4, false, true, NULL, 'IT equipment repairs'),
(gen_random_uuid()::text, NOW(), NOW(), '6118', 'Alte cheltuieli cu întreținerea și reparațiile', 'Other maintenance', 6, 'expense', 'debit', '611', 4, false, true, NULL, 'Other repairs'),

-- 612 - Rent and leasing (Chirii)
(gen_random_uuid()::text, NOW(), NOW(), '6121', 'Cheltuieli cu redevențele', 'Royalties expense', 6, 'expense', 'debit', '612', 4, false, true, NULL, 'Royalty payments'),
(gen_random_uuid()::text, NOW(), NOW(), '6122', 'Cheltuieli cu chiriile pentru clădiri', 'Building rent expense', 6, 'expense', 'debit', '612', 4, false, true, NULL, 'Building rent'),
(gen_random_uuid()::text, NOW(), NOW(), '6123', 'Cheltuieli cu chiriile pentru echipamente', 'Equipment rent expense', 6, 'expense', 'debit', '612', 4, false, true, NULL, 'Equipment rent'),
(gen_random_uuid()::text, NOW(), NOW(), '6124', 'Cheltuieli cu chiriile pentru vehicule', 'Vehicle lease expense', 6, 'expense', 'debit', '612', 4, false, true, NULL, 'Vehicle leasing'),
(gen_random_uuid()::text, NOW(), NOW(), '6125', 'Cheltuieli cu licențele software', 'Software license expense', 6, 'expense', 'debit', '612', 4, false, true, NULL, 'Software licenses'),

-- 613 - Insurance (Asigurări)
(gen_random_uuid()::text, NOW(), NOW(), '6131', 'Cheltuieli cu asigurările de bunuri', 'Property insurance expense', 6, 'expense', 'debit', '613', 4, false, true, NULL, 'Property insurance'),
(gen_random_uuid()::text, NOW(), NOW(), '6132', 'Cheltuieli cu asigurările auto', 'Vehicle insurance expense', 6, 'expense', 'debit', '613', 4, false, true, NULL, 'Vehicle insurance'),
(gen_random_uuid()::text, NOW(), NOW(), '6133', 'Cheltuieli cu asigurările de răspundere civilă', 'Liability insurance expense', 6, 'expense', 'debit', '613', 4, false, true, NULL, 'Liability insurance'),
(gen_random_uuid()::text, NOW(), NOW(), '6138', 'Alte cheltuieli cu asigurările', 'Other insurance expense', 6, 'expense', 'debit', '613', 4, false, true, NULL, 'Other insurance'),

-- 614 - Research (Studii și cercetări)
(gen_random_uuid()::text, NOW(), NOW(), '6141', 'Cheltuieli cu studiile și cercetările', 'R&D expense', 6, 'expense', 'debit', '614', 4, false, true, NULL, 'Research and development'),

-- 621 - Contractors (Colaboratori)
(gen_random_uuid()::text, NOW(), NOW(), '6211', 'Cheltuieli cu colaboratorii persoane fizice', 'Individual contractors', 6, 'expense', 'debit', '621', 4, false, true, NULL, 'Individual contractor fees'),
(gen_random_uuid()::text, NOW(), NOW(), '6212', 'Cheltuieli cu colaboratorii persoane juridice', 'Corporate contractors', 6, 'expense', 'debit', '621', 4, false, true, NULL, 'Corporate contractor fees'),

-- 622 - Commissions and fees (Comisioane și onorarii)
(gen_random_uuid()::text, NOW(), NOW(), '6221', 'Cheltuieli cu comisioanele', 'Commission expense', 6, 'expense', 'debit', '622', 4, false, true, NULL, 'Sales commissions'),
(gen_random_uuid()::text, NOW(), NOW(), '6222', 'Cheltuieli cu onorariile', 'Professional fees expense', 6, 'expense', 'debit', '622', 4, false, true, NULL, 'Professional fees'),
(gen_random_uuid()::text, NOW(), NOW(), '6223', 'Cheltuieli cu serviciile juridice', 'Legal services expense', 6, 'expense', 'debit', '622', 4, false, true, NULL, 'Legal fees'),
(gen_random_uuid()::text, NOW(), NOW(), '6224', 'Cheltuieli cu serviciile de contabilitate', 'Accounting services expense', 6, 'expense', 'debit', '622', 4, false, true, NULL, 'Accounting fees'),
(gen_random_uuid()::text, NOW(), NOW(), '6225', 'Cheltuieli cu serviciile de consultanță', 'Consulting services expense', 6, 'expense', 'debit', '622', 4, false, true, NULL, 'Consulting fees'),
(gen_random_uuid()::text, NOW(), NOW(), '6226', 'Cheltuieli cu serviciile de arhitectură și inginerie', 'Architecture and engineering', 6, 'expense', 'debit', '622', 4, false, true, NULL, 'Architecture and engineering fees'),
(gen_random_uuid()::text, NOW(), NOW(), '6228', 'Alte cheltuieli cu comisioanele și onorariile', 'Other professional fees', 6, 'expense', 'debit', '622', 4, false, true, NULL, 'Other professional fees'),

-- 623 - Advertising and protocol (Protocol, reclamă)
(gen_random_uuid()::text, NOW(), NOW(), '6231', 'Cheltuieli de protocol', 'Entertainment expense', 6, 'expense', 'debit', '623', 4, false, true, NULL, 'Business entertainment'),
(gen_random_uuid()::text, NOW(), NOW(), '6232', 'Cheltuieli cu reclama și publicitatea', 'Advertising expense', 6, 'expense', 'debit', '623', 4, false, true, NULL, 'Advertising and marketing'),
(gen_random_uuid()::text, NOW(), NOW(), '6233', 'Cheltuieli cu sponsorizările', 'Sponsorship expense', 6, 'expense', 'debit', '623', 4, false, true, NULL, 'Sponsorships'),
(gen_random_uuid()::text, NOW(), NOW(), '6234', 'Cheltuieli cu evenimentele', 'Events expense', 6, 'expense', 'debit', '623', 4, false, true, NULL, 'Events and conferences'),

-- 624 - Transport (Transport)
(gen_random_uuid()::text, NOW(), NOW(), '6241', 'Cheltuieli cu transportul de bunuri', 'Freight expense', 6, 'expense', 'debit', '624', 4, false, true, NULL, 'Freight costs'),
(gen_random_uuid()::text, NOW(), NOW(), '6242', 'Cheltuieli cu transportul de personal', 'Employee transport expense', 6, 'expense', 'debit', '624', 4, false, true, NULL, 'Employee transport'),
(gen_random_uuid()::text, NOW(), NOW(), '6243', 'Cheltuieli cu transportul aerian', 'Air transport expense', 6, 'expense', 'debit', '624', 4, false, true, NULL, 'Air freight'),
(gen_random_uuid()::text, NOW(), NOW(), '6244', 'Cheltuieli cu transportul maritim', 'Sea transport expense', 6, 'expense', 'debit', '624', 4, false, true, NULL, 'Sea freight'),
(gen_random_uuid()::text, NOW(), NOW(), '6245', 'Cheltuieli cu transportul feroviar', 'Rail transport expense', 6, 'expense', 'debit', '624', 4, false, true, NULL, 'Rail freight'),
(gen_random_uuid()::text, NOW(), NOW(), '6246', 'Cheltuieli cu transportul rutier', 'Road transport expense', 6, 'expense', 'debit', '624', 4, false, true, NULL, 'Road freight'),

-- 625 - Travel (Deplasări)
(gen_random_uuid()::text, NOW(), NOW(), '6251', 'Cheltuieli cu deplasările interne', 'Domestic travel expense', 6, 'expense', 'debit', '625', 4, false, true, NULL, 'Domestic business travel'),
(gen_random_uuid()::text, NOW(), NOW(), '6252', 'Cheltuieli cu deplasările externe', 'International travel expense', 6, 'expense', 'debit', '625', 4, false, true, NULL, 'International business travel'),
(gen_random_uuid()::text, NOW(), NOW(), '6253', 'Cheltuieli cu cazarea', 'Accommodation expense', 6, 'expense', 'debit', '625', 4, false, true, NULL, 'Hotel and accommodation'),
(gen_random_uuid()::text, NOW(), NOW(), '6254', 'Cheltuieli cu diurna', 'Per diem expense', 6, 'expense', 'debit', '625', 4, false, true, NULL, 'Daily allowances'),
(gen_random_uuid()::text, NOW(), NOW(), '6255', 'Cheltuieli cu mesele în deplasare', 'Meals during travel', 6, 'expense', 'debit', '625', 4, false, true, NULL, 'Meals during business travel'),

-- 626 - Postal and telecom (Poștale și telecomunicații)
(gen_random_uuid()::text, NOW(), NOW(), '6261', 'Cheltuieli poștale', 'Postal expense', 6, 'expense', 'debit', '626', 4, false, true, NULL, 'Postal services'),
(gen_random_uuid()::text, NOW(), NOW(), '6262', 'Cheltuieli cu telecomunicațiile', 'Telecommunications expense', 6, 'expense', 'debit', '626', 4, false, true, NULL, 'Phone and internet'),
(gen_random_uuid()::text, NOW(), NOW(), '6263', 'Cheltuieli cu serviciile de curierat', 'Courier expense', 6, 'expense', 'debit', '626', 4, false, true, NULL, 'Courier services'),

-- 627 - Bank charges (Servicii bancare)
(gen_random_uuid()::text, NOW(), NOW(), '6271', 'Cheltuieli cu serviciile bancare', 'Bank service charges', 6, 'expense', 'debit', '627', 4, false, true, NULL, 'Bank fees'),
(gen_random_uuid()::text, NOW(), NOW(), '6272', 'Cheltuieli cu comisioanele bancare', 'Bank commission expense', 6, 'expense', 'debit', '627', 4, false, true, NULL, 'Bank commissions'),

-- 628 - Other services (Alte servicii)
(gen_random_uuid()::text, NOW(), NOW(), '6281', 'Cheltuieli cu serviciile IT', 'IT services expense', 6, 'expense', 'debit', '628', 4, false, true, NULL, 'IT services'),
(gen_random_uuid()::text, NOW(), NOW(), '6282', 'Cheltuieli cu serviciile de pază și securitate', 'Security services expense', 6, 'expense', 'debit', '628', 4, false, true, NULL, 'Security services'),
(gen_random_uuid()::text, NOW(), NOW(), '6283', 'Cheltuieli cu serviciile de curățenie', 'Cleaning services expense', 6, 'expense', 'debit', '628', 4, false, true, NULL, 'Cleaning services'),
(gen_random_uuid()::text, NOW(), NOW(), '6284', 'Cheltuieli cu serviciile de sănătate', 'Health services expense', 6, 'expense', 'debit', '628', 4, false, true, NULL, 'Health and medical services'),
(gen_random_uuid()::text, NOW(), NOW(), '6285', 'Cheltuieli cu serviciile de educație și formare', 'Training services expense', 6, 'expense', 'debit', '628', 4, false, true, NULL, 'Training and education'),
(gen_random_uuid()::text, NOW(), NOW(), '6286', 'Cheltuieli cu serviciile de depozitare', 'Warehousing expense', 6, 'expense', 'debit', '628', 4, false, true, NULL, 'Warehousing services'),
(gen_random_uuid()::text, NOW(), NOW(), '6287', 'Cheltuieli cu serviciile de gestionare deșeuri', 'Waste management expense', 6, 'expense', 'debit', '628', 4, false, true, NULL, 'Waste management'),
(gen_random_uuid()::text, NOW(), NOW(), '6288', 'Alte cheltuieli cu serviciile terților', 'Other third-party services', 6, 'expense', 'debit', '628', 4, false, true, NULL, 'Other services'),
(gen_random_uuid()::text, NOW(), NOW(), '6289', 'Cheltuieli cu serviciile administrative', 'Administrative services expense', 6, 'expense', 'debit', '628', 4, false, true, NULL, 'Administrative services'),

-- 635 - Other taxes (Alte impozite și taxe)
(gen_random_uuid()::text, NOW(), NOW(), '6351', 'Cheltuieli cu impozitul pe clădiri', 'Building tax expense', 6, 'expense', 'debit', '635', 4, false, true, NULL, 'Property tax'),
(gen_random_uuid()::text, NOW(), NOW(), '6352', 'Cheltuieli cu impozitul pe terenuri', 'Land tax expense', 6, 'expense', 'debit', '635', 4, false, true, NULL, 'Land tax'),
(gen_random_uuid()::text, NOW(), NOW(), '6353', 'Cheltuieli cu impozitul pe vehicule', 'Vehicle tax expense', 6, 'expense', 'debit', '635', 4, false, true, NULL, 'Vehicle tax'),
(gen_random_uuid()::text, NOW(), NOW(), '6354', 'Cheltuieli cu taxele locale', 'Local taxes expense', 6, 'expense', 'debit', '635', 4, false, true, NULL, 'Local government taxes'),
(gen_random_uuid()::text, NOW(), NOW(), '6358', 'Alte cheltuieli cu impozitele și taxele', 'Other taxes expense', 6, 'expense', 'debit', '635', 4, false, true, NULL, 'Other taxes and duties'),

-- 681 - Depreciation expense
(gen_random_uuid()::text, NOW(), NOW(), '6812', 'Cheltuieli cu amortizarea construcțiilor', 'Building depreciation', 6, 'expense', 'debit', '681', 4, false, true, NULL, 'Building depreciation'),
(gen_random_uuid()::text, NOW(), NOW(), '6813', 'Cheltuieli cu amortizarea echipamentelor', 'Equipment depreciation', 6, 'expense', 'debit', '681', 4, false, true, NULL, 'Equipment depreciation'),
(gen_random_uuid()::text, NOW(), NOW(), '6814', 'Cheltuieli cu amortizarea vehiculelor', 'Vehicle depreciation', 6, 'expense', 'debit', '681', 4, false, true, NULL, 'Vehicle depreciation'),

-- ============================================
-- CLASS 7 - Revenue (detailed sub-accounts)
-- ============================================

-- 701 - Revenue from finished products
(gen_random_uuid()::text, NOW(), NOW(), '7011', 'Venituri din vânzarea produselor finite', 'Revenue - finished products', 7, 'revenue', 'credit', '701', 4, false, true, NULL, 'Sales of finished goods'),

-- 704 - Revenue from services
(gen_random_uuid()::text, NOW(), NOW(), '7041', 'Venituri din prestări de servicii', 'Revenue - services rendered', 7, 'revenue', 'credit', '704', 4, false, true, NULL, 'Service revenue'),

-- 707 - Revenue from merchandise
(gen_random_uuid()::text, NOW(), NOW(), '7071', 'Venituri din vânzarea mărfurilor', 'Revenue - merchandise sales', 7, 'revenue', 'credit', '707', 4, false, true, NULL, 'Merchandise sales'),

-- 708 - Revenue from other activities
(gen_random_uuid()::text, NOW(), NOW(), '7081', 'Venituri din activități diverse', 'Revenue - other activities', 7, 'revenue', 'credit', '708', 4, false, true, NULL, 'Other operating revenue'),

-- 766 - Interest revenue
(gen_random_uuid()::text, NOW(), NOW(), '7661', 'Venituri din dobânzi', 'Interest revenue', 7, 'revenue', 'credit', '766', 4, false, true, NULL, 'Interest income')
ON CONFLICT (code) DO NOTHING;
