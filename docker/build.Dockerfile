FROM kazimsarikaya/csi-sharedhostpath-builder as builder
COPY . /source
WORKDIR /source
RUN make build

FROM kazimsarikaya/csi-sharedhostpath-runner
COPY --from=builder /source/bin/csi-shp-driver /csi-shp-driver
ENTRYPOINT ["/csi-shp-driver"]
