# Makefile
# usage: make <target>

.PHONY: init tidy fmt fmt-fix check all lint lint-yaml lint-action lint-helm help
.DEFAULT_GOAL := help

# ---------- variables ----------
HELM_CHART_PATH := ./helm-chart/kubernetes-app-nginx
MODULE_NAME = grafan-lxd-monitoring
BUILD_OUTPUT = /tmp/main
GO = docker run --rm --user $(shell id -u):$(shell id -g) \
  -e HOME=/tmp -e GOCACHE=/tmp/.cache \
  -v "$(CURDIR)/app":/app -w /app golang:1.26.2-alpine

help:
	@echo "Go targets:"
	@echo "  init        - go mod init"
	@echo "  tidy        - go mod tidy"
	@echo "  fmt         - list files needing gofmt (no changes)"
	@echo "  fmt-fix     - gofmt -w (rewrite files)"
	@echo "  check       - gofmt check + go vet + go build"
	@echo "  all         - tidy + fmt-fix + vet + build"
	@echo "File targets:"
	@echo "  lint-yaml   - yamllint -s"
	@echo "  lint-action - actionlint on Gitea workflows"
	@echo "  lint-helm   - helm lint"
	@echo "  lint        - run all three file linters"

# ---------- Go ----------
init:
	$(GO) go mod init $(MODULE_NAME)

tidy:
	$(GO) go mod tidy

fmt:
	$(GO) gofmt -l .

fmt-fix:
	$(GO) gofmt -w .

check:
	$(GO) sh -c 'test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; } && go vet ./... && go build -o $(BUILD_OUTPUT) . && echo OK'

all:
	$(GO) sh -c 'go mod tidy && gofmt -w . && go vet ./... && go build -o /tmp/main .'

# ---------- Files ----------
lint: lint-yaml lint-action lint-helm

lint-yaml:
	@echo "Checking YAML..."
	yamllint -s .

lint-action:
	@echo "Checking workflows..."
	actionlint .gitea/workflows/*.yml

lint-helm:
	@echo "Linting Helm chart..."
	helm lint $(HELM_CHART_PATH)