APP := cinder

.PHONY: run build tidy fmt

run:
	go run .

build:
	go build -o bin/$(APP) .

tidy:
	go mod tidy

fmt:
	go fmt ./...
