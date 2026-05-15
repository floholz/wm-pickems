# WM Pickems

World Cup 2026 prediction game for you and your friends. Predict every match,
predict the whole tournament up front, compare on a per-League leaderboard.

See [PLAN.md](PLAN.md) for the full design, scoring rules and roadmap.

## Stack

- **Backend:** Go + [PocketBase](https://pocketbase.io) (auth, SQLite, REST,
  scheduler) — used as a framework.
- **Frontend:** SvelteKit SPA (`adapter-static`), embedded into the Go binary.
- **Ship:** one multistage Docker image; SQLite data on a `pb_data` volume.

## Develop

```sh
make install        # frontend deps
make dev-backend    # PocketBase on http://127.0.0.1:8090 (admin UI at /_/)
make dev-frontend   # SvelteKit dev server (proxies /api to the backend)
```

## Build & run as a single binary

```sh
make run            # builds the SPA, embeds it, runs the binary
```

## Docker

```sh
cp .env.example .env   # set API_FOOTBALL_KEY etc.
docker compose up --build
```

App + API are served from one origin on port `8090`.
