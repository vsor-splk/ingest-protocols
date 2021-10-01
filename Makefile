SHELL                 = /bin/bash
BASE                  = $(CURDIR)
BIN                   = $(GOPATH)/bin
PKG                   = $(GOPATH)/pkg
CP_RF                 = cp -rf
RACE_PKG              = $(PKG)/darwin_amd64_race/github.com/signalfx
GOLANGCI_LINT_VERSION = 1.20.0
GOCOV                 = $(BIN)/gocov
.SHELLFLAGS           = -c # Run commands in a -c flag

# enable module support across all go commands.
export GO111MODULE = on

.SILENT: ;               # no need for @
.ONESHELL: ;             # recipes execute in same shell
.NOTPARALLEL: ;          # wait for this target to finish

# default is verification of sfxinternalgo which includes all services
.PHONY: verify
verify: install-tools lint test

# Tools that package is dependent on
.PHONY: install-tools
install-tools: golangci gocov
gocov: ; $(info $(M) downloading gocov)
	go install github.com/axw/gocov/gocov@v1.0.0
golangci: ; $(info $(M) downloading golangci)
	curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(shell go env GOPATH)/bin v${GOLANGCI_LINT_VERSION}

.PHONY: fmt
fmt: ; $(info $(M) running fmt on $(CURDIR)/cmd)
	cd $(CURDIR)/cmd ; go fmt ./...

.PHONY: lint
lint: ; $(info $(M) running linting on $(CURDIR))
	$(BIN)/golangci-lint run -c $(CURDIR)/.golangci.yml -v

ARGS                                        = -race -timeout=60s -failfast
COVERAGE_MODE                               = atomic
FULL_COVERAGE                               = 100.0
COVERAGE_DIR                                = $(CURDIR)/coverage.$(shell date -u +"%Y_%m_%dT%H_%M_%SZ")
COVERAGE_P_FILE                             = $(COVERAGE_DIR)/coverage/parallel/coverage.out
COVERAGE_S_FILE                             = $(COVERAGE_DIR)/coverage/serialized/coverage.out
COVERAGE_FILE                               = $(COVERAGE_DIR)/coverage/coverage.out

ALL_PKGS := $(shell go list ./... | grep -v etcdIntf | grep -v format)

.PHONY: test
test: ; $(info $(M) running test cases across all services in ingest-protocols)
		mkdir -p $(COVERAGE_DIR)/coverage/
		go test $(ARGS) -coverprofile=$(COVERAGE_FILE).all -covermode=$(COVERAGE_MODE) $(ALL_PKGS) || exit 2 ;\
		total_coverage=`$(GOCOV) convert $(COVERAGE_FILE).all | $(GOCOV) report | grep -i "total coverage"` ;\
		if [[ "$$total_coverage" != *'$(FULL_COVERAGE)'* ]]; then \
			$(GOCOV) convert $(COVERAGE_FILE).all | $(GOCOV) report ;\
			exit 2 ;\
		fi ;\

.PHONY: clean
clean: ; $(info $(M) cleaning test & pkg/cache for $(CURDIR))
	rm -rf $(CURDIR)/coverage.*

.PHONY: generate
generate:
	protoc --gofast_out=./protocol/signalfx/format/log protocol/signalfx/format/log/signalfx_log.proto
	grep proto.ProtoPackageIsVersion3 protocol/signalfx/format/log/signalfx_log.pb.go > /dev/null
