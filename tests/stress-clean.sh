#!/bin/sh

for ns in $(seq 1 10); do
cat <<EOF
---
apiVersion: v1
kind: Namespace
metadata:
  name: test-ns-$ns
EOF
done |   kubectl delete -f -
