# Activescale

## Features

- Envoy metrics sink (gRPC StreamMetrics) ingestion
- Pod-level `active_requests` and `active_connections` custom metrics for HPA
- Redis/Valkey storage with TTL
- Optional TLS for Redis (`REDIS_TLS`, `REDIS_CA_FILE`, `REDIS_TLS_INSECURE`)
- Custom Metrics API via kube-apiserver aggregation
- Kustomize base + environment overlays (`manifest/*`)
- ArgoCD ApplicationSet for multi-environment sync
- Klog-based logging with verbosity control (`LOG_VERBOSITY`)
- Periodic summary logs for Envoy ingest and API responses

## Architecture

```mermaid
graph LR
    PodA["📦 service pod A<br/>Istio/Envoy"]
    PodB["📦 service pod B<br/>Istio/Envoy"]
    PodC["📦 service pod C<br/>Istio/Envoy"]

    Agg(("⚙️ <b>activescale<b/><br/>(Stateless, HA)"))

    Redis[("🗄️ shared memory<br/>(e.g., Redis)")]

    KEDA{{"🚀 HPA<br/>(average computed by HPA)"}}

    PodA -."1. Push<br/>(5s delay)".-> Agg
    PodB -."1. Push<br/>(5s delay)".-> Agg
    PodC -."1. Push<br/>(5s delay)".-> Agg

    Agg ==>|2. update + TTL| Redis

    KEDA --"3. Query"--> Agg
    Redis -."4. metric value (per pod)".-> Agg
    Agg --"5. metric value (per pod)"--> KEDA

    style Redis fill:#E3ECF8,stroke:#6E8FB3,stroke-width:2px,color:#000
    style KEDA fill:#DDEFD8,stroke:#7DA67D,stroke-width:2px,color:#000
    style Agg fill:#E3ECFF,stroke:#6E8FB3,color:#000
    style PodA fill:#FAFAFA,stroke:#999
    style PodB fill:#FAFAFA,stroke:#999
	  style PodC fill:#FAFAFA,stroke:#999
```

## Envoy Metrics Message Shape

Activescale receives Envoy gRPC `StreamMetrics` messages. Each message contains a node identity and a list of metric families. A simplified shape:

```json
{
  "identifier": {
    "node": {
      "id": "sidecar~10.0.0.1~my-pod.my-ns~my-ns.svc.cluster.local",
      "metadata": {
        "NAME": "my-pod",
        "NAMESPACE": "my-ns"
      }
    }
  },
  "envoy_metrics": [
    {
      "name": "http.inbound_0.0.0.0_8080;.downstream_rq_active",
      "metric": [
        { "gauge": { "value": 3 } }
      ]
    },
    {
      "name": "listener.0.0.0.0_8080.downstream_cx_active",
      "metric": [
        { "gauge": { "value": 12 } }
      ]
    },
    {
      "name": "cluster.xds-grpc;.circuit_breakers.default.cx_pool_open",
      "metric": [
        { "gauge": { "value": 1 } }
      ]
    }
  ]
}
```

### Summary Counters Meaning

- `messages`: number of `StreamMetrics` messages received (one `Recv()` call).
- `stored_metrics`: number of metric writes stored in Redis for accepted `active_requests` and `active_connections` samples.
- `dropped_by_ids`: messages dropped because pod identity could not be extracted.
- `dropped_by_names`: metric families skipped because their name did not match the `active_requests` or `active_connections` rules.

`stored` does not necessarily equal `messages` because a message can contain multiple metric families or multiple samples, and `dropped_by_names` is counted per metric family, not per message.

## Debugging

Check Custom Metrics API is registered:
```bash
kubectl get --raw '/apis/custom.metrics.k8s.io/v1beta2'
```

Query a metric with a selector:
```bash
kubectl get --raw '/apis/custom.metrics.k8s.io/v1beta2/namespaces/<ns>/pods/*/active_requests?labelSelector=app=<app>'
```

Query active connections with a selector:
```bash
kubectl get --raw '/apis/custom.metrics.k8s.io/v1beta2/namespaces/<ns>/pods/*/active_connections?labelSelector=app=<app>'
```

Check activescale ingest logs:
```bash
kubectl logs -n ns-observability deploy/activescale | rg -n "stored active_requests|stored active_connections|skipping metric name|missing pod identity"
```

Enable debug logs and restart:
```bash
kubectl -n ns-observability set env deploy/activescale LOG_VERBOSITY=4
kubectl -n ns-observability rollout restart deploy/activescale
```

Confirm Envoy bootstrap includes the metrics service:
```bash
istioctl proxy-config bootstrap <pod> -n <ns> | rg -n "envoyMetricsService|metrics_service|envoy_grpc|cluster_name|activescale|9000"
```

## Notes

Activescale reads both Envoy HTTP connection manager and listener stats, depending on the metric.

- Activescale accepts only inbound-scoped `downstream_rq_active` metrics (e.g., `http.inbound_0.0.0.0_8080;.downstream_rq_active`).
- Prometheus-style `envoy_http_downstream_rq_active` is an alias across scopes; activescale intentionally reads only the inbound-scoped Envoy metric families.
- Admin/agent/outbound scopes are ignored by activescale auto-detect:
    - `http.admin.*`: Envoy admin interface (management) traffic
    - `http.agent.*`: Istio/Envoy internal agent traffic
    - `http.outbound_*`: outbound listener stats
- Activescale sums `listener.*.downstream_cx_active` into `active_connections`, but excludes known infrastructure listeners:
    - `15000`: Envoy admin port
    - `15020`: Istio merged metrics port
    - `15021`: Istio health/readiness port
    - `15090`: Envoy Prometheus metrics port
- These listener ports are excluded so `active_connections` reflects service traffic only, not admin, health, or telemetry scrapes.
