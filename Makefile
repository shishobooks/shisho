SHELL := /bin/bash

BUILD_DIR ?= ./build/api

GO_TOOLS = \
	github.com/DarthSim/hivemind \
	github.com/air-verse/air \
	github.com/gzuidhof/tygo

TEST_FILES ?= ./pkg/...
TEST_FLAGS ?=
COVERAGE_PROFILE ?= coverage.out

TYGO_INPUTS = $(shell yq '.packages[] | .path + "/" + .include_files[]' tygo.yaml | sed 's|github.com/shishobooks/shisho/||' | tr '\n' ' ')
TYGO_OUTPUTS = $(shell yq '.packages[].output_path' tygo.yaml | tr '\n' ' ')

.PHONY: build
build:
	CGO_ENABLED=0 go build -o $(BUILD_DIR)/api -installsuffix cgo -ldflags '-w -s' ./cmd/api

.PHONY: build\:air
build\:air:
	go build -o $(BUILD_DIR)/api-air ./cmd/api

.PHONY: db\:migrate
db\:migrate:
	go run ./cmd/migrations init
	go run ./cmd/migrations migrate

.PHONY: db\:migrate\:create
db\:migrate\:create:
	go run ./cmd/migrations create $(name)

.PHONY: db\:rollback
db\:rollback:
	go run ./cmd/migrations rollback

.PHONY: lint
lint: $(BUILD_DIR)/golangci-lint
	$(BUILD_DIR)/golangci-lint run

$(BUILD_DIR)/golangci-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(BUILD_DIR) v1.64.8

$(BUILD_DIR)/hivemind $(BUILD_DIR)/air $(BUILD_DIR)/tygo:
	go get $(GO_TOOLS) && GOBIN=$$(pwd)/$(BUILD_DIR) go install $(GO_TOOLS)

.PHONY: start
start: $(BUILD_DIR)/hivemind
	$(BUILD_DIR)/hivemind --port 3689

.PHONY: start\:air
start\:air: $(BUILD_DIR)/air
	$(BUILD_DIR)/air

.PHONY: test
test:
	TZ=America/Chicago ENVIRONMENT=test CI=true go test -race $(TEST_FILES) -coverprofile $(COVERAGE_PROFILE) $(TEST_FLAGS)

.PHONY: tygo
tygo: $(TYGO_OUTPUTS)

$(TYGO_OUTPUTS): $(BUILD_DIR)/tygo tygo.yaml $(TYGO_INPUTS)
	$(BUILD_DIR)/tygo generate
