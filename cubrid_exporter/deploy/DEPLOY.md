# cubrid_exporter 운영 배포 가이드

단일 정적 바이너리 + systemd + (선택)TLS/토큰. postgres_exporter와 동일한 배포 방식.

## 1. 설치
```bash
# 릴리스 tarball 풀기 (make release 로 생성: dist/)
tar xzf cubrid_exporter-1.1.0.linux-amd64.tar.gz
sudo cp cubrid_exporter-1.1.0.linux-amd64/cubrid_exporter /usr/local/bin/
sudo chmod +x /usr/local/bin/cubrid_exporter
```

익스포터는 **읽기 전용 CUBRID CLI**(statdump/spacedb/tranlist/heartbeat/paramdump/applyinfo)만 호출하므로,
로컬 CUBRID 서버에 접속 가능한 계정(보통 CUBRID 소유자)으로 실행해야 합니다.

## 2. systemd 등록
```bash
sudo cp deploy/cubrid_exporter.service /etc/systemd/system/
sudoedit /etc/systemd/system/cubrid_exporter.service   # User/CUBRID/--dbs 환경에 맞게 수정
sudo systemctl daemon-reload
sudo systemctl enable --now cubrid_exporter
systemctl status cubrid_exporter
curl -s http://127.0.0.1:9890/healthz
```

## 3. 인증 (Bearer 토큰, 선택)
```bash
# 유닛에 Environment=CUBRID_API_TOKEN=... 설정 시 /metrics·/v1/* 가 토큰 요구(/healthz 는 공개)
curl -H "Authorization: Bearer <token>" http://127.0.0.1:9890/metrics
```
Prometheus 쪽:
```yaml
scrape_configs:
  - job_name: cubrid
    authorization: { type: Bearer, credentials: <token> }
    static_configs: [ { targets: ['cubrid-host:9890'] } ]
```

## 4. TLS (HTTPS, 선택)
```bash
# 인증서 발급(사내 CA 권장; 예시는 자체서명)
sudo mkdir -p /etc/cubrid_exporter
sudo openssl req -x509 -newkey rsa:2048 -nodes -days 825 \
  -keyout /etc/cubrid_exporter/tls.key -out /etc/cubrid_exporter/tls.crt \
  -subj "/CN=cubrid-host"
# 유닛의 ExecStart 에 --tls-cert/--tls-key 추가(주석 참고) 후 재기동
sudo systemctl restart cubrid_exporter
```
Prometheus 쪽(https 스크랩):
```yaml
scrape_configs:
  - job_name: cubrid
    scheme: https
    tls_config: { insecure_skip_verify: true }   # 사내 CA 사용 시 ca_file 지정
    static_configs: [ { targets: ['cubrid-host:9890'] } ]
```

## 5. 방화벽
```bash
sudo firewall-cmd --permanent --add-port=9890/tcp   # 익스포터
sudo firewall-cmd --reload
# Prometheus(9090)·Grafana(3000)는 모니터링 서버에서 개방
```

## 6. Prometheus / Grafana (운영 권장)
- Prometheus·Grafana 는 **DB 호스트가 아닌 별도 모니터링 서버**에 설치.
- Prometheus: 공식 패키지/바이너리 + systemd, `rule_files` 로 알림(`cubrid_rules.yml`), Alertmanager 연동.
- Grafana: 리버스 프록시(nginx) 뒤에서 TLS 적용, 프로비저닝(`grafana-prov/`)으로 데이터소스·대시보드 코드 관리.
- 알림 규칙·대시보드 JSON 은 이 저장소의 `../../monitoring/` 에 포함.

## 7. 업그레이드
바이너리 교체 → `sudo systemctl restart cubrid_exporter`. 메트릭 계약(v1.1)은 이름/타입 불변(추가만) 이라 대시보드·알림 그대로 유지.
