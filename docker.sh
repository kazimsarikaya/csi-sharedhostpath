#!/bin/sh
REV=$(git describe --long --tags --match='v*' --dirty 2>/dev/null || echo dev)

if [ "x$1" == "xtest" ]; then
docker rm -f testdb
docker run -d --name testdb --rm -e POSTGRES_USER=sharedhostpath  -e POSTGRES_PASSWORD=sharedhostpath -p 5432:5432 postgres:12-alpine
  docker build -f docker/test.Dockerfile -t kazimsarikaya/csi-sharedhostpath:test-$REV . ||exit 1
  docker run --name csi_tester --rm --privileged kazimsarikaya/csi-sharedhostpath:test-$REV -dataroot "/csi-data-dir/" -dsn "user=sharedhostpath password=sharedhostpath dbname=sharedhostpath port=5432 host=192.168.99.114 sslmode=disable" -ginkgo.v 9 -v 9 -test.v 9 ||exit 1
  docker rmi kazimsarikaya/csi-sharedhostpath:test-$REV
  docker rm -f testdb
else
  docker build -f docker/build.Dockerfile -t kazimsarikaya/csi-sharedhostpath:$REV . ||exit 1

  if [ "x$REV" != "xdev" ]; then
    docker tag kazimsarikaya/csi-sharedhostpath:$REV kazimsarikaya/csi-sharedhostpath:dev-latest
  fi

  if [ "x$1" == "xpush" ]; then
    docker push kazimsarikaya/csi-sharedhostpath:dev-latest
  fi
fi
