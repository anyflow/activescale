## What Is Activescale and Why Is It Useful?

Activescale supports fast Kubernetes autoscaling by pushing per-Pod active-request and connection metrics from Envoy.

- It detects **traffic spikes** much faster than CPU- and memory-based HPA and scales out proactively.
- It substantially reduces the **resource buffer reserved for spikes, improving resource utilization**.
- As a result, services can react to abrupt traffic changes faster without excessive pre-provisioning.

The core mechanism uses Istio active-request and connection metrics (RPS \* latency, based on Little's Law) and removes collection latency. See [Motivation](motivation-en.md) for details.

The source code is available at <https://github.com/anyflow/activescale>.

## Where Can Activescale Be Used?

- It is available in all TCN regions and environments.
- It can be used alongside an existing CPU- and memory-based HPA.
- It applies only to services with Istio installed.

## Which Metric Should Be Used?

- Use `inbound_active_requests` for a typical HTTP service. Use `outbound_active_requests` for a service without inbound metrics, such as an ingress gateway.
- Configure CPU and memory metrics as well when scaling should also react to CPU or memory signals.
- Use `active_connections` when scaling based on connection count is more important.

## Configuration

The configuration is nearly identical to a conventional HPA (Horizontal Pod Autoscaler).

### Activescale Only

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: my-svc-hpa
  namespace: default
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: my-svc
  minReplicas: 2
  maxReplicas: 50
  metrics:
    - type: Pods
      pods:
        metric:
          name: inbound_active_requests # Or outbound_active_requests when no inbound metric is available, as with an ingress gateway
        target:
          type: AverageValue
          averageValue: "10"
```

### With Existing CPU and Memory Metrics

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: my-svc-hpa
  namespace: default
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: my-svc
  minReplicas: 2
  maxReplicas: 50
  metrics:
    - type: Pods
      pods:
        metric:
          name: inbound_active_requests
        target:
          type: AverageValue
          averageValue: "10"
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 60
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 70
```

### For Persistent-Connection-Based Scaling

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: my-svc-hpa
  namespace: default
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: my-svc
  minReplicas: 2
  maxReplicas: 50
  metrics:
    - type: Pods
      pods:
        metric:
          name: active_connections
        target:
          type: AverageValue
          averageValue: "20"
```

## Verifying Active Metrics

Query the Custom Metrics API directly with `kubectl` to verify that a workload's active metrics are exposed correctly. Use this command format:

```bash
kubectl get --raw \
  '/apis/custom.metrics.k8s.io/v1beta2/namespaces/<namespace>/pods/*/<metric-name>?labelSelector=<URL-encoded-label-selector>' \
  | jq
```

Set `<metric-name>` to one of `inbound_active_requests`, `outbound_active_requests`, or `active_connections`.

### Example

The following commands use the earlier `my-svc` example in the `default` namespace, where the Pod label is `app=my-svc`. The `=` in the label selector is URL-encoded as `%3D`.

```bash
# Inbound requests for an application or API service
kubectl get --raw \
  '/apis/custom.metrics.k8s.io/v1beta2/namespaces/default/pods/*/inbound_active_requests?labelSelector=app%3Dmy-svc' \
  | jq

# Outbound requests for a gateway or proxy
kubectl get --raw \
  '/apis/custom.metrics.k8s.io/v1beta2/namespaces/default/pods/*/outbound_active_requests?labelSelector=app%3Dmy-svc' \
  | jq

# Persistent connections
kubectl get --raw \
  '/apis/custom.metrics.k8s.io/v1beta2/namespaces/default/pods/*/active_connections?labelSelector=app%3Dmy-svc' \
  | jq
```

- A per-Pod `value` in `items` means the metric is exposed correctly.
- `value: 0` means metric exposure is working, but there are currently no in-flight requests or open connections.
- `HTTP 404 NotFound` means fresh Envoy telemetry is unavailable; it does not mean the value is zero.
