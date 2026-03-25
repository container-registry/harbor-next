# Create a Harbor deployment on SUSE Rancher

## Supported versions

- Rancher manager >= 2.13.1
- RKE2 >= v1.34.3+rke2r1 (1b103f296ab20fac6b32951c9efe59d28a5ed79f)

## Deploy a database

We create a namespace where our database will be running:

```bash
kubectl create namespace my-db
```

In this guide we use local storage, so we install a storage provisioner:

```bash
kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.34/deploy/local-path-storage.yaml
```

Set it as default:

```bash
kubectl patch storageclass local-path -p '{"metadata": {"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
```

Alternatively, create a custom storage class:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: local
provisioner: rancher.io/local-path
reclaimPolicy: Delete
volumeBindingMode: WaitForFirstConsumer
```

> **Note:** Only one StorageClass should be marked as default. The `local-path` class above is already set as default in the previous step.

We deploy our external Postgresql database with the Bitnami Helm chart:

```bash
helm repo add bitnami https://charts.bitnami.com/bitnami
helm install postgresql bitnami/postgresql \
  --namespace my-db \
  --set auth.postgresPassword=test1234! \
  --set persistence.size=8Gi
```

Once installed, let us connect to it by forwarding the port 5432 to the host:

```bash
kubectl port-forward --namespace=my-db postgresql-0 5432:5432


Forwarding from 127.0.0.1:5432 -> 5432
Forwarding from [::1]:5432 -> 5432
```

Then we connect to our database from the host by entering the password given during installation:

```bash
psql -h localhost -U postgres -p 5432
Password for user postgres:
```

We create a `registry` database which will be used by Harbor:

```sql
postgres=# create database registry;
CREATE DATABASE
postgres=# exit
```

## Deploy Harbor next

We pull the Helm chart from the OCI repository. First, log in to the registry if authentication is required:

```bash
helm registry login 8gears.container-registry.com
```

We pull the chart using the OCI reference:

```bash
helm pull oci://8gears.container-registry.com/8gcr/charts/harbor-next
```

Decompress the downloaded chart:

```bash
tar xzvf harbor-*.tgz
```

We update `values.yaml` to set the hostname, port and credentials of our database we will use with Harbor:

```yaml
database:
  host: "postgresql.my-db.svc.cluster.local"
  port: 5432
  username: "postgres"
  password: "test1234!"
  database: registry
```

We set also `externalURL` and `harborAdminPassword`:

```yaml
externalURL: "https://harbor.example.com"
harborAdminPassword: "strong-password"
```

Finally we configure the ingress:

```yaml
ingress:
  core: harbor.example.com
  hosts:
    - host: harbor.example.com
  tls:
    - secretName: harbor-tls
      hosts:
        - harbor.example.com
```

The hostname chosen for our local deployment is `harbor.example.com`. To override our DNS resolver, we set a mapping to localhost in `/etc/hosts`:

```
127.0.0.1 harbor.example.com
```

We will now deploy Harbor:

```bash
helm upgrade --install test-1 . \
  --namespace my-container-registry \
  --create-namespace
```

After some time we now see all the pods and our installation running:

```bash
kubectl get pods -n my-container-registry
NAME                                        READY   STATUS    RESTARTS      AGE
test-1-harbor-core-754c57dd77-xqfkb         1/1     Running   0             61s
test-1-harbor-exporter-867746db74-5pgmn     1/1     Running   0             61s
test-1-harbor-jobservice-67bc99bd86-7bvlp   0/1     Running   3 (35s ago)   61s
test-1-harbor-portal-57f7894ff7-hfs8h       1/1     Running   0             60s
test-1-harbor-registry-5bd8d59648-9fqs5     1/1     Running   0             61s
test-1-harbor-scanner-trivy-0               1/1     Running   0             61s
test-1-valkey-59486f6977-nqb2z              1/1     Running   0             61s
```

Since RKE2 comes with an Nginx ingress deployed by default, we can access the portal from a browser at:

```
https://harbor.example.com
```

The default credentials are `admin` and `Harbor12345`.
