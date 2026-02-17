#!/bin/bash
set -x

set -e

# Load version configuration
source "$(dirname "$0")/../../versions.env"

sudo make package_online GOBUILDTAGS="include_oss include_gcs" VERSIONTAG=dev-gitaction PKGVERSIONTAG=dev-gitaction UIVERSIONTAG=dev-gitaction GOBUILDIMAGE=golang:${GO_VERSION} COMPILETAG=compile_golangimage TRIVYFLAG=true EXPORTERFLAG=true HTTPPROXY= PULL_BASE_FROM_DOCKERHUB=false
sudo make package_offline GOBUILDTAGS="include_oss include_gcs" VERSIONTAG=dev-gitaction PKGVERSIONTAG=dev-gitaction UIVERSIONTAG=dev-gitaction GOBUILDIMAGE=golang:${GO_VERSION} COMPILETAG=compile_golangimage TRIVYFLAG=true EXPORTERFLAG=true HTTPPROXY= PULL_BASE_FROM_DOCKERHUB=false
