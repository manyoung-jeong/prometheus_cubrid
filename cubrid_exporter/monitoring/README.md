# Monitoring stack for cubrid_exporter

Drop-in Prometheus + Grafana configuration. CUBRID → cubrid_exporter(OpenMetrics)
→ Prometheus → Grafana, the same way PostgreSQL uses postgres_exporter.

## Files
| File | Purpose |
|---|---|
| `prometheus.yml` | Prometheus scrape config (edit the `targets` host) + rule file |
| `cubrid_rules.yml` | 7 alert rules (down / storage / cache hit / rollback / long txn / lock waiters / replication lag) |
| `grafana-dashboards/cubrid.json` | CUBRID Overview dashboard (13 panels, `db` variable) |
| `grafana-prov/datasources/prometheus.yml` | Grafana Prometheus datasource (uid `cubrid-prom`) |
| `grafana-prov/dashboards/provider.yml` | Grafana dashboard file provider |

## Quick start
```bash
# 1) start the exporter against your CUBRID (read-only)
./cubrid_exporter --port 9890 --source statdump --dbs testdb,demodb --refresh 15000

# 2) Prometheus (edit prometheus.yml targets first)
prometheus --config.file=prometheus.yml

# 3) Grafana: copy grafana-prov/* into Grafana's provisioning dir and
#    grafana-dashboards/ into the path set in provider.yml, then start Grafana.
```

Open Grafana → dashboard **CUBRID Overview**. The datasource expects Prometheus
at `http://127.0.0.1:9090` (edit `grafana-prov/datasources/prometheus.yml` if different).

> The dashboard and rules are datasource-agnostic (PromQL on `cubrid_*` metrics),
> so any Prometheus that scrapes the exporter works.

## Full stack (companion repos)
This repo covers the CUBRID DB exporter + Prometheus/Grafana config. The other two
pieces of the standard stack are used as-is (upstream binaries) and documented in:

| Component | Repo (branch) | Role |
|---|---|---|
| node_exporter | `manyoung-jeong/node_exporter_cubrid` (`cubrid-stack`) | server OS metrics |
| Alertmanager | `manyoung-jeong/alertmanager_cubrid` (`cubrid-stack`) | alert routing/notification |

`prometheus.yml` here already includes the `node` scrape job and the `alerting:`
block — point them at your hosts.
