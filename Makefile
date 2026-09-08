.PHONY: build build-go web web-install install test test-go test-web coverage clean tidy lint website website-dev indexnow release dev-api dev-web docker docker-up

BINARY  := healthsync
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  = -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

build: web build-go ## Build the web UI and the binary (UI embedded)

build-go: ## Build the binary only (uses the embedded UI if already built, else a placeholder page)
	@mkdir -p bin
	go build $(LDFLAGS) -o bin/$(BINARY) .

web-install: ## Install web dependencies
	cd web && npm ci --no-audit --no-fund

web: ## Build the web UI into internal/web/dist
	cd web && npm run build

dev-api: ## Run the API/server locally against ./.data (UI served from embedded build or placeholder)
	go run . server --data-dir ./.data --port 8080

dev-web: ## Run the Vite dev server (proxies /api to :8080)
	cd web && npm run dev

docker: ## Build the Docker image locally
	docker build -t healthsync:local .

docker-up: ## Build and start with docker compose
	docker compose up -d --build

install: ## Install to $GOPATH/bin
	go install $(LDFLAGS) .

test: test-go test-web ## Run Go and web tests

test-go: ## Run Go tests
	go test ./... -count=1

test-web: ## Type-check and unit-test the web UI
	cd web && npx tsc --noEmit && npx vitest run

coverage: ## Run tests with coverage
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

coverage-html: coverage ## Open coverage report in browser
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html

clean: ## Remove build artifacts
	rm -rf bin/ coverage.out coverage.html

tidy: ## Tidy go modules
	go mod tidy

lint: ## Run go vet
	go vet ./...

release: ## Build release tarballs for GitHub upload (darwin/linux arm64+amd64, windows arm64+amd64)
	@mkdir -p bin
	@for platform in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64 windows/arm64 windows/amd64; do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		echo "Building $(BINARY)-$$os-$$arch..."; \
		if [ "$$os" = "windows" ]; then \
			GOOS=$$os GOARCH=$$arch go build $(LDFLAGS) -o bin/$(BINARY).exe . || exit 1; \
			zip -j bin/$(BINARY)-$$os-$$arch.zip bin/$(BINARY).exe; \
			rm bin/$(BINARY).exe; \
		else \
			GOOS=$$os GOARCH=$$arch go build $(LDFLAGS) -o bin/$(BINARY) . || exit 1; \
			chmod +x bin/$(BINARY); \
			tar -czf bin/$(BINARY)-$$os-$$arch.tar.gz -C bin $(BINARY); \
			rm bin/$(BINARY); \
		fi; \
	done
	@echo "Done. Upload contents of bin/ to GitHub Releases"

website: ## Build the Hugo website
	cd website && hugo --minify

website-dev: ## Start Hugo dev server
	cd website && hugo server -D

indexnow: ## Submit live sitemap URLs to IndexNow (run after deploying content changes)
	sh scripts/indexnow.sh

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
