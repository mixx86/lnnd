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

IFACE ?= lo
PORT ?= 5771

# Capture as root, decode as the user: tshark running as root did not
# register the Lua dissector.
sniff:
	sudo tshark -l -q -i $(IFACE) -f "tcp port $(PORT)" -w - | \
	tshark -l -r - -X lua_script:$(CURDIR)/tools/lnnp.lua \
		-o lnnp.port:$(PORT) -Y lnnp \
		-T fields -e frame.time_relative \
		-e ip.src -e tcp.srcport -e ip.dst -e tcp.dstport \
		-e _ws.col.info

.PHONY: run lnnd-docker docker-stop docker fmt golines lll sniff
