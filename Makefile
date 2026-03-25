APP := testrr
BIN_DIR := bin
BIN := $(BIN_DIR)/$(APP)
TEMPL_CMD := go run github.com/a-h/templ/cmd/templ@v0.3.1001

.PHONY: help deps generate assets fmt lint test build run clean check sample-upload

help:
	@printf "%s\n" \
		"Available targets:" \
		"  make deps      - install/update Go and Node dependencies" \
		"  make generate  - generate templ code" \
		"  make assets    - build embedded frontend assets" \
		"  make fmt       - format Go code" \
		"  make lint      - run Go vet and TypeScript checks" \
		"  make test      - run the Go test suite" \
		"  make build     - build the testrr binary" \
		"  make run       - run the server locally" \
		"  make sample-upload - create sample projects and upload sample timelines" \
		"  make clean     - remove built artifacts" \
		"  make check     - run generate, assets, lint, test, and build"

deps:
	go mod tidy
	npm install

generate:
	$(TEMPL_CMD) generate

assets:
	npm run build

fmt: generate
	go fmt ./...

lint: generate
	go vet ./...
	npm exec -- tsc --noEmit

test: generate assets
	go test ./...

build: generate assets
	mkdir -p $(BIN_DIR)
	go build -o $(BIN) ./cmd/testrr

run: generate assets
	go run ./cmd/testrr serve

sample-upload: build
	bash samples/upload-samples.sh

clean:
	rm -rf $(BIN_DIR)
	rm -rf web/dist

check: generate assets lint test build
