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
	return &Runner{
		app:          app,
		sender:       mailer.Pick(app),
		push:         push.NewSender(push.ResolveKeys(app)),
		lastAllowKey: allowlistKey(readConfig(app).Allowlist),
		verbose:      strings.EqualFold(os.Getenv("NOTIFY_LOG_LEVEL"), "debug"),
	}
}

// Register wires the notify cron (and, in dev, a manual trigger route).
func Register(app core.App, se *core.ServeEvent) {
	seedNotifyConfig(app)
	r := New(app)

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
	}
}

// RunOnce executes every event detector for the current (possibly simulated)
// time and returns a summary. Detector errors are logged but don't abort the
// pass — one broken event shouldn't suppress the others.
func (r *Runner) RunOnce(ctx context.Context) (*Result, error) {
	now := clock.Now(r.app)
	cfg := readConfig(r.app)
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
	if err := r.detectTipsReminder(ctx, res, now, lead, matches, recipients, base); err != nil {
		log.Printf("[notify] tips_reminder: %v", err)
	}
	if now.Hour() == cfg.RecapHourUTC {
		if err := r.detectResultsRecap(ctx, res, now, matches, recipients, base); err != nil {
			log.Printf("[notify] results_recap: %v", err)
		}
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
		"leadHours":    defaultLeadHours,
		"recapHourUTC": defaultRecapHourUTC,
		"allowlist":    []string{},
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
	data.SettingsUrl = r.base().url + "/settings"

	r.dispatchEmail(ctx, res, ncol, u, event, dedupKey, data)
	r.dispatchPush(ctx, res, ncol, u, event, dedupKey, data)
}

// dispatchEmail sends one email to one user for one event.
func (r *Runner) dispatchEmail(ctx context.Context, res *Result, ncol *core.Collection,
	u *core.Record, event, dedupKey string, data tplData) {

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

	mid, sendErr := r.sender.Send(ctx, mailer.Message{
		ToEmail: u.Email(),
		ToName:  u.GetString("name"),
		Subject: subject,
		HTML:    html,
		Text:    text,
	})
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

	ok, sendErr := r.sendPush(ctx, subs, push.Notification{
		Title: title, Body: body, URL: data.CTAUrl, Tag: event,
	})
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

// AppNameOr returns d's AppName if set, else the fallback.
func (d tplData) AppNameOr(fallback string) string {
	if d.AppName != "" {
		return d.AppName
	}
	return fallback
}
