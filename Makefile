LOCALBIN := $(CURDIR)/bin

GOLANGCI_LINT_VERSION := v2.15.0
GOVULNCHECK_VERSION := v1.10.0
GITLEAKS_VERSION := v8.30.1
COMMITLINT_VERSION := v0.12.0

GOLANGCI_LINT := $(LOCALBIN)/golangci-lint-$(GOLANGCI_LINT_VERSION)
GOVULNCHECK := $(LOCALBIN)/govulncheck-$(GOVULNCHECK_VERSION)
GITLEAKS := $(LOCALBIN)/gitleaks-$(GITLEAKS_VERSION)
COMMITLINT := $(LOCALBIN)/commitlint-$(COMMITLINT_VERSION)

.PHONY: test lint vulncheck secrets verify setup-hooks

test:
	go test -v -race -cover ./...

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run

vulncheck: $(GOVULNCHECK)
	$(GOVULNCHECK) ./...

secrets: $(GITLEAKS)
	$(GITLEAKS) git --redact --no-banner
	$(GITLEAKS) git --pre-commit --staged --redact --no-banner

verify: test lint vulncheck secrets

setup-hooks: $(GITLEAKS) $(COMMITLINT)
	git config --local core.hooksPath .githooks

$(GOVULNCHECK):
	@mkdir -p $(LOCALBIN)
	@rm -f $(LOCALBIN)/govulncheck $(LOCALBIN)/govulncheck-*
	GOBIN=$(LOCALBIN) go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	@mv $(LOCALBIN)/govulncheck $@
	@ln -sf $(notdir $@) $(LOCALBIN)/govulncheck

$(GITLEAKS):
	@mkdir -p $(LOCALBIN)
	@rm -f $(LOCALBIN)/gitleaks $(LOCALBIN)/gitleaks-*
	GOBIN=$(LOCALBIN) go install github.com/zricethezav/gitleaks/v8@$(GITLEAKS_VERSION)
	@mv $(LOCALBIN)/gitleaks $@
	@ln -sf $(notdir $@) $(LOCALBIN)/gitleaks

$(COMMITLINT):
	@mkdir -p $(LOCALBIN)
	@rm -f $(LOCALBIN)/commitlint $(LOCALBIN)/commitlint-*
	GOBIN=$(LOCALBIN) go install github.com/conventionalcommit/commitlint@$(COMMITLINT_VERSION)
	@mv $(LOCALBIN)/commitlint $@
	@ln -sf $(notdir $@) $(LOCALBIN)/commitlint

$(GOLANGCI_LINT):
	@mkdir -p $(LOCALBIN)
	@rm -f $(LOCALBIN)/golangci-lint $(LOCALBIN)/golangci-lint-*
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/$(GOLANGCI_LINT_VERSION)/install.sh | sh -s -- -b $(LOCALBIN) $(GOLANGCI_LINT_VERSION)
	@mv $(LOCALBIN)/golangci-lint $@
	@ln -sf $(notdir $@) $(LOCALBIN)/golangci-lint
