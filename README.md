# Shared Host Path Plugin Driver

This is a **[Kubernetes CSI]** Plugin for kubernetes installation which each host has shared storage mount such as [NFS], [GLusterFS],[Lustre],...

Plugin manages creation of Persistent Volume Claims ([PVC]) and bounds them to the auto created folders or disk images on shared storage.

The main aim for the project is portability for volume claims between diffrenet backended kubernetes.

For storing metadata of plugin, plugin uses [PostgreSQL] as a database. For high availabiliy, database can be installed with replication. Because of replication The plugin also provides a [Patroni] deployment.

The plugin and patroni images are at docker hub as [kazimsarikaya/csi-sharedhostpath][dockerhubplugin] and [kazimsarikaya/patroni][dockerhubpatroni].

# Setup and Examples

Firstly determine which namespace you will deploy. Default is **storage**.

The installation of plugin requires a postgresql database. Any kind HA postgresql is installation is prefered.

## 1. Example Installation of PostgreSQL

The installation uses Patroni for postgresql replication. For patroni the project provides docker image. But other methods are accepted. The example [yaml](deploy/patroni-pg.yaml) is at [deploy](deploy) folder. The yaml creates required service account, rbac rules for patroni. Then creates 3 replica of patroni which will be deployed the master nodes (kuberentes installation has been assumed minimul three masters). Example yaml uses network share as hostpath for data volume. It can be installed on local disk. It's up to you.

Please update superuser password at patroni yaml. Default is: **postgres**. The master instance service name is **plugindb** in example deploy folder.

When databases are up, then create a user and database for plugin.

```
create user sharedhostpath with encrypted password 'sharedhostpath';
create database sharedhostpath with owner sharedhostpath;
```

Current plugin uses master as write write, however next releases will be uses slaves for queries.

## 2. Plugin Setup

You should determine your driver name. Default is **sharedhostpath.sanaldiyar.com**. Then prepare DSN of postgres. Default is **user=sharedhostpath password=sharedhostpath dbname=sharedhostpath port=5432 host=plugindb sslmode=disable**.

The other important configuration is mounting shared storage to the driver pod. The default location is **/csi-data-dir**. Don't forget configuring mount paths.

Then apply [driver info](deploy/csi-shp-driverinfo.yaml), [rbac](deploy/rbac.yaml) and [plugin](deploy/shp-plugin.yaml) to the kubernetes. The yamls will be create three replica of provisioner (controller) and a daemon set (node).

## 3. Examples

Inside [examples](examples/) folder, there is two type of example: **folder** and **disk**. Driver name while deploying plugin determines the prefix of parameters of storage classes. Default prefix is **sharedhostpath.sanaldiyar.com/**.

The parameter **type** defines how will storage created. **folder** means a regular folder at shared storage such as NFS. **disk** means a **spare** file which will be mounted as **raw** or **formatted fs**. The parameter **fsType** determines how will be a **disk** type formatted. xfs and ext4 is supoorted, however xfs recommended. A disk type may be mounted as **raw disk**, however folder couldnot.

Firstly apply storage classes. Then example pvc and pods.

# Notes

The project source is at [kazimsarikaya/csi-sharedhostpath](https://github.com/kazimsarikaya/csi-sharedhostpath)

[Kubernetes CSI]: https://kubernetes-csi.github.io
[NFS]: https://en.wikipedia.org/wiki/Network_File_System
[GLusterFS]: https://www.gluster.org
[Lustre]: https://www.lustre.org
[PVC]: https://v1-16.docs.kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims
[PostgreSQL]: https://www.postgresql.org
[Patroni]: https://github.com/zalando/patroni
[dockerhubplugin]: https://hub.docker.com/repository/docker/kazimsarikaya/csi-sharedhostpath
[dockerhubpatroni]: https://hub.docker.com/repository/docker/kazimsarikaya/patroni
