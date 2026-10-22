# The web app must be built before the Dashboard so it is embedded in the binary.
# Set VERSION (e.g. make VERSION=v1.2.3) to stamp a release version into both binaries.

VERSION ?= dev
LDFLAGS := -s -w -X github.com/Doublefire-Chen/Telltale/internal/version.Version=$(VERSION)

.PHONY: build web dashboard agent test clean

build: dashboard agent

web:
	cd web && npm ci && npm run build

dashboard: web
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o bin/telltale-dashboard ./cmd/telltale-dashboard

agent:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o bin/telltale-agent ./cmd/telltale-agent

test:
	go vet ./...
	go test ./...
	cd web && npm run typecheck && npm test

clean:
	rm -rf bin web/dist/app
