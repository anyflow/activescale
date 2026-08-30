## Activescale은 무엇인가, 이 왜 좋은가

ActiveScale은 Envoy에서 제공하는 Pod별 활성 요청 및 연결 메트릭을 푸시 기반으로 전송하여 빠른 Kubernetes 자동 스케일링을 지원함.

- **Spike, 즉 요청 급증**을 기존 CPU, Memory HPA보다 **훨씬 빠르게 감지**하여 선제적으로 scale-out을 이룸.
- Spike 대응을 위해 미리 크게 잡아두던 **resource buffer를 대폭 줄여 리소스 사용 효율성(utilization)을 높임.**
- 결과적으로 과도한 선제 증설 없이도 급격한 트래픽 변화에 더 빠르게 반응 가능.

HOW의 핵심은 Istio의 active request/connection 메트릭(RPS \* latency; 리틀의 법칙에 근거) 사용과 수집 지연 제거임. 상세 내용은 [Motivation](motivation-kr.md) 참고.

Source code는 <https://github.com/anyflow/activescale>에서 확인 가능.

## Activescale을 어디에 쓸 수 있나

- TCN 전 권역, 전 형상에서 사용 가능.
- 기존 CPU + Memory HPA와 병행 사용 가능.
- Istio가 설치된 서비스에만 적용 가능.

## 언제 어떤 옵션을 쓰나

- 일반적인 HTTP 기반 서비스는 `inbound_active_requests` 사용(하기 설정 방법 참고). ingress-gateway 등의 inbound metric이 없는 서비스에는 `outbound_active_requests` 사용.
- Activescale뿐 아니라 CPU, Memory 신호에 의해서도 scaling하려면 CPU, Memory도 함께 설정.
- 연결 수 기반 제어가 더 중요한 서비스에는 `active_connections` 사용.

## 설정 방법은?

기존 HPA(Horizontal Pod Autoscaler) 설정과 큰 차이 없음.

### Activescale만 사용하는 경우

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
          name: inbound_active_requests # 또는 outbound_active_requests (ingress-gateway 등의 inbound metric이 없는 경우)
        target:
          type: AverageValue
          averageValue: "10"
```

### 기존 CPU + Memory와 함께 사용하는 경우

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

### Persistent connection 기반의 scaling이 필요한 경우

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

## active metric 정상 동작 여부 확인 방법

kubectl을 통해 Custom Metrics API를 직접 조회하여 workload의 active metric 정상 노출 여부 확인 가능. 다음은 명령 형식임.

```bash
kubectl get --raw \
  '/apis/custom.metrics.k8s.io/v1beta2/namespaces/<namespace>/pods/*/<metric-name>?labelSelector=<URL-encoded-label-selector>' \
  | jq
```

`<metric-name>`에는 `inbound_active_requests`, `outbound_active_requests`, `active_connections` 중 확인할 metric 입력.

### 예제

앞선 `default` namespace의 `my-svc` 예제에서 Pod label이 `app=my-svc`인 경우임. Label selector의 `=`는 URL에서 `%3D`로 인코딩함.

```bash
# Application/API service의 inbound request
kubectl get --raw \
  '/apis/custom.metrics.k8s.io/v1beta2/namespaces/default/pods/*/inbound_active_requests?labelSelector=app%3Dmy-svc' \
  | jq

# Gateway/Proxy의 outbound request
kubectl get --raw \
  '/apis/custom.metrics.k8s.io/v1beta2/namespaces/default/pods/*/outbound_active_requests?labelSelector=app%3Dmy-svc' \
  | jq

# Persistent connection
kubectl get --raw \
  '/apis/custom.metrics.k8s.io/v1beta2/namespaces/default/pods/*/active_connections?labelSelector=app%3Dmy-svc' \
  | jq
```

- `items`에 Pod별 `value`가 반환되면 metric이 정상 노출되는 상태임.
- `value: 0`은 metric 노출은 정상이지만 현재 처리 중인 request 또는 열린 connection이 없음을 의미함.
- `HTTP 404 NotFound`는 값 0이 아니라 fresh Envoy telemetry가 없음을 의미함.
