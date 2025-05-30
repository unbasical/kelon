PROJECT_NAME := "kelon"
PKG := "github.com/unbasical/$(PROJECT_NAME)"
PKG_LIST := $(shell go list ${PKG}/... | grep -v /vendor/)
GO_FILES := $(shell find . -name '*.go' | grep -v /vendor/ | grep -v _test.go)

.PHONY: all dep lint vet test test-coverage build install clean e2e-test load-test load-test-update-postman
 
all: build

dep: ## Get the dependencies
	@go mod download

lint-dep: ## Install linting dependencies
	@echo "========== Performing lint-dep stage"
	@sh -c "(cd /tmp && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest)"

lint: lint-dep ## Lint Golang files
	@test -f .golangci.yml || { echo ".golangci.yml file is missing"; exit 1; }
	@echo "========== Performing lint stage"
	@$(shell go env GOPATH)/bin/golangci-lint -c .golangci.yml run

vet: ## Run go vet
	@go vet ${PKG_LIST}

test: ## Run unittests
	@go test -short ${PKG_LIST}

test-coverage: ## Run tests with coverage
	@go test -short -coverprofile cover.out -covermode=atomic ${PKG_LIST} 
	# @cat cover.out >> coverage.txt

build: dep ## Build the binary file
	@go build -o out/kelon $(PKG)/cmd/kelon

install:
	@go install $(PKG)/cmd/kelon
 
clean: ## Remove previous build
	@rm -f $(PROJECT_NAME)/build
 
help: ## Display this help screen
	@grep -h -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

e2e-test:
	@go test ./test/e2e

load-test:
	docker-compose up --build -d

	if [[ $$(ls ./test/load/scripts | wc -l ) -ne 4 ]]; then make load-test-update-postman; fi

	while [[ "$$(curl -s -o /dev/null -w ''%{http_code}'' localhost:8181/health)" != "200" ]]; do sleep 2; done

	docker run -it -v $(PWD)/test/load:/output/ --rm --network="kelon_compose_network" loadimpact/k6 run /output/mongo_k6_load_tests.js || (docker-compose down --volumes; exit 1;)
	docker run -it -v $(PWD)/test/load:/output/ --rm --network="kelon_compose_network" loadimpact/k6 run /output/mysql_k6_load_tests.js || (docker-compose down --volumes; exit 1;)
	docker run -it -v $(PWD)/test/load:/output/ --rm --network="kelon_compose_network" loadimpact/k6 run /output/postgre_k6_load_tests.js || (docker-compose down --volumes; exit 1;)

	docker-compose down --volumes

	exit 0

# run once before running load test
load-test-update-postman:
	docker run -it -v $(PWD)/test/load:/output/ --rm loadimpact/postman-to-k6 /output/collections/mongo_kelon_load.postman_collection.json -o /output/scripts/mongo_default_function_autogenerated.js
	docker run -it -v $(PWD)/test/load:/output/ --rm loadimpact/postman-to-k6 /output/collections/mysql_kelon_load.postman_collection.json -o /output/scripts/mysql_default_function_autogenerated.js
	docker run -it -v $(PWD)/test/load:/output/ --rm loadimpact/postman-to-k6 /output/collections/postgre_kelon_load.postman_collection.json -o /output/scripts/postgre_default_function_autogenerated.js


integration-test:
	@go test ./test/integration