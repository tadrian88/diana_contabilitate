COMPOSE = docker compose
LOCAL_DEMO_EMAIL = demo@accountingtechco.com
# make dev SEED=0 skips the SOFTCO2 test client; SOFTCO2_AUTO_CONFIRM=1 also
# confirms the extracted contracts and imports the 45 test invoices.
SEED ?= 1
SOFTCO2_AUTO_CONFIRM ?= 0

.PHONY: setup check dev seed-legislation provision-local-user seed-softco2 diagnose down status migrate migration-status release-local deploy-gcp-test reset

setup:
	@test -f .env.local || python3 scripts/prepare-local-env.py

check: setup
	python3 scripts/check-local-env.py

dev: check
	$(COMPOSE) up -d postgres redis
	$(COMPOSE) run --rm migrate
	$(MAKE) seed-legislation
	$(MAKE) provision-local-user
	$(COMPOSE) up -d --build --wait
	@if [ "$(SEED)" != "0" ]; then $(MAKE) seed-softco2 || echo "SOFTCO2 seed failed; the stack keeps running. Retry: make seed-softco2"; fi
	$(COMPOSE) logs -f

# Local test client SOFTCO2 SRL: company, approved accounting profile, SAGA
# export, simulated ANAF/SPV connection and contract PDFs. Idempotent.
# The password comes from DIANA_PASSWORD or is asked interactively.
seed-softco2:
	bash test-data/softco2/setup-softco2.sh tot $(if $(filter 1,$(SOFTCO2_AUTO_CONFIRM)),--auto-confirm)

# Imports the repository TEST_ONLY legislation snapshots into the global local
# corpus. Idempotent: identical versions are skipped, divergent ones fail.
seed-legislation:
	$(COMPOSE) --profile seed run --rm --build legislation-seed

provision-local-user:
	@user_exists=$$($(COMPOSE) exec -T postgres psql -U diana -d diana -tAc "SELECT EXISTS (SELECT 1 FROM auth_users WHERE email = '$(LOCAL_DEMO_EMAIL)')") || exit 1; \
	if [ "$$user_exists" != "t" ]; then \
		echo "Local user $(LOCAL_DEMO_EMAIL) is missing. Choose its password:"; \
		$(COMPOSE) build api; \
		DIANA_AUTH_PASSWORD="$$DIANA_PASSWORD" $(COMPOSE) run --rm --no-deps -e DIANA_AUTH_PASSWORD api authuser provision --email "$(LOCAL_DEMO_EMAIL)" --all-clients; \
	else \
		echo "Local user $(LOCAL_DEMO_EMAIL) already exists."; \
	fi

diagnose:
	python3 scripts/check-local-env.py --report
	-$(COMPOSE) ps -a
	-$(COMPOSE) logs --tail=80 migrate api worker frontend

down:
	$(COMPOSE) down

status:
	$(COMPOSE) ps

migrate: setup
	$(COMPOSE) run --rm migrate

migration-status: setup
	$(COMPOSE) run --rm migrate migrate status --env local

release-local:
	./scripts/release-local.sh

deploy-gcp-test:
	./scripts/gcp/deploy-test.sh

# Destructive and explicitly limited to the single local environment.
reset:
	@printf 'Delete all local data? Type DELETE: '; read answer; test "$$answer" = DELETE
	$(COMPOSE) down --volumes
