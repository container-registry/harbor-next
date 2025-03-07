package main

import (
	"os"
	"strings"
)

const (
	GOLANGCILINT_VERSION = "v1.61.0"
	GO_VERSION           = "1.23.2"
	SYFT_VERSION         = "v1.9.0"
	GORELEASER_VERSION   = "v2.3.2"
	// version of registry for pulling the source code
	REGISTRY_SRC_TAG = "v2.8.3"
	// source of upstream distribution code
	DISTRIBUTION_SRC = "https://github.com/distribution/distribution.git"
	NPM_REGISTRY     = "https://registry.npmjs.org"
	// trivy-adapter
	TRIVYVERSION        = "v0.56.1"
	TRIVYADAPTERVERSION = "v0.32.0-rc.1"
	DEV_PLATFORM        = "linux/amd64"
	DEV_VERSION         = "dev"
	DEBUG               = true
	DEBUG_PORT          = "4001"
)

var (
	TRIVY_VERSION_NO_PREFIX    = strings.TrimPrefix(TRIVYVERSION, "v")
	TRIVY_DOWNLOAD_URL         = "https://github.com/aquasecurity/trivy/releases/download/" + TRIVYVERSION + "/trivy_" + TRIVY_VERSION_NO_PREFIX + "_Linux-64bit.tar.gz"
	TRIVY_ADAPTER_DOWNLOAD_URL = "https://github.com/goharbor/harbor-scanner-trivy/archive/refs/tags/" + TRIVYADAPTERVERSION + ".tar.gz"
	USER_HOME_DIR, _           = os.UserHomeDir()
)
