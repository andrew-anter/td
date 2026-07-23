BINARY               := td
PKG                  := ./...
INSTALL_DIR          := $(HOME)/.local/bin
BASH_COMPLETION_DIR  := $(HOME)/.local/share/bash-completion/completions
ZSH_COMPLETION_DIR   := $(HOME)/.local/share/zsh/site-functions
FISH_COMPLETION_DIR  := $(HOME)/.local/share/fish/vendor_completions.d
GO                  ?= go

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

install: build ## Build and install the binary and shell completions
	@mkdir -p $(INSTALL_DIR)
	install -m 0755 $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed $(BINARY) to $(INSTALL_DIR)/$(BINARY)"
	@mkdir -p $(BASH_COMPLETION_DIR)
	@./$(BINARY) completion bash > $(BASH_COMPLETION_DIR)/$(BINARY)
	@echo "Installed bash completion to $(BASH_COMPLETION_DIR)/$(BINARY)"
	@mkdir -p $(ZSH_COMPLETION_DIR)
	@./$(BINARY) completion zsh > $(ZSH_COMPLETION_DIR)/_$(BINARY)
	@echo "Installed zsh completion to $(ZSH_COMPLETION_DIR)/_$(BINARY)"
	@mkdir -p $(FISH_COMPLETION_DIR)
	@./$(BINARY) completion fish > $(FISH_COMPLETION_DIR)/$(BINARY).fish
	@echo "Installed fish completion to $(FISH_COMPLETION_DIR)/$(BINARY).fish"

uninstall: ## Remove the installed binary and shell completions
	rm -f $(INSTALL_DIR)/$(BINARY)
	rm -f $(BASH_COMPLETION_DIR)/$(BINARY)
	rm -f $(ZSH_COMPLETION_DIR)/_$(BINARY)
	rm -f $(FISH_COMPLETION_DIR)/$(BINARY).fish
