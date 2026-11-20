# cubrid_exporter — Go 포팅 · 테스트 결과

- 일자: 2026-11-20
- 위치: `prometheus_cubrid/cubrid_exporter/` (manyoung-jeong/prometheus_cubrid fork 내 자립형 Go 모듈)
- 원본: Node.js PoC `open_api/` (동일 메트릭 계약 `schema/metrics.v1.json` 공유)
- 목적: CUBRID 메트릭을 **PostgreSQL `pg_stat_*` 수준**으로, **Prometheus/Grafana가 그대로 포팅**할 수 있는 **단일 정적 바이너리** 익스포터로 제공

## 1. 빌드 환경
| 항목 | 값 |
|---|---|
| Go | go1.23.4 linux/amd64 (홈 로컬 설치 `~/go-sdk`, 루트 불필요) |
| 모듈 | `cubrid_exporter` (stdlib-only, 외부 의존성 **0**) |
| 부모 저장소 | Prometheus v3.16.0 fork (go.work 보유) → 모듈 분리 위해 `GOWORK=off GOTOOLCHAIN=local` |
| 소스 | 7개 `.go` (853 LOC) + 테스트 1개 + 스키마 1개 |
| gofmt / go vet | clean |

## 2. 산출 바이너리
| 빌드 | 크기 | 링크 |
|---|---|---|
| 기본 | 8.46 MB | dynamic |
| 배포본 `CGO_ENABLED=0 -ldflags='-s -w'` | **5.73 MB** | **static (`not a dynamic executable`)** |

→ `postgres_exporter`·`node_exporter`와 동일하게 **파일 1개 복사 → 실행**. 런타임·라이브러리 설치 불필요.

## 3. 단위 테스트 (`go test ./...`)
`ok cubrid_exporter` — 6개 케이스 통과:
- `TestSchemaLoads` — 계약 로드, 8그룹, 40+메트릭
- `TestExpositionShape` — 전 메트릭 HELP/TYPE 존재, 값 포맷(0.9375, 라벨 조합) 스팟체크
- `TestBlksHitDerivation` — `blks_hit = fetched − read` 파생 검증
- `TestJSONGroups` — JSON 8그룹 생성
- `TestParseStatdump` — statdump `Num_*` 파싱 + **미래 카운터도 수용(additions-only)**

## 4. Node PoC 동등성 검증 (핵심)
Go(:9881) · Node(:9882) 동시 기동, mock·동일 DB(testdb,demodb)로 `/metrics` 대조.

| 항목 | Node | Go | 판정 |
|---|---|---|---|
| 총 라인 | 182 | 182 | 일치 |
| 샘플 수(비주석) | 94 | 94 | 일치 |
| **구조 diff**(메트릭명+라벨+HELP/TYPE, 값 제외) | — | — | **완전 동일(diff 0줄)** |
| 값 포맷 `used_ratio` | 0.9375 | 0.9375 | 일치 |
| 값 포맷 `lock_wait_seconds`(반올림3) | 9.001 / 9.002 | 9.001 / 9.002 | 일치 |
| 세션 상태별(active/idle/waiting/total) | 8/40/1/49 | 8/40/1/49 | 일치 |
| HA info 라벨(node/role/state) | master/slave | master/slave | 일치 |

→ **동일 elapsed 기준 값까지 바이트 단위 일치.** 메트릭 계약·출력이 Node 구현과 등가.

## 5. 기능 검증
| 검증 | 결과 |
|---|---|
| `/metrics` OpenMetrics(0.0.4) | 94 샘플 정상 |
| 카운터 단조증가 → `rate()` | commits 증가 확인(rate 산출 가능) |
| `/v1/stat/<group>`·`/v1/stat/all` JSON | 유효 JSON, 8그룹 |
| `/v1/databases` | 목록 정상 |
| Bearer 인증(`CUBRID_API_TOKEN`) | 토큰없음 401 / 오토큰 401 / 정상 200 |
| `/healthz` | 토큰 무관 공개 200 |
| 다중 DB(3개) 확장 | 141 샘플(94/2×3) 정상 |
| statdump 모드(라이브 CUBRID 無) | 크래시 없이 `cubrid_up=0` graceful degrade |

## 6. 결론
- **개발업체 입장(포팅):** 언어 무관. `/metrics`(OpenMetrics) 스크랩 → PostgreSQL과 **동일한 작업**(prometheus.yml + PromQL + Grafana JSON). CUBRID만 별도 프로젝트 안 생김.
- **운영 입장(배포):** 생태계 표준과 동일한 **단일 정적 바이너리**. 설치 절차 이질성 제거.
- **유지보수:** stdlib-only(의존성 0) + 계약 동결 + 파서 additions-only → 사실상 재컴파일만으로 장기 유지.

## 7. 운영 statdump 실데이터 연결 (완료, 2026-11-20)
운영 CUBRID 환경(CUBRID 11.4.6)에 **읽기 전용 CLI**로 직접 연결.

| 소스 | 채운 그룹 |
|---|---|
| `cubrid statdump <db>@localhost` | database / io / lock / bgwriter (실제 Num_* 카운터, checkpoints=Num_log_end_checkpoints) |
| `cubrid spacedb -s` | space (total/free/used_ratio/data, 정수 바이트) |
| `cubrid tranlist -s` | activity(sessions_active) |
| `cubrid heartbeat status` | ha(node/role/state) — HA DB만 |

- 수집 주기 **15초**(부하 테스트 보호), 스냅샷 캐시로 요청당 fork 없음.
- 라이브 결과(4개 실DB 전부 `cubrid_up=1`, 146 샘플):

| db | commits(실측) | space_total | HA |
|---|---|---|---|
| testdb | 528,617 | 13.4 GB | master(node-primary) |
| demodb | 2,435,606 | 3.3 GB | master(node-primary) |
| db-app | 10,194,039 | 44.6 GB | (non-HA) |
| db-big | 168,901,291 | 149.7 GB | (non-HA) |

- 라이브 CUBRID 접속 실패 시 해당 DB `cubrid_up=0` graceful(서버 유지).
- 기동: `run/start-live.sh [port] [dbs]`. 현재 **cubrid-host:9890** 에서 가동 중.

## 8. v1.1 — 지표 완성 + 알림 + 대시보드 + 배포 (완료, 2026-11-20)

**① 지표 커버리지 완성 (PostgreSQL 수준 도달)**
- 추가 수집: `max_clients`(paramdump), `oldest_xact_seconds`·`lock_current_waiters`(tranlist), HA 복제지연 `ha_replication_delay_seconds`·`copy/apply_log_lag_pages`(applyinfo)
- 실측 불가분 제거(정직): `space_index_bytes`·`space_log_bytes`(현대 CUBRID 볼륨 미분리), `checkpoint_write_pages`(카운터 없음), `ha_last_applied_timestamp`(소스 없음)
- 결과: **40종 전부 실측값**(빈 지표 0). 예: oldest_xact db-big 22.4초, copy_log_lag demodb 20,541p

**② Prometheus 알림 규칙** (`monitoring/prometheus/cubrid_rules.yml`, 7종)
- CubridDown / HighStorageUsage / LowCacheHitRatio / HighRollbackRatio / LongRunningTransaction / LockWaiters / ReplicationLag
- 실발동 확인(PENDING): 공간 db-app 94%·db-big 92.7%, 롤백비율 testdb 48%·db-app 50%

**③ Grafana 대시보드 확정** (`monitoring/grafana-dashboards/cubrid.json`, 13패널)
- 한글화·임계 색상, 신규 패널(HA 복제지연·롤백 비율·최장 트랜잭션), `db` 변수 필터

**④ 운영 배포** 
- 익스포터 TLS 옵션(`--tls-cert/--tls-key`, HTTPS 200/평문 400 검증) + Bearer 토큰
- systemd 유닛(`deploy/cubrid_exporter.service`, 하드닝), 배포 가이드(`deploy/DEPLOY.md`)
- 릴리스 v1.1.0 멀티아키 tarball(amd64/arm64)

## 9. 다음 단계
1. **운영 statdump 연결** — `collector.go: collectOne()` 의 `source=="statdump"` 경로에 `cubrid statdump`/`broker status`/`heartbeat`/`spacedb` 연결(계약·포맷 불변).
2. **배포 산출물** — Makefile/`promu` 스타일 릴리스(멀티아키 `GOOS/GOARCH`), systemd unit 샘플.
3. **가이드북** — 기존 `open_api/docs/index.html`(외부 배포용)을 Go 엔드포인트 기준으로 동기화.
