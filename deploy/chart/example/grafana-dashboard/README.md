# Harbor metrics and Grafana dashboard

[values.yaml](values.yaml) enables Harbor metrics, the exporter, a ServiceMonitor,
and the dashboard ConfigMap. It also enables PostgreSQL/pgx monitoring for 8gcr
builds that support it; standard Harbor does not expose those database metrics.

## Prerequisites

- An external PostgreSQL database. Replace the URL and database host in the values file.
- A `harbor-admin` Secret with `HARBOR_ADMIN_PASSWORD` and a `harbor-db` Secret with
  `POSTGRESQL_PASSWORD`, both in the Harbor namespace.
- Prometheus Operator and its ServiceMonitor CRD. Set `serviceMonitor.labels.release`
  to match your Prometheus selector, and allow it to discover monitors in the Harbor namespace.
- Grafana with a Prometheus datasource and a dashboard sidecar watching ConfigMaps
  labelled `grafana_dashboard: "1"` in the existing `monitoring` namespace.
  Configure the sidecar's folder annotation as `grafana_folder` to use the Harbor folder.

## Install

From the repository root, after adapting the values:

```sh
helm upgrade --install harbor deploy/chart --namespace harbor --create-namespace \
  -f deploy/chart/example/grafana-dashboard/values.yaml
```

The ConfigMap contains the chart's canonical `dashboards/harbor.json`. Enable its
provisioning on only one release per Grafana organization; the same dashboard can
display every scraped Harbor installation through its shared **Data source →
Cluster → Namespace** selectors.

All Harbor series must expose consistent `cluster` and `namespace` labels. Add a
`cluster` scrape label if your setup does not attach one, even with one datasource
per cluster. Both selectors are single-select without All and default to the first
available value alphabetically when no valid selection exists. Namespace refreshes
within the selected cluster, preserving an existing selection when still valid.

All rows are expanded. Runtime follows Overview, and Database includes an 8gcr
feature note explaining why pgx panels may show no data. See the
[dashboard guide](../../../../contrib/grafana-dashboard/README.md) for manual
import, panel coverage and editing instructions.
