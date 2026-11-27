
# Makefile
# usage make <target>

#.RECIPEPREFIX = >
.PHONY: init tidy fmt check all lint-workflows lint-helm help
.DEFAULT_GOAL := help

help:
	@echo "Available targets:"
  @echo "--------------------------------"
  @echo "For Go module management:"
  @echo "  init - Initialize Go module"
  @echo "  tidy - Clean up Go module dependencies"
  @echo "  fmt - Format Go code"
  @echo "  check - Check Go code formatting, vet, and build"
  @echo "  all - Run tidy, fmt, vet, and build"
  @echo "--------------------------------"
  @echo "Linting workflows and Helm charts:"
	@echo "  lint-workflows - Check YAML and workflow syntax"
	@echo "  lint-helm - Lint Helm chart"
  @echo "--------------------------------"

GO = docker run --rm --user $(shell id -u):$(shell id -g) \
  -e HOME=/tmp -e GOCACHE=/tmp/.cache \
  -v "$(CURDIR)/app":/app -w /app golang:1.26.2-alpine
MODULE_NAME = grafan-lxd-monitoring
HELM_CHART_DIR = helm/kubernetes-app-nginx
BUILD_OUTPUT = /tmp/main

init:
  $(GO) go mod init $(MODULE_NAME)

tidy:
  $(GO) go mod tidy

fmt:
  $(GO) gofmt -w .

check:
  $(GO) sh -c "gofmt -l . ; go vet ./... && go build -o /tmp/test ."

all:
  $(GO) sh -c "go mod tidy && gofmt -w . && go vet ./... && go build -o $(BUILD_OUTPUT) ."

lint-workflows:
	@echo "Checking YAML syntax..."
	yamllint -d relaxed .gitea/workflows/

	@echo "Checking workflow syntax..."
	actionlint .gitea/workflows/*.yml

	@echo "Workflow linting completed!"

lint-helm:
	@echo "Linting Helm charts..."
	helm lint $(HELM_CHART_DIR)

	@echo "Helm linting completed!"