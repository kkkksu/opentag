BINARY := opentag
PKG := ./...

.PHONY: build run test vet lint tidy clean docker helm-lint

build:
	go build -o bin/$(BINARY) ./cmd/opentag

run: build
	./bin/$(BINARY) -config config.yaml

test:
	go test $(PKG)

vet:
	go vet $(PKG)

tidy:
	go mod tidy

lint: vet
	gofmt -l .

clean:
	rm -rf bin

docker:
	docker build -t opentag:dev -f deploy/Dockerfile .

helm-lint:
	helm lint deploy/helm/opentag --set slack.appToken=xapp-x --set slack.botToken=xoxb-x
