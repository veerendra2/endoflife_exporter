# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: generate ## Default target: runs code generation

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: ## Run Go tests.
	go test ./...

##@ Build

.PHONY: generate
generate: oapi-codegen ## Generate Go types from the OpenAPI spec.
	@echo "Generating Go types from $(OAPI_SPEC)..."
	@$(OAPI_CODEGEN) $(GENERATE_FLAGS) -o $(OUTPUT_FILE) $(OAPI_SPEC)
	@echo "Generated types to $(OUTPUT_FILE)"

.PHONY: run
run: ## Run the application from your host (without building a binary first).
	go run main.go --log.format=console --config=sample.config.yml

.PHONY: clean
clean: ## Remove generated files and build artifacts.
	@echo "Cleaning generated files..."
	@rm -f $(OUTPUT_FILE)
	@rm -f bin/manager
	@echo "Cleaned $(OUTPUT_FILE) and bin/manager"

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
OAPI_CODEGEN ?= $(LOCALBIN)/oapi-codegen-$(OAPI_CODEGEN_VERSION)

## Tool Versions
OAPI_CODEGEN_VERSION ?= v2.4.1 # Adjust this to the specific version you want to pin

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary (ideally with version)
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f $(1) ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
source_file="$$(echo "$(1)" | sed "s/-$(3)$$//")" ;\
if [ "$$source_file" != "$(1)" ]; then mv -f "$$source_file" $(1); fi ;\
}
endef

.PHONY: oapi-codegen
oapi-codegen: $(OAPI_CODEGEN) ## Download oapi-codegen locally if necessary.
$(OAPI_CODEGEN): $(LOCALBIN)
	$(call go-install-tool,$(OAPI_CODEGEN),github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen,$(OAPI_CODEGEN_VERSION))

# Define variables for oapi-codegen specific settings
OAPI_SPEC := pkg/endoflife/openapi.yaml
OUTPUT_FILE := pkg/endoflife/types.go
GENERATE_FLAGS := -generate types -package endoflife
