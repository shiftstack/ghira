build: main.go
	go build -o bin/ghira ./$<

lint:
	go vet ./...
	gofmt -w -s main.go
.PHONY: lint
