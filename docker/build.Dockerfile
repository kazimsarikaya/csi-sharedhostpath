FROM golang:alpine as build-base
RUN apk add make git build-base

FROM build-base as source
COPY . /source
WORKDIR /source

FROM alpine:3.12 as runner
RUN apk add xfsprogs e2fsprogs util-linux && rm -fr /var/cache/apk/*

FROM source as builder
RUN make build

FROM runner
COPY --from=builder /source/bin/csi-shp-driver /csi-shp-driver
ENTRYPOINT ["/csi-shp-driver"]
