# Local Development

Full stack in one command: the exporter, Prometheus scraping it, and Grafana with the
dashboard already provisioned.

```bash
cd development
docker compose up -d --build
```

| Endpoint              | What                              |
| --------------------- | --------------------------------- |
| http://localhost:8081 | endoflife_exporter                |
| http://localhost:9091 | Prometheus                        |
| http://localhost:3001 | Grafana (`admin`/`admin`)         |

Grafana opens with anonymous admin access, so no login is needed.

## What is scraped

`prometheus.yml` has two jobs:

- `endoflife_exporter` — the exporter's own metrics (`/metrics`).
- `endoflife_products` — one `/probe` scrape per product. The target is the
  endoflife.date slug; the optional `releases` label picks the release cycles.

Add products by appending to `static_configs`:

```yaml
      - targets: ["postgres"]
        labels:
          releases: "17,16"
```

The dev config scrapes every 30s so the dashboard fills immediately. Real deployments
should use something like `168h`: end-of-life data changes rarely.

## Probe the exporter directly

```bash
curl 'http://localhost:8081/probe?target=redis'          # latest cycle only
curl 'http://localhost:8081/probe?target=mongo&releases=8.0,7.0'
curl 'http://localhost:8081/probe?target=ubuntu&releases=all'
```

## Dashboard

`../assets/endoflife-grafana-dashboard.json` is mounted into Grafana and reloaded from
disk. Edit that file, then restart Grafana:

```bash
docker compose restart grafana
```

Regenerate the README screenshot with Grafana's image renderer:

```bash
docker compose -f compose.yml -f render.yml up -d grafana renderer
curl -o ../assets/dashboard-screenshot.png \
  'http://localhost:3001/render/d/4bea73cf-7911-48e6-986b-c775a21a6bb2/x?from=now-6h&to=now&width=1600&height=1250&kiosk'
```

## Tear down

```bash
docker compose down          # keep Prometheus/Grafana data
docker compose down -v       # drop the volumes too
```
