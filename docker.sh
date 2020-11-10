#!/bin/sh
REV=$(git describe --long --tags --match='v*' --dirty 2>/dev/null || echo dev)

if [ "x$1" == "xtest" ]; then
  docker build -f docker/test.Dockerfile -t kazimsarikaya/csi-sharedhostpath:test-$REV . ||exit 1
  docker run --privileged kazimsarikaya/csi-sharedhostpath:test-$REV -test.v 9
else
  docker build -f docker/build.Dockerfile -t kazimsarikaya/csi-sharedhostpath:$REV . ||exit 1

  if [ "x$REV" != "xdev" ]; then
    docker tag kazimsarikaya/csi-sharedhostpath:$REV kazimsarikaya/csi-sharedhostpath:dev-latest
  fi

  if [ "x$1" == "xpush" ]; then
    docker push kazimsarikaya/csi-sharedhostpath:dev-latest
  fi
fi
