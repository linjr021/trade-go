.PHONY: check check-go check-fe build build-fe test

check: check-go check-fe

check-go:
	go test ./...
	~/go/bin/staticcheck ./...

check-fe:
	cd frontend && npm run check

build: build-fe

build-fe:
	cd frontend && npm run build

test:
	go test ./...
