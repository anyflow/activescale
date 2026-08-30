# Activescale HPA vs CPU/Memory HPA 전체 테스트 요약

## Introduction

이 문서는 동일 sample 내 두 HPA를 비교한 TC1 13회의 scaling 지연, TC1 32회와 TC2 20회의 Scale-out Coverage, 이를 바탕으로 한 예상 resource utilization을 비교한다.

참고로, Scale-out Coverage는 이상적인 scale-out에 필요한 replica × 시간 면적 중 실제 replica가 충족한 비율이며, 100%에 가까울수록 부하 증가에 더 빠르고 충분하게 대응했음을 의미한다.

## Summary

### TC1: 순간적인 대규모 Spike 대응 검증

100 → 1,000 QPS 10배 증가 시나리오.

| HPA | Scaling 시작 지연<br>(13개 sample) | 최대 replica 도달 지연<br>(13개 sample) | Scale-out Coverage<br>(32개 sample) | 예상 resource utilization |
| --- | --- | --- | --- | --- |
| CPU/Memory HPA | 33초~∞<br>median 64초<br>2개 sample 실패 | 33초~∞<br>median 186초 | median 5.66%<br>12/32 sample에서 0.00% | 기준 30% |
| Activescale HPA | 3~34초<br>median 17초 | 19~262초<br>median 34초 | median 89.47%<br>32/32 sample에서 우세 | 추정 113.1%<br>median 상대 증가율 276.84% |

- **핵심 차이**: Scaling 시작 47초 단축, 최대 replica 도달 152초(2분 32초) 단축. 최대 replica 도달은 모든 비교에서 Activescale 우세.
- **두 HPA의 Spike 대응 차이를 보여주는 sample**
  - **Activescale HPA**: 3초 만에 scaling 시작, 25초 만에 근사 최대 Pod 도달, Spike 부하 수용.
  - **CPU/Memory HPA**: 1분 30초 후 scaling 시작, 3분 시점에도 근사 최대 Pod 미도달, 지연 중 장애 진행.

### TC2: 실사용형 Traffic 변동 대응 검증

50 QPS warmup → 200 → 600 → 400 → 800 → 100 QPS scale-in 시나리오.

| HPA | Scale-out Coverage (20개 sample) | 예상 resource utilization |
| --- | --- | --- |
| CPU/Memory HPA | median 84.50% | 기준 30% |
| Activescale HPA | median 96.30%, 20/20 sample에서 우세 | 추정 55.6%, median 상대 증가율 85.32% |

- **핵심 차이**: median Coverage 11.80%p 향상. TC2 sample에서 scale-out 감지 10초, scale-in 감지 71초 단축.
- **두 HPA의 traffic 변동 대응 차이를 보여주는 sample**
  - **Activescale HPA**: scale-out 감지/최대 도달 10/400초, scale-in 감지/최저 도달 8/139초, average/peak replica 9.2/18.
  - **CPU/Memory HPA**: scale-out 감지/최대 도달 20/410초, scale-in 감지/최저 도달 79/225초, average/peak replica 8.8/16.

## TC1: 순간적인 대규모 Spike 대응 검증

### 두 HPA의 Scaling 신속성 비교 — 13개 Sample

Activescale HPA의 일관된 조기 scaling 시작 및 최대 replica 도달 확인.

- **Scaling 시작 지연**
  - **Activescale HPA**: range 3~34초, median 17초.
  - **CPU/Memory HPA**: range 33초~∞, median 64초. 2회 scaling 실패.
  - **차이**: median 기준 47초 단축.
- **최대 replica 도달 지연**
  - **Activescale HPA**: range 19~262초, median 34초.
  - **CPU/Memory HPA**: range 33초~∞, median 186초.
  - **차이**: median 기준 152초(2분 32초) 단축, 모든 비교에서 Activescale 우세.

### Scale-out Coverage 전체 32회

- **CPU/Memory HPA Coverage**: min 0.00%, median 5.66%, max 63.54%. 12/32 sample에서 0.00%(사실상 scale-out 없음).
- **Activescale HPA Coverage**: min 75.88%, median 89.47%, max 97.86%. 32/32 sample에서 우세.

![TC1 Scale-out Coverage 분포](assets/tc1-scale-out-coverage.svg)

<div style="text-align: center;"><small>Figure 1. TC1 32개 sample의 Scale-out Coverage 분포.</small></div>

### 예상 Resource Utilization

- **CPU/Memory HPA resource utilization**: 기준 예시 30%.
- **Activescale HPA resource utilization**: 추정 113.1%.
- **상대 증가율**: min 110.91%, median 276.84%, max 839.35%.

### Spike 대응 차이 Sample

- **Activescale HPA**: 약 3초 만에 scaling 시작, 약 25초 만에 근사 최대 Pod 도달, Spike 부하 수용.
- **CPU/Memory HPA**: 약 1분 30초 후 scaling 시작, 3분 시점에도 근사 최대 Pod 미도달, 지연 중 장애 진행.

![TC1 sample의 Activescale HPA와 CPU/Memory HPA replica 변화](assets/scale-out-comparison.png)

<div style="text-align: center;"><small>Figure 2. 두 HPA의 Spike 대응 차이를 보여주는 TC1 단일 sample의 requested QPS와 replica 변화.</small></div>

## TC2: 실사용형 Traffic 변동 대응 검증

### 전체 20회 결과

- **CPU/Memory HPA overall Coverage**: min 78.62%, median 84.50%, max 87.95%.
- **Activescale HPA overall Coverage**: min 94.33%, median 96.30%, max 98.77%. 20/20 sample에서 우세.

![TC2 overall Scale-out Coverage 분포](assets/tc2-scale-out-coverage.svg)

<div style="text-align: center;"><small>Figure 3. TC2 20개 sample의 overall Scale-out Coverage 분포.</small></div>

### 예상 Resource Utilization

- **CPU/Memory HPA resource utilization**: 기준 예시 30%.
- **Activescale HPA resource utilization**: 추정 55.6%.
- **상대 증가율**: min 59.10%, median 85.32%, max 124.80%.

### Traffic 변동 대응 차이 Sample

- **Activescale HPA**: scale-out 감지/최대 도달 10/400초, scale-in 감지/최저 도달 8/139초, average/peak replica 9.2/18.
- **CPU/Memory HPA**: scale-out 감지/최대 도달 20/410초, scale-in 감지/최저 도달 79/225초, average/peak replica 8.8/16.

![TC2 sample의 Activescale HPA와 CPU/Memory HPA replica 변화](assets/tc2-scale-out-comparison.svg)

<div style="text-align: center;"><small>Figure 4. 두 HPA의 traffic 변동 대응 차이를 보여주는 TC2 sample의 200→600→400→800→100 QPS와 replica 변화.</small></div>

## 해석 범위

- TC1 scaling 지연의 ∞는 평가 시간 내 scaling 시작 또는 최대 replica 도달 실패를 의미.
- Coverage는 scale-in을 평가하지 않으며 traffic이 감소하는 구간은 제외.
- Target replica는 QPS와 replica의 선형 관계를 가정하므로 비선형 구간에서 왜곡 가능.
- Resource utilization은 동일 baseline load에서 필요한 사전 buffer 감소량을 기준으로 한 추정치이며 일반적인 성능 보장을 의미하지 않음.

## Appendix: 계산식

Scale-out Coverage는 이상적인 scale-out 면적 중 실제 replica가 충족한 면적의 비율임. 면적 단위는 replica × 시간이며 100%에 가까울수록 신속한 scale-out을 의미함.

### Coverage 계산식

```text
TC1:
C = (
      1
      - D / ∫[t0..t1] (R_target(t) - R_base) dt
    ) × 100
```

```text
TC2:
C_overall = (
              1
              - (D_1 + D_2) / (A_1 + A_2)
            ) × 100
```

- **D**: 이상적인 scale-out 중 실제로 충족하지 못한 replica × 시간 면적
- **A**: 이상적인 전체 scale-out replica × 시간 면적
- **R_target**: QPS와 replica의 선형 관계를 가정해 계산한 target replica 수

### 예상 Resource Utilization 증가율 계산식

```text
B_m = D_m / T_m

U = (
      (R_base + B_CM) / (R_base + B_AS)
      - 1
    ) × 100
```

- **B_m**: mode m의 미충족 면적을 평균 부족 replica 수로 환산한 등가 buffer
- **U**: 동일 baseline load에서 CPU/Memory HPA 대비 Activescale HPA의 예상 resource utilization 증가율
