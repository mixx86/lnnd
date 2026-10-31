run:
ifeq ($(TRACE),on)
	GODEBUG=gctrace=1 go run ./cmd/lnnd
else
	go run ./...
endif

lnnd-docker:
	docker run lnnd

docker-stop:
	docker compose stop

docker:
	docker compose up

GOBIN ?= $(shell go env GOPATH)/bin
GOLINES ?= $(GOBIN)/golines
GOLANGCI_LINT ?= $(GOBIN)/golangci-lint

fmt:
	gofmt -w ./cmd ./internal

golines:
	$(GOLINES) -m 80 -t 4 --base-formatter=gofmt -w ./cmd ./internal

lll:
	$(GOLANGCI_LINT) run ./...

.PHONY: run lnnd-docker docker-stop docker fmt golines lll
