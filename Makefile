BINARY      := td
PKG         := ./...
INSTALL_DIR := $(HOME)/.local/bin
GO         ?= go

.PHONY: all help build run test tidy fmt vet clean install uninstall

all: build

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Compile the binary
	$(GO) build -o $(BINARY) .

run: ## Run the program
	$(GO) run .

test: ## Run tests
	$(GO) test $(PKG)

tidy: ## Tidy module dependencies
	$(GO) mod tidy

fmt: ## Format source code
	$(GO) fmt $(PKG)

vet: ## Run go vet
	$(GO) vet $(PKG)

clean: ## Remove build artifacts
	rm -f $(BINARY)

install: build ## Build and install the binary to $(INSTALL_DIR)
	@mkdir -p $(INSTALL_DIR)
	install -m 0755 $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed $(BINARY) to $(INSTALL_DIR)/$(BINARY)"

uninstall: ## Remove the installed binary
	rm -f $(INSTALL_DIR)/$(BINARY)
