FROM golang:alpine as builder
RUN apk add make git build-base
COPY . /source
WORKDIR /source
RUN make

FROM alpine:3.12
RUN apk add xfsprogs e2fsprogs util-linux && rm -fr /var/cache/apk/*
COPY --from=builder /source/bin/csi-shp-driver /csi-shp-driver
ENTRYPOINT ["/csi-shp-driver"]
