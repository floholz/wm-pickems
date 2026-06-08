// Package notify is the notification core: a scheduler that detects upcoming
// tournament deadlines and recaps, and dispatches them through the swappable
// mailer. It is keyed off clock.Now so it follows the dev virtual clock, writes
// a per-send ledger row for idempotency + status, and honours per-user prefs.
//
// Channels: only "email" is wired today. The ledger, prefs and dispatch are
// channel-aware so Web Push slots in later as a second Sender + channel.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/floholz/wm-pickems/internal/clock"
	"github.com/floholz/wm-pickems/internal/mailer"
	"github.com/floholz/wm-pickems/internal/push"
	"github.com/floholz/wm-pickems/internal/users"
)

// Runner holds the dependencies for one notify pass.
type Runner struct {
	app    core.App
	sender mailer.Sender
	push   push.Sender
	// lastAllowKey is the normalized allowlist seen on the previous pass, used
	// to log when the allowlist changes at runtime.
	lastAllowKey string
	// gate is the global delivery policy (master channel switches + per-event
	// overrides), refreshed at the start of each pass. Safe to share across the
	// three notification crons: every pass derives it from the same app_meta row,
	// so a concurrent refresh can only ever make it momentarily stale, never wrong.
	gate config
	// verbose enables the per-pass heartbeat log (NOTIFY_LOG_LEVEL=debug).
	verbose bool
}

// base identity reused across emails.
type baseInfo struct {
	appName string
	url     string // app base URL, no trailing slash
}

// Result summarises one RunOnce pass (returned to the dev trigger route).
type Result struct {
	Considered int `json:"considered"`
	Sent       int `json:"sent"`
	Failed     int `json:"failed"`
	Skipped    int `json:"skipped"`
}

// New builds a Runner, selecting the mail provider once. lastAllowKey is
// seeded from the current config so the first pass doesn't report a "change".
func New(app core.App) *Runner {
	cfg := readConfig(app)
	return &Runner{
		app:          app,
		sender:       mailer.Pick(app),
		push:         push.NewSender(push.ResolveKeys(app)),
		lastAllowKey: allowlistKey(cfg.Allowlist),
		gate:         cfg,
		verbose:      strings.EqualFold(os.Getenv("NOTIFY_LOG_LEVEL"), "debug"),
	}
}

// disabled reports whether the scheduler is switched off via NOTIFY_DISABLED.
// Accepts the usual truthy spellings (1/true/yes/on, any case).
func disabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("NOTIFY_DISABLED"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// Register wires the notify cron (and, in dev, a manual trigger route).
func Register(app core.App, se *core.ServeEvent) {
	seedNotifyConfig(app)
	r := New(app)

	// Global delivery-policy endpoints (read for all, write for admins). Wired
	// unconditionally so the switches work even when the scheduler is disabled.
	registerPolicy(app, se)

	// NOTIFY_DISABLED skips the scheduler entirely so local/test runs never fire
	// automated notifications. The dev manual-trigger and preview routes below
	// stay wired, so rendering can still be exercised on demand.
	if disabled() {
		log.Printf("[notify] scheduler disabled (NOTIFY_DISABLED) — no automated notifications will be sent")
	} else {
		cronExpr := os.Getenv("NOTIFY_CRON")
		if cronExpr == "" {
			cronExpr = "*/15 * * * *"
		}
		app.Cron().MustAdd("notify", cronExpr, func() {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			if _, err := r.RunOnce(ctx); err != nil {
				log.Printf("[notify] %v", err)
			}
		})
		log.Printf("[notify] scheduler enabled (%s) via %s", cronExpr, r.sender.Name())
		if al := readConfig(app).Allowlist; len(al) > 0 {
			log.Printf("[notify] allowlist active — only %d address(es) will be emailed", len(al))
		}
		registerChatCron(app, r)
		registerChatDigestCron(app, r)
	}

	// Dev-only manual trigger so the flow can be exercised against the virtual
	// clock without waiting for the cron. Mirrors the /api/dev gating in dev.go.
	if os.Getenv("WMP_DEV") == "1" {
		se.Router.POST("/api/dev/notify/run", func(e *core.RequestEvent) error {
			ctx, cancel := context.WithTimeout(e.Request.Context(), 90*time.Second)
			defer cancel()
			res, err := r.RunOnce(ctx)
			if err != nil {
				return e.JSON(500, map[string]any{"error": err.Error(), "result": res})
			}
			return e.JSON(http.StatusOK, res)
		}).Bind(apis.RequireAuth())

		// Run the chat-notification sweep on demand.
		se.Router.POST("/api/dev/chat/notify", func(e *core.RequestEvent) error {
			ctx, cancel := context.WithTimeout(e.Request.Context(), 60*time.Second)
			defer cancel()
			return e.JSON(http.StatusOK, map[string]any{"sent": r.chatPass(ctx)})
		}).Bind(apis.RequireAuth())

		// Run the chat email-digest sweep on demand.
		se.Router.POST("/api/dev/chat/digest", func(e *core.RequestEvent) error {
			ctx, cancel := context.WithTimeout(e.Request.Context(), 90*time.Second)
			defer cancel()
			return e.JSON(http.StatusOK, map[string]any{"sent": r.chatDigestPass(ctx)})
		}).Bind(apis.RequireAuth())

		// Render an email in the browser (no auth, dev-only) for fast visual
		// iteration: /api/dev/notify/preview?event=results_recap[&fmt=text].
		se.Router.GET("/api/dev/notify/preview", func(e *core.RequestEvent) error {
			event := e.Request.URL.Query().Get("event")
			if event == "" {
				event = "stage_starting"
			}
			data := r.sampleData(event)
			subject, html, text, err := render(event, data)
			if err != nil {
				return e.JSON(500, map[string]string{"error": err.Error()})
			}
			if e.Request.URL.Query().Get("fmt") == "text" {
				return e.Blob(http.StatusOK, "text/plain; charset=utf-8", []byte("Subject: "+subject+"\n\n"+text))
			}
			// In real emails the mark is an inline cid: attachment; for the
			// browser preview, point it at the served asset so it renders.
			html = strings.ReplaceAll(html, "cid:mark", "/email/mark.png")
			return e.Blob(http.StatusOK, "text/html; charset=utf-8", []byte(html))
		})

		// Send a representative email for one event to the caller, to verify
		// rendering in a real mail client.
		se.Router.POST("/api/dev/notify/email", func(e *core.RequestEvent) error {
			var body struct {
				Event string `json:"event"`
			}
			if err := e.BindBody(&body); err != nil {
				return e.JSON(400, map[string]string{"error": err.Error()})
			}
			ctx, cancel := context.WithTimeout(e.Request.Context(), 20*time.Second)
			defer cancel()
			provider, err := r.SendSampleEmail(ctx, e.Auth, body.Event)
			if err != nil {
				return e.JSON(400, map[string]any{"error": err.Error(), "provider": provider})
			}
			return e.JSON(http.StatusOK, map[string]any{"to": e.Auth.Email(), "provider": provider})
		}).Bind(apis.RequireAuth())

		// Send a representative push for one event to the caller's devices, so
		// each notification type can be previewed on a real device.
		se.Router.POST("/api/dev/push/sample", func(e *core.RequestEvent) error {
			var body struct {
				Event string `json:"event"`
			}
			if err := e.BindBody(&body); err != nil {
				return e.JSON(400, map[string]string{"error": err.Error()})
			}
			ctx, cancel := context.WithTimeout(e.Request.Context(), 15*time.Second)
			defer cancel()
			sent, total, err := r.SendSample(ctx, e.Auth.Id, body.Event)
			if err != nil {
				return e.JSON(400, map[string]any{"error": err.Error()})
			}
			return e.JSON(http.StatusOK, map[string]any{"sent": sent, "total": total})
		}).Bind(apis.RequireAuth())
	}
}

// RunOnce executes every event detector for the current (possibly simulated)
// time and returns a summary. Detector errors are logged but don't abort the
// pass — one broken event shouldn't suppress the others.
func (r *Runner) RunOnce(ctx context.Context) (*Result, error) {
	now := clock.Now(r.app)
	cfg := readConfig(r.app)
	r.gate = cfg
	r.logAllowlistChange(cfg.Allowlist)
	lead := time.Duration(cfg.LeadHours) * time.Hour
	base := r.base()
	res := &Result{}

	// Log a one-line pass summary. In the default (info) level only passes that
	// did something are logged; NOTIFY_LOG_LEVEL=debug logs every pass as a
	// heartbeat to confirm the scheduler is running.
	defer func() {
		if r.verbose || res.Sent > 0 || res.Failed > 0 {
			log.Printf("[notify] pass: considered=%d sent=%d failed=%d skipped=%d",
				res.Considered, res.Sent, res.Failed, res.Skipped)
		}
	}()

	recipients, err := r.eligibleUsers(cfg.Allowlist)
	if err != nil {
		return res, fmt.Errorf("load recipients: %w", err)
	}
	if len(recipients) == 0 {
		return res, nil
	}

	matches, err := r.app.FindRecordsByFilter("matches", "id != ''", "kickoff", 0, 0)
	if err != nil {
		return res, fmt.Errorf("load matches: %w", err)
	}

	if err := r.detectStageStarting(ctx, res, now, lead, matches, recipients, base); err != nil {
		log.Printf("[notify] stage_starting: %v", err)
	}
	if err := r.detectForecastReminder(ctx, res, now, lead, matches, recipients, base); err != nil {
		log.Printf("[notify] forecast_reminder: %v", err)
	}
	if now.Hour() >= cfg.CountdownHourUTC {
		if err := r.detectKickoffCountdown(ctx, res, now, matches, recipients, base); err != nil {
			log.Printf("[notify] kickoff_countdown: %v", err)
		}
	}
	if err := r.detectTipsReminder(ctx, res, now, lead, matches, recipients, base); err != nil {
		log.Printf("[notify] tips_reminder: %v", err)
	}
	if now.Hour() == cfg.RecapHourUTC {
		if err := r.detectResultsRecap(ctx, res, now, matches, recipients, base); err != nil {
			log.Printf("[notify] results_recap: %v", err)
		}
	}
	if err := r.detectLeagueLead(ctx, res, recipients, base); err != nil {
		log.Printf("[notify] league_lead: %v", err)
	}

	return res, nil
}

// logAllowlistChange logs when the resolved allowlist differs from the previous
// pass (e.g. edited via app_meta at runtime), so the rollout state is visible
// without a restart.
func (r *Runner) logAllowlistChange(current []string) {
	key := allowlistKey(current)
	if key == r.lastAllowKey {
		return
	}
	r.lastAllowKey = key
	if len(current) == 0 {
		log.Printf("[notify] allowlist cleared — emailing all eligible users")
	} else {
		log.Printf("[notify] allowlist changed — now %d address(es)", len(current))
	}
}

// allowlistKey is an order-independent fingerprint of an allowlist for change
// detection.
func allowlistKey(list []string) string {
	sorted := append([]string(nil), list...)
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}

// seedNotifyConfig writes a default notify_config row (lead time, recap hour,
// empty allowlist) into app_meta on first boot if absent, so the settings are
// discoverable and editable from the PocketBase dashboard. Existing rows are
// left untouched.
func seedNotifyConfig(app core.App) {
	if _, err := app.FindFirstRecordByFilter("app_meta",
		"key = {:k}", map[string]any{"k": metaKey}); err == nil {
		return
	}
	col, err := app.FindCollectionByNameOrId("app_meta")
	if err != nil {
		return
	}
	rec := core.NewRecord(col)
	rec.Set("key", metaKey)
	rec.Set("value", map[string]any{
		"leadHours":        defaultLeadHours,
		"recapHourUTC":     defaultRecapHourUTC,
		"countdownHourUTC": defaultCountdownHourUTC,
		"allowlist":        []string{},
		// Master delivery switches (both on) + per-event overrides (none). Surfaced
		// here so the policy is discoverable/editable from the PB dashboard too.
		"channels": map[string]any{"email": true, "push": true},
		"disabled": map[string]any{},
	})
	if err := app.Save(rec); err != nil {
		log.Printf("[notify] seed config: %v", err)
	}
}

func (r *Runner) base() baseInfo {
	meta := r.app.Settings().Meta
	url := strings.TrimRight(meta.AppURL, "/")
	name := meta.AppName
	if name == "" {
		name = "WM Pickems"
	}
	return baseInfo{appName: name, url: url}
}

// eligibleUsers returns real (non-bot) users with an email address. When
// allowlist is non-empty, only addresses on it are returned (gradual rollout).
func (r *Runner) eligibleUsers(allowlist []string) ([]*core.Record, error) {
	all, err := r.app.FindRecordsByFilter("users", "id != ''", "", 0, 0)
	if err != nil {
		return nil, err
	}
	var allow map[string]bool
	if len(allowlist) > 0 {
		allow = make(map[string]bool, len(allowlist))
		for _, e := range allowlist {
			allow[e] = true
		}
	}
	out := make([]*core.Record, 0, len(all))
	for _, u := range all {
		if users.IsBot(u) || u.Email() == "" {
			continue
		}
		if allow != nil && !allow[strings.ToLower(u.Email())] {
			continue
		}
		out = append(out, u)
	}
	return out, nil
}

// prefEnabled reports whether a user wants `event` notifications on `channel`.
// Absent prefs (or an absent entry) default to ON, so users opt out rather than
// in.
func prefEnabled(u *core.Record, event, channel string) bool {
	return prefEnabledFromRaw(u.GetString("notifyPrefs"), event, channel)
}

// prefEnabledFromRaw is the pure core of prefEnabled, split out for testing.
func prefEnabledFromRaw(raw, event, channel string) bool {
	if raw == "" {
		return true
	}
	var prefs map[string]map[string]bool
	if err := json.Unmarshal([]byte(raw), &prefs); err != nil {
		return true
	}
	ev, ok := prefs[event]
	if !ok {
		return true
	}
	v, ok := ev[channel]
	if !ok {
		return true
	}
	return v
}

// alreadySent reports whether this (dedupKey, channel) has already been recorded
// (race-free under the single-threaded cron; the unique index is the backstop).
func (r *Runner) alreadySent(dedupKey, channel string) bool {
	_, err := r.app.FindFirstRecordByFilter("notifications",
		"dedupKey = {:k} && channel = {:c}",
		map[string]any{"k": dedupKey, "c": channel})
	return err == nil
}

// dispatch fans one event out to every channel the user has enabled (email +
// push), recording each outcome in the ledger under its own channel row.
func (r *Runner) dispatch(ctx context.Context, res *Result, ncol *core.Collection,
	u *core.Record, event, dedupKey string, data tplData) {

	data.AppName = data.AppNameOr(r.base().appName)
	data.BaseURL = r.base().url
	data.SettingsUrl = r.base().url + "/settings"

	r.dispatchEmail(ctx, res, ncol, u, event, dedupKey, data)
	r.dispatchPush(ctx, res, ncol, u, event, dedupKey, data)
}

// dispatchEmail sends one email to one user for one event.
func (r *Runner) dispatchEmail(ctx context.Context, res *Result, ncol *core.Collection,
	u *core.Record, event, dedupKey string, data tplData) {

	// Global policy gate (e.g. mail provider suspended). Suppressed platform-wide,
	// not a user choice, so it's silent and uncounted — like push-not-configured.
	if !r.gate.channelAllowed(event, "email") {
		return
	}
	res.Considered++
	if !prefEnabled(u, event, "email") {
		res.Skipped++
		return
	}
	if r.alreadySent(dedupKey, "email") {
		res.Skipped++
		return
	}

	subject, html, text, err := render(event, data)
	rec := newLedgerRow(ncol, u.Id, event, dedupKey, "email")
	if err != nil {
		rec.Set("status", "failed")
		rec.Set("error", err.Error())
		_ = r.app.Save(rec)
		res.Failed++
		return
	}
	// Insert the queued row first; a unique-index failure means a concurrent
	// pass already claimed this key — skip silently.
	if err := r.app.Save(rec); err != nil {
		res.Skipped++
		return
	}

	mid, sendErr := r.sender.Send(ctx, mailerMessage(u, subject, html, text))
	if sendErr != nil {
		rec.Set("status", "failed")
		rec.Set("error", sendErr.Error())
		_ = r.app.Save(rec)
		res.Failed++
		return
	}
	rec.Set("status", "sent")
	rec.Set("providerMessageId", mid)
	rec.Set("sentAt", time.Now().UTC())
	_ = r.app.Save(rec)
	res.Sent++
}

// dispatchPush sends a push to all of a user's devices for one event. It no-ops
// (without counting) when push is unconfigured or the user has no subscription,
// since those aren't user-choice skips.
func (r *Runner) dispatchPush(ctx context.Context, res *Result, ncol *core.Collection,
	u *core.Record, event, dedupKey string, data tplData) {

	if r.push == nil || !r.push.Enabled() {
		return
	}
	if !r.gate.channelAllowed(event, "push") {
		return
	}
	subs, err := push.Subscriptions(r.app, u.Id)
	if err != nil || len(subs) == 0 {
		return
	}

	res.Considered++
	if !prefEnabled(u, event, "push") {
		res.Skipped++
		return
	}
	if r.alreadySent(dedupKey, "push") {
		res.Skipped++
		return
	}

	title, body, err := renderPush(event, data)
	rec := newLedgerRow(ncol, u.Id, event, dedupKey, "push")
	if err != nil {
		rec.Set("status", "failed")
		rec.Set("error", err.Error())
		_ = r.app.Save(rec)
		res.Failed++
		return
	}
	if err := r.app.Save(rec); err != nil {
		res.Skipped++
		return
	}

	n := push.Notification{
		Title: title, Body: body, URL: toPath(data.CTAUrl), Tag: event, Icon: pushIcon(event, data),
	}
	if data.HighPriority {
		n.Urgency = push.UrgencyHigh
		n.RequireInteraction = true
	}
	ok, sendErr := r.sendPush(ctx, subs, n)
	if ok == 0 {
		rec.Set("status", "failed")
		if sendErr != nil {
			rec.Set("error", sendErr.Error())
		} else {
			rec.Set("error", "no reachable subscriptions")
		}
		_ = r.app.Save(rec)
		res.Failed++
		return
	}
	rec.Set("status", "sent")
	rec.Set("sentAt", time.Now().UTC())
	_ = r.app.Save(rec)
	res.Sent++
}

// sendPush delivers a notification to every subscription, pruning dead ones, and
// returns how many were accepted plus the last non-fatal error.
func (r *Runner) sendPush(ctx context.Context, subs []push.Subscription, n push.Notification) (int, error) {
	ok := 0
	var lastErr error
	for _, s := range subs {
		err := r.push.Send(ctx, s, n)
		switch {
		case err == nil:
			ok++
		case errors.Is(err, push.ErrGone):
			push.PruneEndpoint(r.app, s.Endpoint)
		default:
			lastErr = err
		}
	}
	return ok, lastErr
}

// newLedgerRow builds a queued notifications row.
func newLedgerRow(ncol *core.Collection, userID, event, dedupKey, channel string) *core.Record {
	rec := core.NewRecord(ncol)
	rec.Set("user", userID)
	rec.Set("event", event)
	rec.Set("dedupKey", dedupKey)
	rec.Set("channel", channel)
	rec.Set("status", "queued")
	return rec
}

// SendSample renders a representative push for an event and delivers it to the
// user's devices — backs the dev test buttons. Returns (accepted, total).
func (r *Runner) SendSample(ctx context.Context, userID, event string) (int, int, error) {
	if r.push == nil || !r.push.Enabled() {
		return 0, 0, fmt.Errorf("push not configured")
	}
	subs, err := push.Subscriptions(r.app, userID)
	if err != nil {
		return 0, 0, err
	}
	if len(subs) == 0 {
		return 0, 0, fmt.Errorf("no push subscriptions on this account")
	}
	data := r.sampleData(event)
	title, body, err := renderPush(event, data)
	if err != nil {
		return 0, len(subs), err
	}
	ok, _ := r.sendPush(ctx, subs, push.Notification{
		Title: title, Body: body, URL: toPath(data.CTAUrl), Tag: event, Icon: pushIcon(event, data),
	})
	return ok, len(subs), nil
}

// SendSampleEmail renders a representative email for an event and sends it to
// the user — backs the dev email test buttons. Returns the active provider name.
func (r *Runner) SendSampleEmail(ctx context.Context, u *core.Record, event string) (string, error) {
	if u.Email() == "" {
		return r.sender.Name(), fmt.Errorf("your account has no email address")
	}
	data := r.sampleData(event)
	subject, html, text, err := render(event, data)
	if err != nil {
		return r.sender.Name(), err
	}
	if _, err := r.sender.Send(ctx, mailerMessage(u, subject, html, text)); err != nil {
		return r.sender.Name(), err
	}
	return r.sender.Name(), nil
}

// sampleData builds representative template data for a sample/test push.
func (r *Runner) sampleData(event string) tplData {
	base := r.base()
	when := formatKickoff(time.Date(2026, 6, 11, 19, 0, 0, 0, time.UTC))
	d := tplData{AppName: base.appName, BaseURL: base.url, SettingsUrl: base.url + "/settings"}
	switch event {
	case "stage_starting":
		d.StageName = "Round of 32"
		d.StartsIn = "12 hours"
		d.WhenText = formatKickoff(time.Date(2026, 6, 28, 18, 0, 0, 0, time.UTC))
		d.CTAText, d.CTAUrl = "Open your tips", base.url+"/tips"
	case "forecast_reminder":
		d.StartsIn = "12 hours"
		d.WhenText = when
		d.CTAText, d.CTAUrl = "Finish your Forecast", base.url+"/forecast"
	case "tips_reminder":
		d.Count = 1
		d.Matches = []matchLine{{Home: "Mexico", Away: "South Africa", HomeCode: "MEX", AwayCode: "RSA", WhenText: when}}
		d.CTAText, d.CTAUrl = "Enter your tips", base.url+"/tips"
	case "results_recap":
		d.Finalized = 3
		d.PointsGained = 7
		d.Total = 42
		d.Ranks = []rankLine{{League: "Friends", Rank: 2, Of: 8}}
		d.CTAText, d.CTAUrl = "See the leaderboard", base.url+"/leagues"
	case "league_lead":
		d.League = "Friends"
		d.Total = 48
		d.CTAText, d.CTAUrl = "See the leaderboard", base.url+"/leagues"
	case "announcement":
		d.Title = "New: live match tracker is here"
		d.Body = "We just shipped a live tracker so you can follow scores in real time. Open the app to check it out and get your tips in before kickoff."
		d.CTAText, d.CTAUrl = "Open WM Tips", base.url+"/"
	case "league_chat":
		d.ChatTotal = 5
		d.ChatLeagues = []chatLine{{League: "Squad", Count: 3}, {League: "Office Pool", Count: 2}}
		d.CTAText, d.CTAUrl = "Open your chats", base.url+"/leagues"
	case "kickoff_countdown":
		d.DaysLeft = 3
		d.WhenText = when
		d.CTAText, d.CTAUrl = "Open WM Tips", base.url+"/"
	default:
		d.CTAUrl = base.url + "/"
	}
	return d
}

// AppNameOr returns d's AppName if set, else the fallback.
func (d tplData) AppNameOr(fallback string) string {
	if d.AppName != "" {
		return d.AppName
	}
	return fallback
}
