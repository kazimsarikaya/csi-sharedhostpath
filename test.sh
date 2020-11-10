#!/bin/sh
rm -fr tmp/*
if [ "x$1" == "xrun" ]; then
  docker rm -f testdb
  docker run -d --name testdb --rm -e POSTGRES_USER=sharedhostpath  -e POSTGRES_PASSWORD=sharedhostpath -p 5432:5432 postgres:12-alpine
fi

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
    ./$tf -dataroot "./tmp/" -dsn "user=sharedhostpath password=sharedhostpath dbname=sharedhostpath port=5432 host=192.168.99.114 sslmode=disable" -ginkgo.v 9 || exit 1
  done
  docker rm -f testdb
fi
