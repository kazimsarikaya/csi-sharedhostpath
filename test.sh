#!/bin/sh
rm -fr tmp/*
REV=$(shell git describe --long --tags --match='v*' --dirty 2>/dev/null || git rev-list -n1 HEAD)
NOW=$(date +'%Y-%m-%d_%T')
go mod tidy
go mod vendor
for pkg in `go list ./...`;
do
  go test -c $pkg -ldflags "-X main.version=$REV -X main.buildTime=$NOW"
done
for tf in $(find . -type f -name "*.test");
do
  ./$tf -test.v
  rm -f ./$tf
done
