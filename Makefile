.RECIPEPREFIX = >
.PHONY: init tidy fmt check all

GO = docker run --rm --user $(shell id -u):$(shell id -g) \
  -e HOME=/tmp -e GOCACHE=/tmp/.cache \
  -v "$(CURDIR)/app":/app -w /app golang:1.26.2-alpine

init:
> $(GO) go mod init go-lxd-vm-dashboard

tidy:
> $(GO) go mod tidy

fmt:
> $(GO) gofmt -w .

check:
> $(GO) sh -c "gofmt -l . ; go vet ./... && go build -o /tmp/test ."

all:
> $(GO) sh -c "go mod tidy && gofmt -w . && go vet ./... && go build -o /tmp/main ."