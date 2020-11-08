#!/bin/sh
REV=$(git describe --long --tags --match='v*' --dirty 2>/dev/null || echo dev)

docker build -t kazimsarikaya/csi-sharedhostpath:$REV .

if [ "x$REV" != "xdev" ]; then
  docker tag kazimsarikaya/csi-sharedhostpath:$REV kazimsarikaya/csi-sharedhostpath:dev-latest
fi

if [ "x$1" == "xpush" ]; then
  docker push kazimsarikaya/csi-sharedhostpath:dev-latest
fi
