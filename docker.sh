#!/bin/sh

need_helper="no"

if [ "x$(docker images -q kazimsarikaya/csi-sharedhostpath-|wc -l|tr -d ' ')" != "x2" ]; then
  need_helper="yes"
elif [ "x$1" == "xhelper" ] && [ "x$2" == "xforce" ]; then
  need_helper="yes"
fi

if [ "x$need_helper" == "xyes" ]; then
  docker build -f docker/builder.Dockerfile -t kazimsarikaya/csi-sharedhostpath-builder
  docker build -f docker/runner.Dockerfile -t kazimsarikaya/csi-sharedhostpath-runner
fi

if [ "x$1" != "xhelper" ]; then
  TARGET_HOST=$(printenv CONTAINER_HOST|sed 's-tcp://--g'|cut -f1 -d:)
  REV=$(git describe --long --tags --match='v*' --dirty 2>/dev/null || echo dev)

  if [ "x$1" == "xtest" ]; then
    docker stop testdb
    while [ "x$(docker ps -a -f "name=testdb" -q|wc -l|tr -d ' ')" != "x0" ]; do
      sleep 0.5
    done
    docker run -d --name testdb --rm -e POSTGRES_USER=sharedhostpath  -e POSTGRES_PASSWORD=sharedhostpath -p 5432:5432 postgres:12-alpine
    docker build -f docker/test.Dockerfile -t kazimsarikaya/csi-sharedhostpath:test-$REV . ||{ docker rm -f testdb; exit 1; }
    while ! nc -z $TARGET_HOST 5432; do
      sleep 0.5 # wait for 1/2 of the second before check again
    done
    docker run --name csi_tester -ti --rm --privileged kazimsarikaya/csi-sharedhostpath:test-$REV -test.coverprofile /dev/stdout -dataroot "/csi-data-dir/" -dsn "user=sharedhostpath password=sharedhostpath dbname=sharedhostpath port=5432 host=$TARGET_HOST sslmode=disable" -ginkgo.v 9 -v 9 -test.v 9 |tee tmp/docker-run.out ||exit 1
    docker rmi kazimsarikaya/csi-sharedhostpath:test-$REV
    docker stop testdb
    cov_start=$(grep -n "mode: set" tmp/docker-run.out |cut -d: -f1)
    cov_end=$(($(grep -n "coverage:" tmp/docker-run.out |cut -d: -f1) - 1))
    sed -n "${cov_start},${cov_end}p" tmp/docker-run.out > tmp/cover.out
    go tool cover -html tmp/cover.out -o tmp/cover.html
  else
    docker build -f docker/build.Dockerfile -t kazimsarikaya/csi-sharedhostpath:$REV . ||exit 1

    if [ "x$REV" != "xdev" ]; then
      docker tag kazimsarikaya/csi-sharedhostpath:$REV kazimsarikaya/csi-sharedhostpath:dev-latest
    fi

    if [ "x$1" == "xpush" ]; then
      docker push kazimsarikaya/csi-sharedhostpath:dev-latest
    fi
  fi
fi
