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

.PHONY: lnnd-run lnnd-docker lnn-run docker-stop docker
