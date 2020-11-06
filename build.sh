#!/bin/sh -e

REV=$(shell git describe --long --tags --match='v*' --dirty 2>/dev/null || git rev-list -n1 HEAD)
NOW=$(date +'%Y-%m-%d_%T')
go mod tidy
go mod vendor
go build -ldflags "-X main.version=$REV -X main.buildTime=$NOW"  -o ./bin/csi-shp-driver ./cmd/sharedhostpath
