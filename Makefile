TIMEOUT=30s
FLAGS := 

.SILENT: lint
.PHONY: lint test test/verbose test/serialized

CLEANUP=true
E2E_ARGS=
TEST_GO_FILES := go test $$(go list ./...)
TEST_COMMAND := $(TEST_GO_FILES) -p 1 -cover -coverprofile=test/coverage.out -timeout $(TIMEOUT) $(FLAGS)
JUNIT_OUTPUT := tee ./test/raw.txt && cat ./test/raw.txt | go-junit-report -set-exit-code > ./test/report.xml

ifeq (${GENERATE_JUNIT}, true)
TEST_COMMAND = go install github.com/jstemmer/go-junit-report && touch test/raw.txt && $(TEST_COMMAND) 2>&1 | $(JUNIT_OUTPUT)
endif

help:
	@grep -oR "^\(.*:\|\)\s*#help: .*$$" Makefile | sed -e 's/\(.*\):\s*#help:\(.*\)/make \1 \n\2\n/'

lint: #help: Runs golangci-lint on the entire project.
	@golangci-lint run --sort-results --config ./.config/.golangci.yaml

clean: #help: Cleans out all temporary files created from test runs
	@rm ./test/coverage.out 2> /dev/null || exit 0
	@rmdir test 2> /dev/null || exit 0

.PHONY: build
build: reflector

reflector: $(shell find ./ -type f -name *.go) #help: Builds the reflector and inserts some helpful variables at link time.
	go build -o ./reflector -ldflags \
		"-X $$(go list -m)/cmd/version.commitHash=$$(git rev-parse --short HEAD) \
		-X $$(go list -m)/cmd/version.semVer=$$(git describe --tags --always --dirty) \
		-X '$$(go list -m)/cmd/version.commitDate=$$(git log -1 --format=%ci)'" ./cmd/*.go

${GOPATH}/bin/mockery:
	@hash mockery || go get github.com/vektra/mockery/v2/.../


gen/mock: ${GOPATH}/bin/mockery \
		pkg/mocks/reflect_mock.go \
		pkg/mocks/rate_limiter_mock.go \
		pkg/mocks/metrics_server_mock.go #help: Generates mocks for various interfaces in the repository

pkg/mocks/reflect_mock.go: pkg/reflect/reflect.go
	@mockery --dir pkg/reflect/ --name Reflector --filename reflect_mock.go --output ./pkg/mocks

pkg/mocks/metrics_server_mock.go: pkg/server/server.go
	@mockery --dir pkg/server/ --name MetricsServer --filename metrics_server_mock.go --output ./pkg/mocks

pkg/mocks/rate_limiter_mock.go: pkg/queue/queue.go
	@mockery --dir pkg/queue/ --name RateLimiter --filename rate_limiter_mock.go --output ./pkg/mocks

test: #help: Runs all tests quietly. Use TIMEOUT to specify a timeout, GENERATE_JUNIT to generate junit XML from the tests, and FLAGS to set extra flags for the test run.
	@mkdir -p ./test
	@$(TEST_COMMAND)

test/race: #help: Runs all tests with -race enabled.
	@$(MAKE) -s FLAGS=-race test

test/verbose: #help: Runs all tests with verbose output enabled.
	@$(MAKE) -s FLAGS=-v test

test/verbose/race: #help: Runs all tests with verbose output and -race enabled.
	@$(MAKE) -s FLAGS="-v -race" test

test/serialized: #help: Runs all tests and generates JSON output.
	@$(MAKE) -s FLAGS="-json" test

test/e2e: #help: Runs the end to end tests through KIND
	./test/test-chart.sh ${E2E_ARGS}

coverage: #help: Generates a coverage profile from all tests.
	@mkdir -p test
	@go test -coverprofile=test/coverage.out $$(go list ./...)  -timeout $(TIMEOUT)

coverage/show: test/coverage.out #help: Opens the browser with HTML browseable coverage output.
	@go tool cover -html test/coverage.out

# Runs regression testing with benchmarks in the library against changes
# e.g. Run the benchmarks on the project, record them, and then compare the output
# to HEAD@{1} (the benchmarks before the latest change. If the difference is outside
# of a given threshold (specifically lower or higher) then failure is reported.
# Tolerances for the threshold could be tuned over time with an artifact file
regression:
	@echo "Not yet implemented"

debug/%: #help: Installs Delve, compiles a test binary, and runs dlv test
	@hash dlv || go install github.com/go-delve/delve/cmd/dlv
	@go test -c -o debug.test ./pkg/$(shell basename $@)/*.go
	@dlv test -wd ./cmd/

image: #help: Builds the docker image
	docker build . -t gcr.io/havulv/reflector:latest

image/local: #help: builds and tags a docker image destined for a local registry
	docker build . -t localhost:5000/havulv/reflector:latest \
		--build-arg COMMIT_HASH="$(shell git rev-parse --short HEAD)" \
		--build-arg SEMVER="$(shell git describe --tags --always --dirty)" \
		--build-arg COMMIT_DATE="$(shell git log -1 --format=%ci)"
	docker push localhost:5000/havulv/reflector:latest

.PHONY: docs
docs:
	mkdocs serve -f .config/mkdocs.yaml

.PHONY: docs/release
docs/release:
	mkdocs build --config-file .config/mkdocs.yaml

stop:
	docker stop `docker ps -aq` 2> /dev/null && docker rm `docker ps -aq` 2> /dev/null; :
