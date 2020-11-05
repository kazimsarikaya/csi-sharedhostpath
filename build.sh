#!/bin/sh

REV=$(shell git describe --long --tags --match='v*' --dirty 2>/dev/null || git rev-list -n1 HEAD)
NOW=$(date +'%Y-%m-%d_%T')

go build -ldflags "-X main.sha1ver=$REV -X main.buildTime=$NOW"
