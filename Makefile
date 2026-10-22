# The web app must be built before the Dashboard so it is embedded in the binary.

.PHONY: build web dashboard test clean

build: dashboard

web:
	cd web && npm ci && npm run build

dashboard: web
	CGO_ENABLED=0 go build -o bin/telltale-dashboard ./cmd/telltale-dashboard

test:
	go vet ./...
	go test ./...
	cd web && npm run typecheck && npm test

clean:
	rm -rf bin web/dist/app
