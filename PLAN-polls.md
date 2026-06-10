# Poll Feature — Implementation Plan

> Status: planned, not yet implemented. Written 2026-06-10.

## Context

Add an admin-triggered **Poll** feature, managed from the Announcements page. Poll cards appear inline in the banner area (same stack as announcements, similar visual style) on all pages. Question types: yes/no, single-choice, multiple-choice, and integer slider with settable range. Each poll has an end time; voting is **one-shot** (no changes — avoids influence from live results) and users only see **live results after submitting** (realtime via PocketBase SSE). Raw per-user votes are stored in a `poll_votes` collection so they can be exported from the PB dashboard; in-app everyone sees only aggregates.

Future (not now): a multi-question Survey feature with its own page and free-text answers. Naming (`polls`/`poll_votes`) deliberately leaves room for later `surveys`/`survey_responses`.

## Key design decisions

1. **Aggregates live on the poll record** (`results` JSON field), updated server-side in the vote endpoint. Users subscribe to the `polls` collection via PB SSE — every save of the poll record (vote, close, create) broadcasts. Raw votes are never client-readable (no rules on `poll_votes`).
2. **Results hiding pre-vote is endpoint + client-side**, not rule-enforced (PB has no field-level ACL). `GET /api/polls/active` omits `results` for open polls the requester hasn't voted in; the SSE payload technically carries `results` but the UI only renders them post-vote/post-close. Acceptable for this audience; escape hatch (separate `poll_results` collection with back-relation view rule) documented but not built.
3. **End-time enforcement is lazy (authoritative) + cron (cosmetic).** Vote endpoint rejects when `closed || clock.Now(app).After(endsAt)` (uses `internal/clock` so the dev virtual clock works). A per-minute cron flips `closed = true` on expired polls so subscribed clients flip to "final results" live.
4. **One-shot guard = unique index** on `poll_votes (poll, user)`. Vote insert + aggregate increment run in `app.RunInTransaction` (precedent: `internal/scoring/recompute.go:20`).
5. **Slider is integer-only** (`min`/`max`/`step` integers, value validated `min ≤ v ≤ max` and `(v-min)%step == 0`) — avoids float step validation.
6. **After close, everyone sees results** (voted or not). Voted/closed cards are dismissible (localStorage, same pattern as AnnounceBanner); open un-voted cards are not.
7. **No poll edit endpoint** — editing a live poll would corrupt vote semantics. Close early + delete only.

## Step 1 — Migration `migrations/NNNN_polls.go`

> **Note:** This plan was written when the latest migration was `0023`. Other features are being implemented in parallel and will likely add migrations in between — before implementing, check `ls migrations/` and use the next free number (don't assume `0024`).

Model on `0017_announcements.go` (idempotent create, down deletes) and `0021_league_chat.go` (relations).

**`polls`** (base collection):
- `question` Text, required, max 300
- `type` Select (1): `yesno | single | multi | slider`
- `options` JSON (max 4000) — `[]string`; server sets `["Yes","No"]` for yesno; empty for slider
- `min`, `max`, `step` Number (slider only; integers enforced in Go)
- `endsAt` Date, required
- `closed` Bool
- `results` JSON (max 20000):
  - choice types: `{"counts":[n0,n1,...],"total":N}` (multi: `total` = voters, counts sum may exceed)
  - slider: `{"counts":{"<value>":n},"total":N,"sum":S}` (frontend derives avg + histogram)
- `created`/`updated` Autodate
- Rules: `ListRule = ViewRule = "@request.auth.id != ''"` (also authorizes the SSE subscription). Create/Update/Delete `nil` (all writes via Go endpoints).

**`poll_votes`** (base collection):
- `poll` Relation→polls (1, required, cascade), `user` Relation→users (1, required, cascade)
- `answer` JSON (max 2000): `{"option":i}` | `{"options":[i,...]}` | `{"value":v}`
- `created` Autodate
- **Unique index** `idx_poll_votes_poll_user` on `(poll, user)`
- **No rules** — superuser-only; admin exports raw rows from PB dashboard (`/_/`).

## Step 2 — Backend `internal/polls/polls.go` (new)

Follow `internal/announce/announce.go` exactly: `Register(app core.App, se *core.ServeEvent)`, pointer-field payload struct, `view()` map, admin group gating (`apis.RequireAuth()` + `users.IsAdmin(e.Auth)` BindFunc on `/api/admin/polls`).

**User routes** (RequireAuth):
- `GET /api/polls/active` — polls newest-first (cap ~20), each with `status` (`open`/`closed`, computed lazily vs `endsAt`) and `myVote` (caller's answer or null; batch lookup via one `FindRecordsByFilter("poll_votes", "user = {:u}")`). **Omit `results` when open and not voted.**
- `POST /api/polls/{id}/vote` — validate open (closed/endsAt via `clock.Now`), validate answer per type, then in `RunInTransaction`: create vote record (unique-index violation → 400 "already voted"), re-read poll, `UnmarshalJSONField` results, increment, save poll. Return `{poll (with results), myVote}`.

**Admin routes** (`/api/admin/polls` group):
- `GET ""` — all polls, full view incl. results.
- `POST ""` — create. Validate: single/multi need 2–10 non-empty options; yesno → server sets options; slider needs integer `min < max`, `step ≥ 1`, `(max-min)/step ≤ 100`; `endsAt` RFC3339 in the future. Initialize zeroed `results`.
- `POST "/{id}/close"` — set `closed = true`, save (fires SSE).
- `DELETE "/{id}"` — delete (cascade removes votes).

**Cron** (inside Register, like `internal/notify/notify.go:105`): `app.Cron().MustAdd("polls_close", "* * * * *", ...)` — find `closed = false && endsAt <= now`, set closed, save each (broadcasts close).

**Wire up:** `polls.Register(e.App, e)` in `main.go` (~line 61, alongside `announce.Register`).

## Step 3 — Frontend API (`frontend/src/lib/api.ts`)

Types: `PollType`, `PollResults`, `PollAnswer`, `Poll` (id, question, type, options, min/max/step, endsAt, status, results?, myVote?, created), `PollCreatePayload`.
Methods: `activePolls()`, `votePoll(id, answer)`, `adminPolls()`, `createPoll(p)`, `closePoll(id)`, `deletePoll(id)`.

## Step 4 — User-facing cards

New `frontend/src/lib/components/PollStack.svelte` + `PollCard.svelte`; mount `<PollStack />` right after `<AnnounceBanner />` in `frontend/src/routes/+layout.svelte:93`. Leave AnnounceBanner untouched.

**PollStack:** onMount fetch `activePolls()`; localStorage dismiss set `poll-dismissed-v1` (reuse loadSet/persist/prune pattern from `AnnounceBanner.svelte:30-67`); one `pb.collection('polls').subscribe('*', ...)` (pattern from `leagues/[id]/chat/+page.svelte:100-136`) merging `results`/`closed`/new polls into state; unsubscribe on destroy.

**PollCard** (style mirrors `.banner`: surface bg, 1px border, 3px accent left border, radius-sm, icon chip, "closes <date>" line):
1. *Open, not voted:* question + input + Vote button. yesno → two big buttons; single → radios; multi → checkboxes; slider → range input with value bubble + min/max labels. Not dismissible.
2. *Voted (open):* live results — horizontal bars (label, count, %; accent fill for own pick); multi % relative to voter total; slider → big average + slim histogram (fallback to avg + "N votes" if >~30 steps). "Live · N votes". Dismissible.
3. *Closed:* same results, "Final · N votes", dismissible, shown even if never voted.

Vote submit: on success patch local poll; on 400 "already voted" refetch.

## Step 5 — Admin console

Co-located component `frontend/src/routes/announcements/PollAdmin.svelte` (page is already ~557 lines; non-`+` files in route dirs are fine), mounted below the announcements list in `announcements/+page.svelte`. Reuse the page's `.card`/`.fld`/`.pill`/`.ib` vocabulary and `ConfirmDialog`.

- Create form: question, type select, dynamic option rows (single/multi), min/max/step (slider), `datetime-local` end time → `.toISOString()`.
- List: status pill (Live/Closed), type badge, question, vote count, compact aggregate bars (extract shared `PollResultsBars.svelte` used by both PollCard and admin), end time, actions: Close early + Delete (both via ConfirmDialog; delete warns votes go too). UI hint: raw votes exportable from PB dashboard.

## Verification

1. `make dev-backend` (migration auto-applies) + `make dev-frontend`; `make test`; `cd frontend && npm run check`.
2. PB dashboard `http://127.0.0.1:8090/_/`: confirm rules + unique index; confirm `GET /api/collections/poll_votes/records` with a user token is denied.
3. **Realtime:** two sessions/accounts — admin creates poll → appears live in both; vote in A, then B → bars update in both without reload; Close early → cards flip to Final live.
4. **One-shot / closing:** replay vote via curl → 400 "already voted"; vote past endsAt (dev virtual clock, `WMP_DEV=1`) → 400; cron flips closed within a minute and pushes the update.
5. Each type end-to-end incl. slider out-of-step value via curl → 400.

## Risks

- Pre-vote results not hard-enforced (accepted; `poll_results` split is the escape hatch if ever needed).
- Verify SSE broadcast fires post-commit for saves inside `RunInTransaction` on PB v0.38 (expected — After-Success hooks run post-commit); fallback: move the poll save just outside the tx, keeping the vote insert transactional.
- Read `results` JSON with `rec.UnmarshalJSONField` into typed structs (precedent: `internal/scoring/scoring.go:494`), not raw type-asserts.
