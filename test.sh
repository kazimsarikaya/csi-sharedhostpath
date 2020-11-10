#!/bin/sh
rm -fr tmp/*
find . -maxdepth 1 -name "*.test" -delete
REV=$(git describe --long --tags --match='v*' --dirty 2>/dev/null || git rev-list -n1 HEAD)
NOW=$(date +'%Y-%m-%d_%T')
go mod tidy
go mod vendor
for pkg in `go list ./...`;
do
  go test -c $pkg
done

if [ "x$1" == "xrun" ]; then
  for tf in $(find . -type f -name "*.test");
  do
    ./$tf -test.v -v 9 || exit 1
  done
fi
