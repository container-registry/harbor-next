# Create a Harbor deployment on SUSE Rancher

A ready-made values file for this scenario lives at
[`example/rke2-rancher/values.yaml`](../../example/rke2-rancher/values.yaml)
(also shipped inside the chart package). This guide walks through the
cluster preparation around it and explains the values that matter.

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
kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.35/deploy/local-path-storage.yaml
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
cd harbor-next
```

The chart ships an RKE2/Rancher values file at
`example/rke2-rancher/values.yaml`. Copy it and adjust the handful of
values that are specific to your installation:

- **`externalURL`** and the `ingress` hostnames — the example uses
  `harbor.example.com` throughout. RKE2 ships an Nginx ingress controller
  by default; the example's annotations are tuned for it, most importantly
  `nginx.ingress.kubernetes.io/proxy-body-size: "0"`, without which large
  image pushes fail at the ingress.
- **`harborAdminPassword`** — required, no default. Use
  `existingSecretAdminPassword` for production.
- **`database`** — pre-wired to the Bitnami PostgreSQL deployed above
  (`postgresql.my-db.svc.cluster.local`, user `postgres`, database
  `registry`). Set `database.password` to the password you chose during
  the database installation (`test1234!` in this walkthrough), or better,
  reference a Secret via `database.existingSecret`.
- **TLS** — the example sets `tls.certSource: auto`, so the chart
  generates its own certificates; the ingress serves them from the
  `harbor-tls` secret. Replace with real certificates for anything
  internet-facing.
- **Persistence** — registry persistence is enabled with a 10Gi PVC on
  the default storage class (the `local-path` provisioner installed
  above).

```bash
cp example/rke2-rancher/values.yaml my-values.yaml
# edit my-values.yaml
```

The hostname chosen for our local deployment is `harbor.example.com`. To override our DNS resolver, we set a mapping to localhost in `/etc/hosts`:

```
127.0.0.1 harbor.example.com
```

We will now deploy Harbor:

```bash
helm upgrade --install test-1 . \
  -f my-values.yaml \
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

Log in with username `admin` and the password you set in `harborAdminPassword`.

> **⚠️ Security:** If you skip `harborAdminPassword`, the install fails —
> there is no default. Never reuse the legacy chart's well-known
> `Harbor12345` outside throwaway environments. Rotate the password from
> the Harbor UI (User Profile → Change Password) after first login.
