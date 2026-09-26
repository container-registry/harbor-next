# Using the Makes Makefile framework
R := https://github.com/makeplus/makes
M := $(or $(MAKES_REPO_DIR),.cache/makes)
P := dff1c42f1c4865cf3535b8171fbaa0bbf27df5ce
$(shell [ -d '$M' ] || (git clone -q $R '$M' && git -C '$M' checkout -q $P))
ifneq ($(shell git -C $M rev-parse HEAD 2>/dev/null),$P)
$(error Makes commit mismatch: expected $P)
endif

.DEFAULT_GOAL := default

include $M/init.mk
include versions.env

GO-VERSION := $(GO_VERSION)
BUN-VERSION := $(BUN_VERSION)
NODE-VERSION := 22.23.2

ifeq (command line,$(origin TASK))
TASK-NAME := $(TASK)
override undefine TASK
endif

include $M/task.mk
include $M/git.mk
include $M/go.mk
# Task probes Go eagerly, before target prerequisites are installed.
# Let each selected Go executable determine its own installation root.
unexport GOROOT
include $M/bun.mk
include $M/node.mk
include $M/docker-compose.mk
include $M/docker-or-podman.mk
include $M/shell.mk

TASK-TARGETS := setup build test lint images info

GO-TASKS := setup build test lint images

TASK-ARGS = $(if $(strip $(ARGS)),-- $(ARGS))
TASK-GOAL = $(or $(TASK-NAME),default)
DEFAULT-DEPS = $(if $(TASK-NAME),\
  $(TASK),$(DOCKER-COMPOSE) $(TASK) $(GO) $(BUN) $(NODE) $(DOCKER))

.PHONY: default $(TASK-TARGETS) clean

default:: $(DEFAULT-DEPS)
	$(TASK) $(TASK-GOAL) $(TASK-ARGS)

$(TASK-TARGETS): $(TASK)
	$(TASK) $@ $(TASK-ARGS)

$(GO-TASKS): $(GO)
lint: $(DOCKER)
images: $(DOCKER-OR-PODMAN)

ifneq (,$(wildcard $(TASK)))
clean::
	@$(TASK) $@ $(TASK-ARGS) || true
endif

MAKES-CLEAN := \
  .task \
  bin \
  build-errors.log \
  devenv/token_service_key.pem \
  deploy/compose/config/token_service_key.pem \
  deploy/chart/.rendered-config.yaml \
  deploy/chart/charts \
  deploy/chart/*.tgz \
  golangci-lint.report \
  govulncheck.sarif \
  kubescape.sarif \
  src/coverage.html \
  src/coverage.out \
  src/coverage-pure.out \
  src/coverage-db.out \
  src/portal/.angular \
  src/portal/ng-swagger-gen \
  src/server/v2.0/models \
  src/server/v2.0/restapi \
  trivy-chart.sarif \
  tmp \
  *.tgz \

MAKES-REALCLEAN := \
  src/portal/node_modules \
  src/portal/src/swagger.json \

include $M/clean.mk
