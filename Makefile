# The web app must be built before the Dashboard so it is embedded in the binary.

.PHONY: build web dashboard agent test clean

build: dashboard agent

web:
	cd web && npm ci && npm run build

dashboard: web
	CGO_ENABLED=0 go build -o bin/telltale-dashboard ./cmd/telltale-dashboard

agent:
	CGO_ENABLED=0 go build -o bin/telltale-agent ./cmd/telltale-agent

test:
	go vet ./...
	go test ./...
	cd web && npm run typecheck && npm test

clean:
	rm -rf bin web/dist/app
