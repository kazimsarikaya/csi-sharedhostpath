FROM kazimsarikaya/csi-sharedhostpath-builder as tester
COPY . /source
WORKDIR /source
RUN make build test

FROM kazimsarikaya/csi-sharedhostpath-runner
COPY --from=tester /source/sharedhostpath.test /sharedhostpath.test
ENTRYPOINT ["/sharedhostpath.test"]
