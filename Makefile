SHELL := /bin/bash

BUILD_DIR ?= ./build/api
DOCKER_TAG ?= latest

GO_TOOLS = \
	github.com/DarthSim/hivemind \
	github.com/air-verse/air \
	github.com/gzuidhof/tygo

TEST_FILES ?= ./pkg/...
TEST_FLAGS ?=
COVERAGE_PROFILE ?= coverage.out

TYGO_INPUTS = $(shell yq '.packages[] | .path + "/" + (.include_files[] // "*.go")' tygo.yaml | sed 's|github.com/shishobooks/shisho/||' | tr '\n' ' ')
TYGO_OUTPUTS = $(shell yq '.packages[].output_path' tygo.yaml | tr '\n' ' ')

.PHONY: check
check: tygo lint
	$(MAKE) -j3 test test\:js lint\:js

.PHONY: build
build:
	CGO_ENABLED=0 go build -o $(BUILD_DIR)/api -installsuffix cgo -ldflags '-w -s' ./cmd/api

.PHONY: build\:air
build\:air:
	go build -o $(BUILD_DIR)/api-air ./cmd/api

.PHONY: db\:migrate
db\:migrate:
	CONFIG_FILE=./shisho.dev.yaml go run ./cmd/migrations init
	CONFIG_FILE=./shisho.dev.yaml go run ./cmd/migrations migrate

.PHONY: db\:migrate\:create
db\:migrate\:create:
	CONFIG_FILE=./shisho.dev.yaml go run ./cmd/migrations create $(name)

.PHONY: db\:rollback
db\:rollback:
	CONFIG_FILE=./shisho.dev.yaml go run ./cmd/migrations rollback

.PHONY: docker
docker:
	docker build -t shisho:$(DOCKER_TAG) .

.PHONY: lint
lint: $(BUILD_DIR)/golangci-lint
	$(BUILD_DIR)/golangci-lint run

.PHONY: lint\:js
lint\:js:
	yarn lint

.PHONY: test\:js
test\:js:
	yarn test

$(BUILD_DIR)/golangci-lint:
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(BUILD_DIR) v2.7.2


$(BUILD_DIR)/hivemind $(BUILD_DIR)/air $(BUILD_DIR)/tygo:
	GOBIN=$$(pwd)/$(BUILD_DIR) go install $(GO_TOOLS)

.PHONY: setup
setup: $(BUILD_DIR)/golangci-lint tygo
	yarn

.PHONY: start
start: $(BUILD_DIR)/hivemind
	$(BUILD_DIR)/hivemind

.PHONY: start\:air
start\:air: $(BUILD_DIR)/air
	$(BUILD_DIR)/air

.PHONY: start\:api
start\:api:
	go run ./cmd/api

.PHONY: test
test:
	TZ=America/Chicago CI=true go test -race $(TEST_FILES) -coverprofile $(COVERAGE_PROFILE) $(TEST_FLAGS)

.PHONY: test\:cover
test\:cover:
	go tool cover -html=$(COVERAGE_PROFILE)

.PHONY: tygo
tygo: $(TYGO_OUTPUTS)

$(TYGO_OUTPUTS): $(BUILD_DIR)/tygo tygo.yaml $(TYGO_INPUTS)
	$(BUILD_DIR)/tygo generate
