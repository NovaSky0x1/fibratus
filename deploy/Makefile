# Fibratus Fleet Server — Build & Deploy Makefile
#
# Usage:
#   make build          Build fleet-server binary + dashboard
#   make server         Build fleet-server binary only
#   make dashboard      Build React dashboard only
#   make run            Run fleet-server locally (dev mode)
#   make migrate        Run database migrations
#   make install        Full install (build + systemd service)
#   make docker-up      Start via Docker Compose
#   make docker-down    Stop Docker Compose
#   make clean          Remove build artifacts

BINARY     := fleet-server
BUILD_DIR  := ../bin
SRC_DIR    := ..
DASH_DIR   := ../web/dashboard
CONFIG     := ../configs/fleet-server.yml

VERSION    := $(shell cd $(SRC_DIR) && git describe --tags 2>/dev/null || echo "dev")
COMMIT     := $(shell cd $(SRC_DIR) && git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE       := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS    := -s -w \
	-X github.com/rabbitstack/fibratus/cmd/fleet-server/app.version=$(VERSION) \
	-X github.com/rabbitstack/fibratus/cmd/fleet-server/app.commit=$(COMMIT) \
	-X github.com/rabbitstack/fibratus/cmd/fleet-server/app.date=$(DATE)

.PHONY: build server dashboard run migrate install docker-up docker-down clean help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: server dashboard ## Build everything (binary + dashboard)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY)"

server: ## Build fleet-server binary
	@echo "Building fleet-server..."
	@mkdir -p $(BUILD_DIR)
	cd $(SRC_DIR) && CGO_ENABLED=0 go build -ldflags='$(LDFLAGS)' -o $(BUILD_DIR)/$(BINARY) ./cmd/fleet-server/
	@echo "Binary: $(BUILD_DIR)/$(BINARY)"

dashboard: ## Build React dashboard
	@echo "Building dashboard..."
	cd $(DASH_DIR) && npm install --silent && npm run build
	@echo "Dashboard built: $(DASH_DIR)/dist/"

run: server ## Build and run fleet-server locally
	$(BUILD_DIR)/$(BINARY) serve --config $(CONFIG)

migrate: server ## Run database migrations
	$(BUILD_DIR)/$(BINARY) migrate --config $(CONFIG)

install: build ## Full install as systemd service (requires sudo)
	@echo "Installing fleet server..."
	sudo bash install-fleet-server.sh

docker-up: ## Start via Docker Compose
	docker compose -f docker-compose.yml up -d --build

docker-down: ## Stop Docker Compose
	docker compose -f docker-compose.yml down

clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)/$(BINARY)
	rm -rf $(DASH_DIR)/node_modules $(DASH_DIR)/dist
