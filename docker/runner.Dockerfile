FROM alpine:3.15 as runner
RUN apk add xfsprogs-extra e2fsprogs-extra util-linux --no-cache
