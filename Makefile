COMPOSE = docker compose

.PHONY: setup check dev diagnose down status migrate migration-status release-local deploy-gcp-test reset

setup:
	@test -f .env.local || python3 scripts/prepare-local-env.py

check: setup
	python3 scripts/check-local-env.py

dev: check
	$(COMPOSE) up --build

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
