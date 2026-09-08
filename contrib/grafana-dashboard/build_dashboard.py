#!/usr/bin/env python3
"""Generate the public Harbor Grafana dashboard (harbor.json).

Usage: build_dashboard.py OUT.json [--title TITLE] [--uid UID] [--with-db]
"""
import json
import sys

DS = {"type": "prometheus", "uid": "${datasource}"}
NS = 'namespace=~"$namespace"'
SVC = 'service=~".*harbor.*"'
RI = "$__rate_interval"

_id = [0]


def nid():
    _id[0] += 1
    return _id[0]


def target(expr, legend="__auto", instant=False, fmt=None):
    t = {"datasource": DS, "editorMode": "code", "expr": expr, "legendFormat": legend, "refId": "A", "range": not instant, "instant": instant}
    if fmt:
        t["format"] = fmt
    return t


def targets(specs, **kw):
    out = []
    for i, (expr, legend) in enumerate(specs):
        t = target(expr, legend, **kw)
        t["refId"] = chr(ord("A") + i)
        out.append(t)
    return out


def thresholds(*steps):
    return {"mode": "absolute", "steps": [{"color": c, "value": v} for c, v in steps]}


def timeseries(title, specs, unit="short", desc="", stack=False, fill=10, w=8, h=7, legend_calcs=None, min0=True, extra_defaults=None, overrides=None, draw="line", legend_placement="bottom"):
    defaults = {
        "color": {"mode": "palette-classic"},
        "custom": {
            "drawStyle": draw, "lineInterpolation": "linear", "lineWidth": 1, "fillOpacity": fill, "gradientMode": "none",
            "showPoints": "never", "pointSize": 4, "spanNulls": False, "insertNulls": False, "axisPlacement": "auto",
            "axisLabel": "", "axisBorderShow": False, "axisCenteredZero": False, "axisColorMode": "text",
            "scaleDistribution": {"type": "linear"}, "thresholdsStyle": {"mode": "off"},
            "stacking": {"mode": "normal" if stack else "none", "group": "A"},
            "hideFrom": {"legend": False, "tooltip": False, "viz": False},
        },
        "unit": unit,
        "thresholds": thresholds(("green", None)),
    }
    if min0:
        defaults["min"] = 0
    if extra_defaults:
        defaults.update(extra_defaults)
    return {
        "id": nid(), "type": "timeseries", "title": title, "description": desc, "datasource": DS,
        "gridPos": {"w": w, "h": h},
        "fieldConfig": {"defaults": defaults, "overrides": overrides or []},
        "options": {
            "legend": {"displayMode": "list", "placement": legend_placement, "showLegend": True, "calcs": legend_calcs or []},
            "tooltip": {"mode": "multi", "sort": "desc", "hideZeros": False},
        },
        "targets": targets(specs),
    }


def stat(title, expr, unit="short", desc="", w=3, h=6, steps=(("green", None),), decimals=None, color_mode="value", graph="area", text=None, mappings=None):
    defaults = {"color": {"mode": "thresholds"}, "unit": unit, "thresholds": thresholds(*steps), "mappings": mappings or []}
    if decimals is not None:
        defaults["decimals"] = decimals
    p = {
        "id": nid(), "type": "stat", "title": title, "description": desc, "datasource": DS,
        "gridPos": {"w": w, "h": h},
        "fieldConfig": {"defaults": defaults, "overrides": []},
        "options": {
            "reduceOptions": {"calcs": ["lastNotNull"], "fields": "", "values": False},
            "orientation": "auto", "textMode": "value", "colorMode": color_mode, "graphMode": graph,
            "justifyMode": "auto", "wideLayout": True, "showPercentChange": False,
        },
        "targets": [target(expr, "")],
    }
    if text:
        p["options"]["text"] = text
    return p


def bargauge(title, expr, legend, unit, desc="", w=8, h=8, steps=(("green", None), ("orange", 70), ("red", 90)), max_=None):
    defaults = {"color": {"mode": "thresholds"}, "unit": unit, "thresholds": thresholds(*steps), "min": 0}
    if max_ is not None:
        defaults["max"] = max_
    return {
        "id": nid(), "type": "bargauge", "title": title, "description": desc, "datasource": DS,
        "gridPos": {"w": w, "h": h},
        "fieldConfig": {"defaults": defaults, "overrides": []},
        "options": {
            "reduceOptions": {"calcs": ["lastNotNull"], "fields": "", "values": False},
            "orientation": "horizontal", "displayMode": "gradient", "valueMode": "color", "namePlacement": "auto",
            "showUnfilled": True, "sizing": "auto", "minVizHeight": 16, "minVizWidth": 8, "maxVizHeight": 300,
            "legend": {"displayMode": "list", "placement": "bottom", "showLegend": False, "calcs": []},
        },
        "targets": [target(expr, legend, instant=True)],
    }


def piechart(title, expr, legend, unit, desc="", w=8, h=8):
    return {
        "id": nid(), "type": "piechart", "title": title, "description": desc, "datasource": DS,
        "gridPos": {"w": w, "h": h},
        "fieldConfig": {"defaults": {"color": {"mode": "palette-classic"}, "unit": unit, "custom": {"hideFrom": {"legend": False, "tooltip": False, "viz": False}}}, "overrides": []},
        "options": {
            "displayLabels": ["percent"], "pieType": "donut",
            "legend": {"displayMode": "table", "placement": "right", "showLegend": True, "values": ["value"]},
            "reduceOptions": {"calcs": ["lastNotNull"], "fields": "", "values": False},
            "sort": "desc", "tooltip": {"mode": "single", "sort": "none", "hideZeros": False},
        },
        "targets": [target(expr, legend, instant=True)],
    }


def table(title, expr, include, rename, desc="", w=8, h=6, sort_by=None, unit_overrides=None):
    tf = [
        {"id": "filterFieldsByName", "options": {"include": {"names": include}}},
        {"id": "organize", "options": {"excludeByName": {}, "indexByName": {n: i for i, n in enumerate(include)}, "renameByName": rename}},
    ]
    if sort_by:
        tf.append({"id": "sortBy", "options": {"fields": {}, "sort": [{"field": sort_by, "desc": True}]}})
    return {
        "id": nid(), "type": "table", "title": title, "description": desc, "datasource": DS,
        "gridPos": {"w": w, "h": h},
        "fieldConfig": {"defaults": {"custom": {"align": "left", "cellOptions": {"type": "auto"}, "inspect": False, "filterable": False}, "thresholds": thresholds(("green", None))}, "overrides": unit_overrides or []},
        "options": {"cellHeight": "sm", "showHeader": True, "footer": {"show": False, "reducer": ["sum"], "fields": ""}},
        "targets": [target(expr, "__auto", instant=True, fmt="table")],
        "transformations": tf,
    }


def state_timeline(title, expr, legend, desc="", w=6, h=6):
    return {
        "id": nid(), "type": "state-timeline", "title": title, "description": desc, "datasource": DS,
        "gridPos": {"w": w, "h": h},
        "fieldConfig": {"defaults": {
            "color": {"mode": "thresholds"}, "unit": "bool", "min": 0, "max": 1, "noValue": "-",
            "custom": {"fillOpacity": 70, "lineWidth": 0, "spanNulls": False, "insertNulls": False, "hideFrom": {"legend": False, "tooltip": False, "viz": False}},
            "thresholds": thresholds(("red", None), ("green", 1)),
            "mappings": [{"type": "value", "options": {"0": {"text": "down", "color": "red"}, "1": {"text": "up", "color": "green"}}}],
        }, "overrides": []},
        "options": {"mergeValues": True, "showValue": "never", "alignValue": "left", "rowHeight": 0.8,
                    "legend": {"displayMode": "list", "placement": "bottom", "showLegend": False},
                    "tooltip": {"mode": "single", "sort": "none", "hideZeros": False}},
        "targets": [target(expr, legend)],
    }


def row(title, collapsed=False):
    return {"id": nid(), "type": "row", "title": title, "collapsed": collapsed, "gridPos": {"w": 24, "h": 1}, "panels": []}


# ----------------------------------------------------------------------------
# Rows. Each row is a list of "lines"; each line is a list of panels whose widths sum to 24.
# ----------------------------------------------------------------------------

def overview():
    return "Overview", [
        [
            state_timeline("Components", f"harbor_up{{{NS}}}", "{{component}}", "Up/down per Harbor component as reported by the exporter's health probe.", w=5),
            table("Instance", f"harbor_system_info{{{NS}}}", ["namespace", "harbor_version", "auth_mode", "self_registration"],
                  {"namespace": "Namespace", "harbor_version": "Version", "auth_mode": "Auth", "self_registration": "Self-reg"},
                  "Version and auth configuration from harbor_system_info.", w=7),
            stat("Health", f"min(harbor_health{{{NS}}})", "short", "harbor_health: 1 healthy, 0 unhealthy (min across selected namespaces).", w=3,
                 steps=(("red", None), ("green", 1)), color_mode="background", graph="none", decimals=0,
                 mappings=[{"type": "value", "options": {"0": {"text": "unhealthy", "color": "red"}, "1": {"text": "healthy", "color": "green"}}}]),
            stat("Storage", f"sum(harbor_statistics_total_storage_consumption{{{NS}}})", "bytes", "Sum of project quota usage as tracked by Harbor (not the bucket size).", w=3, graph="none"),
            stat("Projects", f"sum(harbor_statistics_total_project_amount{{{NS}}})", "short", "Total projects.", w=3, graph="none", decimals=0),
            stat("Repositories", f"sum(harbor_statistics_total_repo_amount{{{NS}}})", "short", "Total repositories.", w=3, graph="none", decimals=0),
        ],
        [
            timeseries("Storage used", [(f"sum(harbor_statistics_total_storage_consumption{{{NS}}})", "used")], "bytes", "Quota-tracked storage over time.", fill=20),
            timeseries("Projects", [
                (f"sum(harbor_statistics_public_project_amount{{{NS}}})", "public"),
                (f"sum(harbor_statistics_private_project_amount{{{NS}}})", "private"),
            ], "short", "Public vs private project count.", stack=True, fill=30),
            timeseries("Artifacts by type", [(f"sum by (artifact_type) (harbor_project_artifact_total{{{NS}}})", "{{artifact_type}}")], "short",
                       "Artifact count per type (IMAGE, CHART, SBOM, ...), all projects.", stack=True, fill=30),
        ],
    ]


def core_api():
    core = f"harbor_core_http_request_total{{{NS}}}"
    return "Core API", [
        [
            timeseries("Requests by status class", [
                (f'sum(rate({core[:-1]}, code=~"2.."}}[{RI}]))', "2xx"),
                (f'sum(rate({core[:-1]}, code=~"3.."}}[{RI}]))', "3xx"),
                (f'sum(rate({core[:-1]}, code=~"4.."}}[{RI}]))', "4xx"),
                (f'sum(rate({core[:-1]}, code=~"5.."}}[{RI}]))', "5xx"),
            ], "reqps", "Core HTTP request rate grouped by status class.", stack=True, fill=40,
                overrides=[{"matcher": {"id": "byName", "options": n}, "properties": [{"id": "color", "value": {"mode": "fixed", "fixedColor": c}}]}
                           for n, c in (("2xx", "green"), ("3xx", "blue"), ("4xx", "orange"), ("5xx", "red"))]),
            timeseries("p90 latency by operation (top 10)", [(f'topk(10, max by (operation) (harbor_core_http_request_duration_seconds{{{NS}, quantile="0.9"}}))', "{{operation}}")], "s",
                       "90th percentile request duration per API operation, ten slowest (summary quantile reported by core).", fill=0),
            timeseries("In-flight requests", [(f"sum(harbor_core_http_inflight_requests{{{NS}}})", "in-flight")], "short", "Requests currently being handled by core.", w=4, fill=20),
            stat("Error rate", f'sum(rate({core[:-1]}, code=~"[45].."}}[{RI}])) / sum(rate({core}[{RI}])) * 100', "percent",
                 "Share of core requests answered with 4xx or 5xx. Includes expected 401 token handshakes and 404 HEAD probes.", w=4, h=7,
                 steps=(("green", None), ("orange", 20), ("red", 50)), decimals=1),
        ],
        [
            timeseries("Top 10 operations by errors (4xx+5xx)", [(f'topk(10, sum by (operation) (rate({core[:-1]}, code=~"[45].."}}[{RI}])))', "{{operation}}")], "reqps",
                       "Operations producing the most error responses.", fill=0),
            timeseries("Top 10 operations by 404", [(f'topk(10, sum by (operation, method) (rate({core[:-1]}, code="404"}}[{RI}])))', "{{method}} {{operation}}")], "reqps",
                       "404s by operation and method. HEAD manifest/blob 404s are normal client probing.", fill=0),
            timeseries("Server errors (5xx) by operation", [(f'sum by (operation, code) (rate({core[:-1]}, code=~"5.."}}[{RI}]))', "{{code}} {{operation}}")], "reqps",
                       "5xx responses only; anything here needs a look.", fill=20, stack=True),
        ],
    ]


def registry():
    req = f"registry_http_requests_total{{{NS}}}"
    return "Registry", [
        [
            timeseries("Requests by handler", [(f"sum by (handler, method) (rate({req}[{RI}]))", "{{method}} {{handler}}")], "reqps",
                       "Distribution registry request rate by handler and method (manifest, blob, blob_upload, tags, ...).", stack=True, fill=30),
            timeseries("p90 latency by handler", [(f"histogram_quantile(0.9, sum by (le, handler) (rate(registry_http_request_duration_seconds_bucket{{{NS}}}[{RI}])))", "{{handler}}")], "s",
                       "90th percentile registry request duration per handler.", fill=0),
            timeseries("In-flight requests", [(f"sum(registry_http_in_flight_requests{{{NS}}})", "in-flight")], "short", "Requests currently being handled by the registry.", w=4, fill=20),
            stat("Registry 5xx", f'sum(rate({req[:-1]}, code=~"5.."}}[{RI}])) / sum(rate({req}[{RI}])) * 100', "percent",
                 "Share of registry responses that are 5xx.", w=4, h=7, steps=(("green", None), ("orange", 1), ("red", 5)), decimals=2),
        ],
        [
            timeseries("Throughput", [
                (f"sum(rate(registry_http_response_size_bytes_sum{{{NS}}}[{RI}]))", "egress (responses)"),
                (f"sum(rate(registry_http_request_size_bytes_sum{{{NS}}}[{RI}]))", "ingress (requests)"),
            ], "Bps", "Bytes served and received by the registry. Blob GETs redirected to object storage (307) are not counted as egress.", fill=20),
            timeseries("Storage backend p90 latency", [(f"histogram_quantile(0.9, sum by (le, action) (rate(registry_storage_action_seconds_bucket{{{NS}}}[{RI}])))", "{{action}}")], "s",
                       "90th percentile latency of storage driver calls (Stat, GetContent, PutContent, Delete, List, Move, RedirectURL).", fill=0),
            timeseries("Storage backend calls", [(f"sum by (action) (rate(registry_storage_action_seconds_count{{{NS}}}[{RI}]))", "{{action}}")], "ops",
                       "Storage driver call rate by action.", stack=True, fill=30),
        ],
        [
            timeseries("Blob descriptor cache", [
                (f"sum(rate(registry_storage_cache_hits_total{{{NS}}}[{RI}])) / sum(rate(registry_storage_cache_requests_total{{{NS}}}[{RI}]))", "hit ratio"),
            ], "percentunit", "Hit ratio of the Redis blob-descriptor cache. Misses turn into Stat calls on the storage backend.", fill=20,
                extra_defaults={"max": 1}, w=6),
            timeseries("Cache errors", [(f"sum(rate(registry_storage_cache_errors_total{{{NS}}}[{RI}]))", "errors")], "ops",
                       "Blob-descriptor cache errors (Redis unreachable or timeouts).", fill=20, w=6),
            timeseries("Proxy cache", [
                (f"sum by (type) (rate(registry_proxy_hits_total{{{NS}}}[{RI}]))", "hit {{type}}"),
                (f"sum by (type) (rate(registry_proxy_misses_total{{{NS}}}[{RI}]))", "miss {{type}}"),
            ], "ops", "Proxy-cache project hits and misses by blob/manifest. Empty when no proxy-cache project is used.", stack=True, fill=30, w=12),
        ],
    ]


def jobs():
    tt = f"harbor_jobservice_task_total{{{NS}}}"
    return "Jobs", [
        [
            timeseries("Task rate by type", [(f'sum by (type) (rate({tt[:-1]}, status="success"}}[{RI}]))', "{{type}}")], "ops",
                       "Successfully finished jobservice tasks per second, by job type.", stack=True, fill=30),
            timeseries("Failed tasks by type", [(f'sum by (type) (rate({tt[:-1]}, status=~"fail|stop"}}[{RI}]))', "{{type}}")], "ops",
                       "Failed or stopped tasks per second, by job type.", stack=True, fill=30),
            timeseries("Queue size by type", [(f"sum by (type) (harbor_task_queue_size{{{NS}}})", "{{type}}")], "short",
                       "Pending tasks per job queue. A growing queue means workers cannot keep up.", stack=True, fill=30),
        ],
        [
            timeseries("Queue latency by type", [(f"max by (type) (harbor_task_queue_latency{{{NS}}})", "{{type}}")], "s",
                       "Age of the oldest pending task per queue.", fill=0),
            timeseries("p90 task duration by type", [(f'max by (type) (harbor_jobservice_task_process_time_seconds{{{NS}, quantile="0.9"}})', "{{type}}")], "s",
                       "90th percentile processing time per job type.", fill=0),
            timeseries("Running tasks by type", [(f"sum by (type) (harbor_task_concurrency{{{NS}}})", "{{type}}")], "short",
                       "Tasks currently executing per job type.", stack=True, fill=30),
        ],
        [
            stat("Scheduled jobs", f"sum(harbor_task_scheduled_total{{{NS}}})", "short", "Number of periodic schedules registered (scan-all, GC, retention, replication, ...).", w=4, h=6, graph="none", decimals=0),
            stat("Workers", f"sum(harbor_jobservice_info{{{NS}}})", "short", "Total jobservice worker slots across pools.", w=4, h=6, graph="none", decimals=0),
            table("Worker pools", f"harbor_jobservice_info{{{NS}}}", ["node", "pool", "workers"], {"node": "Node", "pool": "Pool", "workers": "Workers"},
                  "One row per jobservice replica.", w=16, h=6),
        ],
    ]


def projects():
    return "Projects", [
        [
            bargauge("Quota usage", f'topk(15, harbor_project_quota_usage_byte{{{NS}}} / (harbor_project_quota_byte{{{NS}}} > 0) * 100)', "{{project_name}}", "percent",
                     "Quota fill per project, top 15. Projects with unlimited quota (-1) are excluded.", max_=100),
            piechart("Storage by project", f"harbor_project_quota_usage_byte{{{NS}}} > 1048576", "{{project_name}}", "bytes",
                     "Quota-tracked storage per project (projects above 1 MiB)."),
            timeseries("Artifact pulls per minute by project", [(f"topk(10, sum by (project_name) (rate(harbor_artifact_pulled{{{NS}}}[5m])) * 60)", "{{project_name}}")], "short",
                       "Artifact pulls per minute per project (5m average), top 10.", fill=0),
        ],
        [
            timeseries("Repositories by project", [(f"topk(10, sum by (project_name) (harbor_project_repo_total{{{NS}}}))", "{{project_name}}")], "short", "Top 10 projects by repository count.", fill=0),
            timeseries("Artifacts by project", [(f"topk(10, sum by (project_name) (harbor_project_artifact_total{{{NS}}}))", "{{project_name}}")], "short", "Top 10 projects by artifact count (all types).", fill=0),
            timeseries("Members by project", [(f"topk(10, sum by (project_name) (harbor_project_member_total{{{NS}}}))", "{{project_name}}")], "short", "Top 10 projects by member count.", fill=0),
        ],
    ]


def runtime():
    sel = f"{SVC}, {NS}"
    return "Runtime", [
        [
            timeseries("Memory (RSS)", [(f"sum by (service, pod) (process_resident_memory_bytes{{{sel}}})", "{{pod}}")], "bytes", "Resident memory per Harbor process. This is what OOM-kills a pod.", w=6, fill=10),
            timeseries("CPU", [(f"sum by (service, pod) (rate(process_cpu_seconds_total{{{sel}}}[{RI}]))", "{{pod}}")], "percentunit", "CPU cores used per Harbor process.", w=6, fill=10),
            timeseries("Goroutines", [(f"sum by (service, pod) (go_goroutines{{{sel}}})", "{{pod}}")], "short", "Goroutines per process. A steady climb is a leak.", w=6, fill=0),
            timeseries("Open file descriptors", [(f"sum by (service, pod) (process_open_fds{{{sel}}})", "{{pod}}")], "short",
                       "Open file descriptors per process (sockets, files). Compare with process_max_fds, usually 1M on Kubernetes.", w=6, fill=0),
        ],
        [
            timeseries("Network receive", [(f"sum by (service, pod) (rate(process_network_receive_bytes_total{{{sel}}}[{RI}]))", "{{pod}}")], "Bps", "Bytes received per process.", fill=10),
            timeseries("Network transmit", [(f"sum by (service, pod) (rate(process_network_transmit_bytes_total{{{sel}}}[{RI}]))", "{{pod}}")], "Bps", "Bytes sent per process.", fill=10),
            timeseries("GC pause p50", [(f'max by (service, pod) (go_gc_duration_seconds{{{sel}, quantile="0.5"}})', "{{pod}}")], "s", "Median garbage-collection pause per process.", fill=0),
        ],
    ]


def database():
    """pgx pool + query metrics from the 8gcr pgx-monitoring patch (POSTGRESQL_METRICS_ENABLED=true)."""
    sel = f"{SVC}, {NS}"
    return "Database (pgx)", [
        [
            timeseries("Pool connections", [
                (f'sum by (service) (db_client_connections_usage{{{sel}, state="used"}})', "{{service}} used"),
                (f'sum by (service) (db_client_connections_usage{{{sel}, state="idle"}})', "{{service}} idle"),
                (f'max by (service) (db_client_connection_max{{{sel}}})', "{{service}} max"),
            ], "short", "pgx pool connections in use and idle per process, against the configured pool maximum.", fill=10),
            timeseries("Acquire wait", [
                (f"sum by (service) (rate(pgx_pool_acquire_wait_duration_seconds_total{{{sel}}}[{RI}]))", "{{service}} wait s/s"),
            ], "s", "Seconds per second spent waiting for a free pool connection. Non-zero means the pool is saturated.", fill=20),
            timeseries("Acquires waiting or canceled", [
                (f"sum by (service) (rate(pgx_pool_waited_for_acquires_total{{{sel}}}[{RI}]))", "{{service}} waited"),
                (f"sum by (service) (rate(pgx_pool_canceled_acquires_total{{{sel}}}[{RI}]))", "{{service}} canceled"),
            ], "ops", "Acquires that had to wait for the pool, and acquires canceled by context (request timeouts).", fill=10),
        ],
        [
            timeseries("Query latency", [
                (f"histogram_quantile(0.5, sum by (le) (rate(db_client_operation_duration_seconds_bucket{{{sel}}}[{RI}])))", "p50"),
                (f"histogram_quantile(0.9, sum by (le) (rate(db_client_operation_duration_seconds_bucket{{{sel}}}[{RI}])))", "p90"),
                (f"histogram_quantile(0.99, sum by (le) (rate(db_client_operation_duration_seconds_bucket{{{sel}}}[{RI}])))", "p99"),
            ], "s", "PostgreSQL operation latency across all Harbor processes.", fill=0),
            timeseries("Queries by type", [(f"sum by (pgx_operation_type) (rate(db_client_operation_duration_seconds_count{{{sel}}}[{RI}]))", "{{pgx_operation_type}}")], "ops",
                       "Database operations per second by type (query, batch, prepare, connect, copy_from).", stack=True, fill=30),
            timeseries("Query errors", [(f'sum by (pgx_status) (rate(db_client_operation_duration_seconds_count{{{sel}, pgx_status!="OK"}}[{RI}]))', "{{pgx_status}}")], "ops",
                       "Operations that returned an error, by PostgreSQL status.", fill=20),
        ],
        [
            timeseries("Mean acquire time", [(f"sum by (service) (rate(pgx_pool_acquire_duration_seconds_total{{{sel}}}[{RI}])) / sum by (service) (rate(pgx_pool_acquires_total{{{sel}}}[{RI}]))", "{{service}}")], "s",
                       "Average time to get a connection from the pool (total acquire duration / acquires).", fill=0),
            timeseries("Connection churn", [
                (f"sum by (service) (rate(pgx_pool_connections_created_total{{{sel}}}[{RI}]))", "{{service}} created"),
                (f"sum by (service, reason) (rate(pgx_pool_connections_destroyed_total{{{sel}}}[{RI}]))", "{{service}} destroyed {{reason}}"),
            ], "ops", "Pool connections opened and closed per second, with the close reason.", fill=10),
            timeseries("Pending connection requests", [(f"sum by (service) (db_client_connections_pending_requests{{{sel}}})", "{{service}}")], "short",
                       "Connections currently being constructed.", fill=20),
        ],
    ]


def build(title, uid, with_db=False):
    panels = []
    y = 0
    builders = [overview, core_api, registry, jobs, projects] + ([database] if with_db else []) + [runtime]
    for builder in builders:
        name, lines = builder()
        r = row(name, collapsed=(name == "Runtime"))
        r["gridPos"].update({"x": 0, "y": y})
        y += 1
        row_panels = []
        for line in lines:
            x = 0
            assert sum(p["gridPos"]["w"] for p in line) == 24, (name, [p["title"] for p in line])
            h = max(p["gridPos"]["h"] for p in line)
            for p in line:
                p["gridPos"].update({"x": x, "y": y, "h": h})
                x += p["gridPos"]["w"]
                row_panels.append(p)
            y += h
        if r["collapsed"]:
            r["panels"] = row_panels
            panels.append(r)
        else:
            panels.append(r)
            panels.extend(row_panels)

    return {
        "__inputs": [],
        "__elements": {},
        "__requires": [
            {"type": "grafana", "id": "grafana", "name": "Grafana", "version": "11.0.0"},
            {"type": "datasource", "id": "prometheus", "name": "Prometheus", "version": "1.0.0"},
        ] + [{"type": "panel", "id": t, "name": t, "version": ""} for t in ("bargauge", "piechart", "stat", "state-timeline", "table", "timeseries")],
        "annotations": {"list": [{"builtIn": 1, "datasource": {"type": "grafana", "uid": "-- Grafana --"}, "enable": True, "hide": True,
                                  "iconColor": "rgba(0, 211, 255, 1)", "name": "Annotations & Alerts", "type": "dashboard"}]},
        "description": "Harbor container registry overview: health, projects, storage, core API traffic, registry and storage backend, jobservice and per-project usage. Requires Harbor metrics enabled on core, jobservice and registry plus the Harbor exporter.",
        "editable": True,
        "fiscalYearStartMonth": 0,
        "graphTooltip": 1,
        "links": [],
        "panels": panels,
        "refresh": "1m",
        "schemaVersion": 39,
        "tags": ["harbor", "registry"],
        "templating": {"list": [
            {"type": "datasource", "name": "datasource", "label": "Data source", "query": "prometheus", "current": {}, "hide": 0, "includeAll": False, "multi": False, "options": [], "refresh": 1, "regex": "", "skipUrlSync": False},
            {"type": "query", "name": "namespace", "label": "Namespace", "datasource": DS,
             "definition": "label_values(harbor_up,namespace)", "query": {"qryType": 1, "query": "label_values(harbor_up,namespace)", "refId": "PrometheusVariableQueryEditor-VariableQuery"},
             "current": {"text": "All", "value": "$__all"}, "hide": 0, "includeAll": True, "allValue": ".*", "multi": False, "options": [], "refresh": 2, "regex": "", "sort": 1, "allowCustomValue": True},
        ]},
        "time": {"from": "now-6h", "to": "now"},
        "timepicker": {},
        "timezone": "browser",
        "title": title,
        "uid": uid,
        "weekStart": "",
    }


if __name__ == "__main__":
    args = sys.argv[1:]
    out = args[0]
    title = args[args.index("--title") + 1] if "--title" in args else "Harbor"
    uid = args[args.index("--uid") + 1] if "--uid" in args else "harbor-overview"
    d = build(title, uid, with_db="--with-db" in args)
    with open(out, "w") as f:
        json.dump(d, f, indent=2, ensure_ascii=False)
        f.write("\n")
    n = sum(1 for p in d["panels"] if p["type"] != "row") + sum(len(p["panels"]) for p in d["panels"] if p["type"] == "row")
    print(f"wrote {out}: {n} panels, {sum(1 for p in d['panels'] if p['type']=='row')} rows")
