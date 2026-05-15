# Deploy

WM Pickems ships as **one self-contained Docker image**: the Go binary serves
the API and the embedded SvelteKit SPA from a single port, with SQLite data on
a mounted volume.

## 1. Configure

```sh
cp .env.example .env
```

| Var | Needed | Notes |
|-----|--------|-------|
| `HTTP_PORT` | no | Host port (default `8090`). |
| `API_FOOTBALL_KEY` | optional | Only used if it's a **paid** API-Football plan (the free tier has no WC2026 access). |
| `RESULTS_SOURCE` | no | `auto` (default): API-Football if its key reaches WC2026, else the free **openfootball** JSON. Force with `apifootball` / `openfootball`. Manual override always works. openfootball is community-updated (hours, not real-time). |
| `PB_ADMIN_EMAIL` / `PB_ADMIN_PASSWORD` | optional | Convenience only — see superuser step below. |

## 2. Run

```sh
docker compose up --build -d
```

App + API: `http://<host>:${HTTP_PORT}`. Data persists in the `pb_data`
Docker volume (SQLite DB, uploaded files, logs). First boot auto-runs
migrations and seeds 48 teams / 12 groups / 104 fixtures.

## 3. Create an admin (superuser)

The PocketBase admin UI (`/_/`) and the admin endpoints
(`/api/sync/refresh`, `/api/admin/matches/{id}/result`,
`/api/admin/recompute`) require a superuser:

```sh
docker compose exec app wm-pickems superuser create you@example.com 'a-strong-pass' --dir=/pb_data
```

## 4. Operating

- **Results**: synced every 30 min from the active source (openfootball by
  default, or a paid API-Football). Force one: `POST /api/sync/refresh`
  (superuser) — returns the source used.
- **Manual override / fix a result**: `POST /api/admin/matches/{id}/result`
  with `{ "FTHome":2, "FTAway":1, "Status":"finished" }` (also `ETHome/ETAway`,
  `PenHome/PenAway` for knockout). Scores recompute automatically.
- **Recompute everything** (after changing a scoring config):
  `POST /api/admin/recompute` (superuser).
- **Scoring config**: edit the `scoring_configs` "Default" record in `/_/`
  (or a per-League override) — no redeploy. Note: a config change (or a
  schema migration that rewrites it) does **not** retro-rescore matches that
  are already finished until you call `POST /api/admin/recompute` (or the
  next result comes in, which recomputes automatically).

## 5. Backup

The whole state is the volume. Snapshot it while running:

```sh
docker run --rm -v wm-pickems_pb_data:/d -v "$PWD":/b alpine \
  tar czf /b/pb_data-backup.tgz -C /d .
```

Restore by extracting back into the volume before `up`.

## 6. TLS / reverse proxy

Terminate TLS at a proxy (Caddy/Traefik/nginx) and forward to the container
port. Example Caddy:

```
pickems.example.com {
    reverse_proxy localhost:8090
}
```

## 7. Updating

```sh
git pull
docker compose up --build -d   # migrations run automatically on boot
```

## Health

`GET /api/health` returns 200 when up — use it for container/proxy health
checks.
