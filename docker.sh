#!/bin/sh
TARGET_HOST=$(printenv CONTAINER_HOST|sed 's-tcp://--g'|cut -f1 -d:)
REV=$(git describe --long --tags --match='v*' --dirty 2>/dev/null || echo dev)

if [ "x$1" == "xtest" ]; then
  docker rm -f testdb
  docker run -d --name testdb --rm -e POSTGRES_USER=sharedhostpath  -e POSTGRES_PASSWORD=sharedhostpath -p 5432:5432 postgres:12-alpine
  while ! nc -z $TARGET_HOST 5432; do
    sleep 0.5 # wait for 1/2 of the second before check again
  done
  docker build -f docker/test.Dockerfile -t kazimsarikaya/csi-sharedhostpath:test-$REV . ||exit 1
  docker run --name csi_tester --rm --privileged kazimsarikaya/csi-sharedhostpath:test-$REV -dataroot "/csi-data-dir/" -dsn "user=sharedhostpath password=sharedhostpath dbname=sharedhostpath port=5432 host=$TARGET_HOST sslmode=disable" -ginkgo.v 9 -v 9 -test.v 9 ||exit 1
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
