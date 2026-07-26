GO_TEST := go test -tags sqlite_fts5
UNIT_PACKAGES := ./internal/config ./internal/embed ./internal/output ./scripts ./skills
INTEGRATION_PACKAGES := ./internal/cli ./internal/repoctx ./internal/store

.PHONY: help test test-unit test-integration

help:
	@printf '%s\n' \
		'make test-unit        Run unit tests' \
		'make test-integration Run integration and smoke tests' \
		'make test             Run all tests'

test-unit:
	$(GO_TEST) $(UNIT_PACKAGES)

test-integration:
	$(GO_TEST) $(INTEGRATION_PACKAGES)
	bash scripts/installer_prompt_test.sh
	bash scripts/package_release_fixture_test.sh
	bash scripts/installer_smoke.sh

test:
	$(GO_TEST) ./...
	bash scripts/installer_prompt_test.sh
	bash scripts/package_release_fixture_test.sh
	bash scripts/installer_smoke.sh
