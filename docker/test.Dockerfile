FROM golang:alpine as builder
RUN apk add make git build-base
COPY . /source
WORKDIR /source
RUN make test

FROM alpine:3.12
RUN apk add xfsprogs e2fsprogs util-linux && rm -fr /var/cache/apk/*
COPY --from=builder /source/sharedhostpath.test /sharedhostpath.test
ENTRYPOINT ["/sharedhostpath.test"]
