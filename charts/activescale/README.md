# Activescale Helm chart

The chart installs the Activescale adapter, `ActivescaleMetric` CRD, the three
default metric resources, RBAC, Service, and Custom Metrics APIService.

```bash
helm install activescale ./charts/activescale \
  --namespace ns-observability \
  --create-namespace \
  --set image.tag=<activescale-image-tag> \
  --set serving.secretName=<existing-tls-secret> \
  --set apiService.caBundle=<base64-ca-bundle>
```

The serving TLS Secret must exist in the release namespace unless
`serving.createSecret=true` is set with `serving.tlsCrt` and `serving.tlsKey`.

Every resource must set `spec.metricName`, which is the HPA and Custom Metrics
API name, `spec.envoyMetricPattern`, `spec.perPodAggregation: sum`, and a
`spec.filter` value of `none` or `connection_aggregate`. `metadata.name`
is only the Kubernetes resource name, so it can use Kubernetes-compatible
hyphens while preserving existing HPA names with underscores.
