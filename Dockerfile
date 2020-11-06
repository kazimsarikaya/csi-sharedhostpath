FROM golang:alpine as builder
RUN apk add make git build-base
COPY . /source
WORKDIR /source
RUN make

FROM alpine
COPY --from=builder /source/bin/csi-shp-driver /csi-shp-driver
ENTRYPOINT ["/csi-shp-driver"]
