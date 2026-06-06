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
| `MAIL_PROVIDER` | optional | Email transport: `mailjet` \| `smtp` \| `log` \| blank (auto). Auto = Mailjet if its keys are set, else PocketBase SMTP if enabled, else a log-only sink. |
| `MAILJET_API_KEY` / `MAILJET_SECRET` | optional | Mailjet Send API credentials (needed when using the `mailjet` provider). |
| `MAIL_FROM` / `MAIL_FROM_NAME` | optional | Sender identity (must be a verified Mailjet sender). Falls back to PocketBase's configured sender. |
| `NOTIFY_CRON` | no | Override the notify scheduler cadence (default `*/15 * * * *`). |
| `NOTIFY_ALLOWLIST` | optional | Comma-separated emails for a gradual rollout — only these addresses get mail. Empty = everyone. |
| `NOTIFY_LOG_LEVEL` | no | `debug` logs a per-pass heartbeat; default logs only passes that sent/failed mail (plus allowlist changes & errors). |
| `VAPID_PUBLIC_KEY` / `VAPID_PRIVATE_KEY` | optional | Web Push keys. Auto-generated & stored in the DB if unset; pin them to keep keys stable across a `pb_data` wipe (regenerating invalidates existing subscriptions). |
| `VAPID_SUBJECT` | no | VAPID JWT contact (`mailto:` / `https:`). Defaults to `mailto:<sender address>`. |
| `PB_ADMIN_EMAIL` / `PB_ADMIN_PASSWORD` | optional | Convenience only — see superuser step below. |

**Notifications.** The app sends reminders before each stage kicks off, before
the Forecast locks, before untipped matches, and a daily results recap — over
**email** (when a mail provider is configured) and **Web Push** (when the user
enables it on a device). Each user manages these per-event and per-channel under
**Settings → Notifications**; push works on installed PWAs / supported browsers
(iOS requires the app be added to the Home Screen first). The reminder lead time (default **12h**), recap hour, and the
rollout allowlist are tunable at runtime via the `notify_config` row in the
`app_meta` collection (PocketBase dashboard), no redeploy needed. With no
provider set, emails are logged only (not delivered).

*Gradual rollout:* set `NOTIFY_ALLOWLIST` (or `notify_config.allowlist`, which
wins when set) to your own address plus a few friends to limit who receives mail
while you trial the feature; clear it to open delivery to all users. The list is
matched case-insensitively against each user's email.

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
