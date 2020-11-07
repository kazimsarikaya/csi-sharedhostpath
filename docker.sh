#!/bin/sh
REV=$(shell git describe --long --tags --match='v*' --dirty 2>/dev/null || echo dev)
docker build -t kazimsarikaya/csi-sharedhostpath:$REV .
if [ "x$1" == "xpush" ]; then
docker push kazimsarikaya/csi-sharedhostpath:$REV
fi
