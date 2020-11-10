#!/bin/sh

for ns in $(seq 1 10); do
  cat <<EOF
---
apiVersion: v1
kind: Namespace
metadata:
  name: test-ns-$ns
EOF
  for i in $(seq 1 20); do
    cat <<EOF
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: csi-pvc-folder-$i
  namespace: test-ns-$ns
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: csi-sharedhostpath-folder
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: csi-pvc-disk-$i
  namespace: test-ns-$ns
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: csi-sharedhostpath-disk
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: fio-tester-folder-$i
  namespace: test-ns-$ns
  labels:
    purpose: fio-testing-write
spec:
  strategy:
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 1
    type: RollingUpdate
  replicas: 1
  selector:
    matchLabels:
      app: fio-testing-folder-$i
  template:
    metadata:
      labels:
        app: fio-testing-folder-$i
    spec:
      containers:
      - name: fio-container-folder-$i
        image: wallnerryan/fiotools-aio
        ports:
        - containerPort: 8000
        volumeMounts:
        - name: fio-data
          mountPath: /myvol
        env:
          - name: REMOTEFILES
            value: "https://gist.githubusercontent.com/kazimsarikaya/5254c89ff28b0d3553030c29fdf5215e/raw/e3e754540fd1e5fea69897888d3f920c8681dd76/seqwrite.fio"
          - name: JOBFILES
            value: seqwrite.fio
          - name: PLOTNAME
            value: seqwrite
      volumes:
      - name: fio-data
        persistentVolumeClaim:
          claimName: csi-pvc-folder-$i
---
apiVersion: v1
kind: Service
metadata:
  name: fiotools-write-folder-$i
  namespace: test-ns-$ns
  labels:
    purpose: fiotools-write
spec:
  type: NodePort
  ports:
    - port: 8000
      targetPort: 8000
      name: http
  selector:
    app: fio-testing-folder-$i
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: fio-tester-disk-$i
  namespace: test-ns-$ns
  labels:
    purpose: fio-testing-write
spec:
  strategy:
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 1
    type: RollingUpdate
  replicas: 1
  selector:
    matchLabels:
      app: fio-testing-disk-$i
  template:
    metadata:
      labels:
        app: fio-testing-disk-$i
    spec:
      containers:
      - name: fio-container-disk-$i
        image: wallnerryan/fiotools-aio
        ports:
        - containerPort: 8000
        volumeMounts:
        - name: fio-data
          mountPath: /myvol
        env:
          - name: REMOTEFILES
            value: "https://gist.githubusercontent.com/kazimsarikaya/5254c89ff28b0d3553030c29fdf5215e/raw/e3e754540fd1e5fea69897888d3f920c8681dd76/seqwrite.fio"
          - name: JOBFILES
            value: seqwrite.fio
          - name: PLOTNAME
            value: seqwrite
      volumes:
      - name: fio-data
        persistentVolumeClaim:
          claimName: csi-pvc-disk-$i
---
apiVersion: v1
kind: Service
metadata:
  name: fiotools-write-disk-$i
  namespace: test-ns-$ns
  labels:
    purpose: fiotools-write
spec:
  type: NodePort
  ports:
    - port: 8000
      targetPort: 8000
      name: http
  selector:
    app: fio-testing-disk-$i
EOF
  done
done | kubectl apply -f -
