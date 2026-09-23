COMPOSE = docker compose
LOCAL_DEMO_EMAIL = demo@accountingtechco.com

.PHONY: setup check dev provision-local-user diagnose down status migrate migration-status release-local deploy-gcp-test reset

setup:
	@test -f .env.local || python3 scripts/prepare-local-env.py

check: setup
	python3 scripts/check-local-env.py

dev: check
	$(COMPOSE) up -d postgres redis
	$(COMPOSE) run --rm migrate
	$(MAKE) provision-local-user
	$(COMPOSE) up --build

provision-local-user:
	@user_exists=$$($(COMPOSE) exec -T postgres psql -U diana -d diana -tAc "SELECT EXISTS (SELECT 1 FROM auth_users WHERE email = '$(LOCAL_DEMO_EMAIL)')") || exit 1; \
	if [ "$$user_exists" != "t" ]; then \
		echo "Local user $(LOCAL_DEMO_EMAIL) is missing. Choose its password:"; \
		$(COMPOSE) build api; \
		$(COMPOSE) run --rm --no-deps api authuser provision --email "$(LOCAL_DEMO_EMAIL)" --all-clients; \
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
