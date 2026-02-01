.PHONY: run-api run-worker

run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker
