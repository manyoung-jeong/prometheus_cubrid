# cubrid_exporter (Go)

CUBRID 런타임 통계를 **PostgreSQL `pg_stat_*` 수준**으로, **Prometheus/Grafana가 그대로 포팅**할 수 있게 제공하는 **단일 정적 바이너리** 익스포터.

`postgres_exporter`·`node_exporter`·`mysqld_exporter` 와 **동일한 배포·연동 방식**(파일 1개 설치, `/metrics` 스크랩)을 목표로, 기존 Node.js PoC(`../../open_api`)를 Go(stdlib-only)로 포팅한 것이다. 메트릭 계약(`schema/metrics.v1.json`)은 두 구현이 공유한다.

## 왜 Go 단일 바이너리인가
- **연동(포팅) 호환성**은 구현 언어가 아니라 **출력 포맷(OpenMetrics)** 이 결정한다 → Prometheus는 언어를 보지 않고 `/metrics` 만 스크랩. 개발업체 작업은 `prometheus.yml` + PromQL + Grafana JSON 뿐(PostgreSQL과 동일).
- **배포·운영 일관성**은 **패키징**이 결정한다 → 생태계 표준 익스포터는 거의 다 Go 단일 정적 바이너리. CUBRID도 "파일 1개 복사 → systemd 등록"이면 현장에서 **CUBRID만 다른 설치 절차**가 되는 문제가 사라진다.
- **의존성 0 (stdlib-only)** → 보안 업데이트 추적 부담 최소, 재컴파일만으로 수년 유지.

## 설계 원칙
- **단일 소스:** `schema/metrics.v1.json` 을 `//go:embed` 로 바이너리에 포함 → `/metrics`·JSON 이 한 계약에서 생성(drift 없음).
- **2 출력구:** `/metrics`(OpenMetrics, Prometheus 주력) + `/v1/stat/*`(JSON, SI 커스텀·비-Prometheus 도구 보조).
- **스냅샷 캐시:** 백그라운드 주기 수집 → 캐시 서빙. 요청마다 `statdump` fork 안 함(DPM 소크에서 CPU 4% 검증 패턴).

## 구조
```
schema/metrics.v1.json   메트릭 계약(v1) — Node PoC 와 동일 파일(단일 소스)
schema.go                스키마 로더(embed) · allMetrics 평탄화
collector.go             수집기(mock + statdump 훅) · 스냅샷 캐시
statdump.go              `cubrid statdump` Num_* 파서(운영 연결용)
samples.go               스냅샷 → 메트릭 샘플 바인딩(samples.js 동등)
exposition.go            OpenMetrics 직렬화(exposition.js 동등)
jsonapi.go               JSON 빌더(jsonapi.js 동등)
main.go                  HTTP 서버 · 플래그 · Bearer 인증
exporter_test.go         단위 테스트(go test)
samples/metrics_sample.txt  /metrics 전체 샘플
```

## 빌드 · 실행
```bash
# 빌드(이 저장소는 go.work 가 있어 모듈 분리를 위해 GOWORK=off)
GOWORK=off GOTOOLCHAIN=local go build -o cubrid_exporter .

# 실행
./cubrid_exporter --port 9880 --source mock --dbs testdb,demodb
curl http://127.0.0.1:9880/metrics
curl 'http://127.0.0.1:9880/v1/stat/database?db=testdb'

# 토큰 보호(선택)
CUBRID_API_TOKEN=secret123 ./cubrid_exporter --port 9880

# 테스트
GOWORK=off GOTOOLCHAIN=local go test ./...
```

플래그: `--port`(9880) · `--source`(mock|statdump) · `--dbs`(CSV) · `--refresh`(ms, 5000).
`/healthz` 는 토큰과 무관하게 공개.

## 범위 (PostgreSQL 패리티)
8 그룹 · 40+ 메트릭 · 94 샘플(2 DB 기준): availability / database / activity / io / lock / bgwriter / ha / space.
`cubrid_deadlocks_total` 은 CUBRID 11.4 perfmon 에 전용 카운터가 없어 v1 제외(v1.1 후보).

## 운영 연결 (다음 단계)
`collector.go` 의 `collectOne(db)` 에서 `source=="statdump"` 경로를 구현:
`cubrid statdump -i <db>` 출력을 `parseStatdump()` 로 넘기고, broker status·heartbeat·spacedb 를
gauge/ha/space 에 채우면 mock → 실데이터로 전환된다. **메트릭 계약(스키마)·출력 포맷은 그대로.**
라이브 CUBRID 가 없으면 해당 DB 는 `cubrid_up=0` 으로 graceful degrade(서버는 계속 동작).
