package notify

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/floholz/wm-pickems/internal/push"
	"github.com/floholz/wm-pickems/internal/scoring"
)

// stageOrder is the canonical tournament progression; stageName maps the stored
// codes to human labels used in emails.
var stageOrder = []string{"group", "R32", "R16", "QF", "SF", "3RD", "FINAL"}

var stageName = map[string]string{
	"group": "Group Stage",
	"R32":   "Round of 32",
	"R16":   "Round of 16",
	"QF":    "Quarter-finals",
	"SF":    "Semi-finals",
	"3RD":   "Third-place Play-off",
	"FINAL": "Final",
}

func (r *Runner) notificationsCol() (*core.Collection, error) {
	return r.app.FindCollectionByNameOrId("notifications")
}

// detectStageStarting emails everyone when a stage's first kickoff enters the
// lead window. One email per stage (dedup stage_starting:<stage>).
func (r *Runner) detectStageStarting(ctx context.Context, res *Result, now time.Time,
	lead time.Duration, matches []*core.Record, recipients []*core.Record, base baseInfo) error {

	// Earliest kickoff per stage.
	starts := map[string]time.Time{}
	for _, m := range matches {
		st := m.GetString("stage")
		ko := m.GetDateTime("kickoff").Time()
		if cur, ok := starts[st]; !ok || ko.Before(cur) {
			starts[st] = ko
		}
	}

	ncol, err := r.notificationsCol()
	if err != nil {
		return err
	}
	for _, st := range stageOrder {
		start, ok := starts[st]
		if !ok || !inLeadWindow(now, start, lead) {
			continue
		}
		data := tplData{
			StageName: stageName[st],
			StartsIn:  humanizeDur(start.Sub(now)),
			WhenText:  formatKickoff(start),
			CTAText:   "Open your tips",
			CTAUrl:    base.url + "/tips",
		}
		for _, u := range recipients {
			// Dedup key is per-user (and channel, via the ledger index) so every
			// recipient is reminded — not just the first.
			r.dispatch(ctx, res, ncol, u, "stage_starting", "stage_starting:"+st+":"+u.Id, data)
		}
	}
	return nil
}

// detectForecastReminder nudges users whose Forecast is incomplete as the global
// lock (tournament first kickoff) approaches.
func (r *Runner) detectForecastReminder(ctx context.Context, res *Result, now time.Time,
	lead time.Duration, matches []*core.Record, recipients []*core.Record, base baseInfo) error {

	if len(matches) == 0 {
		return nil
	}
	start := matches[0].GetDateTime("kickoff").Time() // sorted by kickoff asc
	if !inLeadWindow(now, start, lead) {
		return nil
	}
	groupCount, err := r.app.CountRecords("tournament_groups")
	if err != nil {
		return err
	}
	ncol, err := r.notificationsCol()
	if err != nil {
		return err
	}
	data := tplData{
		StartsIn: humanizeDur(start.Sub(now)),
		WhenText: formatKickoff(start),
		CTAText:  "Finish your Forecast",
		CTAUrl:   base.url + "/forecast",
	}
	for _, u := range recipients {
		if !r.forecastIncomplete(u.Id, int(groupCount)) {
			continue
		}
		r.dispatch(ctx, res, ncol, u, "forecast_reminder", "forecast_reminder:"+u.Id, data)
	}
	return nil
}

// countdownFromDays is how many days before the first kickoff the daily
// pre-tournament countdown starts (so a value of 4 yields 4-, 3-, 2-, 1-days-left
// and a "starts today" send on kickoff day).
const countdownFromDays = 4

// detectKickoffCountdown sends a once-daily countdown to the whole field in the
// final days before the tournament's first kickoff ("4 days left" … "1 more day"
// … "the World Cup starts today"), always reminding players to finish their tips
// and Forecast. Gated to the countdown hour by the caller; deduped per calendar
// day so it fires once a day regardless of cron cadence.
func (r *Runner) detectKickoffCountdown(ctx context.Context, res *Result, now time.Time,
	matches []*core.Record, recipients []*core.Record, base baseInfo) error {

	if len(matches) == 0 {
		return nil
	}
	start := matches[0].GetDateTime("kickoff").Time().UTC() // sorted by kickoff asc

	// Calendar-day difference (UTC), so the count flips at midnight rather than on
	// the rolling 24h boundary: e.g. any time on the 7th with kickoff on the 11th
	// is "4 days left", and any time on kickoff day is "today" (0).
	kickoffDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	daysLeft := int(kickoffDay.Sub(today).Hours() / 24)
	if daysLeft < 0 || daysLeft > countdownFromDays {
		return nil
	}

	ncol, err := r.notificationsCol()
	if err != nil {
		return err
	}
	dateKey := today.Format("2006-01-02")
	data := tplData{
		DaysLeft: daysLeft,
		WhenText: formatKickoff(start),
		CTAText:  "Open WM Tips",
		CTAUrl:   base.url + "/",
	}
	for _, u := range recipients {
		r.dispatch(ctx, res, ncol, u, "kickoff_countdown", "kickoff_countdown:"+dateKey+":"+u.Id, data)
	}
	return nil
}

// detectTipsReminder sends a per-user digest of upcoming matches (within the
// lead window) the user hasn't tipped. Dedup is per (user, match) so each match
// is reminded at most once, while the email batches all newly-missing matches.
func (r *Runner) detectTipsReminder(ctx context.Context, res *Result, now time.Time,
	lead time.Duration, matches []*core.Record, recipients []*core.Record, base baseInfo) error {

	windowEnd := now.Add(lead)
	var upcoming []*core.Record
	for _, m := range matches {
		ko := m.GetDateTime("kickoff").Time()
		if m.GetString("status") == "scheduled" && ko.After(now) && !ko.After(windowEnd) {
			upcoming = append(upcoming, m)
		}
	}
	if len(upcoming) == 0 {
		return nil
	}
	ncol, err := r.notificationsCol()
	if err != nil {
		return err
	}
	names := r.teamNames()
	codes := r.teamCodes()

	for _, u := range recipients {
		r.tipsEmail(ctx, res, ncol, u, upcoming, names, codes, base)
		r.tipsPush(ctx, res, ncol, u, upcoming, names, codes, base)
	}
	return nil
}

// tipsKey is the per-(user, match) dedup key for tip reminders.
func tipsKey(userID, matchID string) string {
	return "tips_reminder:" + userID + ":" + matchID
}

// missingTips returns the upcoming matches the user hasn't tipped and hasn't yet
// been reminded about on this channel.
func (r *Runner) missingTips(u *core.Record, upcoming []*core.Record, channel string) []*core.Record {
	var out []*core.Record
	for _, m := range upcoming {
		if r.hasTip(u.Id, m.Id) {
			continue
		}
		if r.alreadySent(tipsKey(u.Id, m.Id), channel) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// tipsData builds the digest template data for a set of untipped matches.
func (r *Runner) tipsData(missing []*core.Record, names, codes map[string]string, base baseInfo) tplData {
	lines := make([]matchLine, 0, len(missing))
	for _, m := range missing {
		lines = append(lines, matchLine{
			Home:     r.teamLabel(m, "homeTeam", "homeLabel", names),
			Away:     r.teamLabel(m, "awayTeam", "awayLabel", names),
			HomeCode: r.teamLabel(m, "homeTeam", "homeLabel", codes),
			AwayCode: r.teamLabel(m, "awayTeam", "awayLabel", codes),
			WhenText: formatKickoff(m.GetDateTime("kickoff").Time()),
		})
	}
	return tplData{
		AppName:     base.appName,
		BaseURL:     base.url,
		SettingsUrl: base.url + "/settings",
		CTAText:     "Enter your tips",
		CTAUrl:      base.url + "/tips",
		Count:       len(lines),
		Matches:     lines,
	}
}

// writeTipsRows records one ledger row per reminded match (so each match is
// deduped independently) sharing the single digest send's outcome.
func (r *Runner) writeTipsRows(ncol *core.Collection, userID string, missing []*core.Record,
	channel, mid, status, errStr string) {
	for _, m := range missing {
		rec := newLedgerRow(ncol, userID, "tips_reminder", tipsKey(userID, m.Id), channel)
		rec.Set("status", status)
		rec.Set("providerMessageId", mid)
		rec.Set("error", errStr)
		if status == "sent" {
			rec.Set("sentAt", time.Now().UTC())
		}
		_ = r.app.Save(rec)
	}
}

// tipsEmail sends the untipped-matches digest by email.
func (r *Runner) tipsEmail(ctx context.Context, res *Result, ncol *core.Collection,
	u *core.Record, upcoming []*core.Record, names, codes map[string]string, base baseInfo) {

	if !r.gate.channelAllowed("tips_reminder", "email") || !prefEnabled(u, "tips_reminder", "email") {
		return
	}
	missing := r.missingTips(u, upcoming, "email")
	if len(missing) == 0 {
		return
	}
	res.Considered++
	data := r.tipsData(missing, names, codes, base)
	subject, html, text, rerr := render("tips_reminder", data)
	if rerr != nil {
		res.Failed++
		return
	}
	mid, serr := r.sender.Send(ctx, mailerMessage(u, subject, html, text))
	status, errStr := "sent", ""
	if serr != nil {
		status, errStr = "failed", serr.Error()
		res.Failed++
	} else {
		res.Sent++
	}
	r.writeTipsRows(ncol, u.Id, missing, "email", mid, status, errStr)
}

// tipsPush sends the untipped-matches digest as a push to all the user's devices.
func (r *Runner) tipsPush(ctx context.Context, res *Result, ncol *core.Collection,
	u *core.Record, upcoming []*core.Record, names, codes map[string]string, base baseInfo) {

	if r.push == nil || !r.push.Enabled() ||
		!r.gate.channelAllowed("tips_reminder", "push") || !prefEnabled(u, "tips_reminder", "push") {
		return
	}
	subs, err := push.Subscriptions(r.app, u.Id)
	if err != nil || len(subs) == 0 {
		return
	}
	missing := r.missingTips(u, upcoming, "push")
	if len(missing) == 0 {
		return
	}
	res.Considered++
	data := r.tipsData(missing, names, codes, base)
	title, body, rerr := renderPush("tips_reminder", data)
	if rerr != nil {
		res.Failed++
		return
	}
	ok, serr := r.sendPush(ctx, subs, push.Notification{
		Title: title, Body: body, URL: toPath(data.CTAUrl), Tag: "tips_reminder", Icon: pushIcon("tips_reminder", data),
	})
	status, errStr := "sent", ""
	if ok == 0 {
		status = "failed"
		if serr != nil {
			errStr = serr.Error()
		} else {
			errStr = "no reachable subscriptions"
		}
		res.Failed++
	} else {
		res.Sent++
	}
	r.writeTipsRows(ncol, u.Id, missing, "push", "", status, errStr)
}

// leaderState is the persisted current #1 of a league (app_meta "lead:<id>").
// Seq increments on each takeover so the ledger dedup key is unique per reign.
type leaderState struct {
	Leader string `json:"leader"`
	Seq    int    `json:"seq"`
}

func (r *Runner) storedLeader(leagueID string) leaderState {
	rec, err := r.app.FindFirstRecordByFilter("app_meta",
		"key = {:k}", map[string]any{"k": "lead:" + leagueID})
	if err != nil {
		return leaderState{}
	}
	var s leaderState
	_ = rec.UnmarshalJSONField("value", &s)
	return s
}

func (r *Runner) setStoredLeader(leagueID string, s leaderState) {
	key := "lead:" + leagueID
	rec, err := r.app.FindFirstRecordByFilter("app_meta", "key = {:k}", map[string]any{"k": key})
	if err != nil {
		col, cerr := r.app.FindCollectionByNameOrId("app_meta")
		if cerr != nil {
			return
		}
		rec = core.NewRecord(col)
		rec.Set("key", key)
	}
	rec.Set("value", map[string]any{"leader": s.Leader, "seq": s.Seq})
	_ = r.app.Save(rec)
}

// detectLeagueLead notifies the new #1 whenever a league's leader changes. The
// leader is persisted per league, so it fires once per genuine takeover (not
// every cron tick), and only for leaders who are eligible recipients.
func (r *Runner) detectLeagueLead(ctx context.Context, res *Result,
	recipients []*core.Record, base baseInfo) error {

	ncol, err := r.notificationsCol()
	if err != nil {
		return err
	}
	byID := map[string]*core.Record{}
	for _, u := range recipients {
		byID[u.Id] = u
	}

	leagues, err := r.app.FindRecordsByFilter("leagues", "id != ''", "", 0, 0)
	if err != nil {
		return err
	}
	for _, lg := range leagues {
		lb, err := scoring.Leaderboard(r.app, lg.Id)
		if err != nil {
			continue
		}
		rows, _ := lb["rows"].([]scoring.Row)
		if len(rows) == 0 {
			continue
		}
		top := rows[0]
		if top.Total <= 0 { // no real leader yet (no points scored)
			continue
		}
		st := r.storedLeader(lg.Id)
		if top.UserID == st.Leader {
			continue // unchanged
		}
		// Genuine takeover — persist immediately so it fires only once.
		st = leaderState{Leader: top.UserID, Seq: st.Seq + 1}
		r.setStoredLeader(lg.Id, st)

		u, ok := byID[top.UserID]
		if !ok {
			continue // new leader is a bot / not eligible / not allowlisted
		}
		data := tplData{
			League:  lg.GetString("name"),
			Total:   top.Total,
			CTAText: "See the leaderboard",
			CTAUrl:  base.url + "/leagues/" + lg.Id,
		}
		dedupKey := "league_lead:" + lg.Id + ":" + top.UserID + ":" + strconv.Itoa(st.Seq)
		r.dispatch(ctx, res, ncol, u, "league_lead", dedupKey, data)
	}
	return nil
}

// detectResultsRecap sends a once-daily digest (gated to the recap hour by the
// caller) summarising points earned from matches finalized in the last 24h plus
// the user's current league rankings.
func (r *Runner) detectResultsRecap(ctx context.Context, res *Result, now time.Time,
	matches []*core.Record, recipients []*core.Record, base baseInfo) error {

	// Recently-resolved matches = finished and kicked off within the last 24h.
	// Keying off kickoff (fixed schedule data) rather than finalizedAt keeps
	// consecutive daily windows gap-free and makes the recap behave correctly
	// under the dev virtual clock (finalizedAt is stamped with real wall-time).
	since := now.Add(-24 * time.Hour)
	finalized := map[string]bool{}
	for _, m := range matches {
		if m.GetString("status") != "finished" {
			continue
		}
		ko := m.GetDateTime("kickoff").Time()
		if !ko.Before(since) && ko.Before(now) {
			finalized[m.Id] = true
		}
	}
	if len(finalized) == 0 {
		return nil // nothing happened — no empty recaps
	}

	cfgID := r.defaultConfigID()
	if cfgID == "" {
		return fmt.Errorf("no default scoring config")
	}
	ncol, err := r.notificationsCol()
	if err != nil {
		return err
	}
	dateKey := now.Format("2006-01-02")
	boards := map[string][]scoring.Row{} // league id -> rows (cached this pass)

	for _, u := range recipients {
		// Only recap users who actually participate (have at least one tip).
		if n, _ := r.app.CountRecords("tips", dbx.HashExp{"user": u.Id}); n == 0 {
			continue
		}

		gained, total := r.userPoints(u.Id, cfgID, finalized)
		ranks := r.userRanks(u.Id, boards)

		data := tplData{
			Finalized:    len(finalized),
			PointsGained: gained,
			Total:        total,
			Ranks:        ranks,
			CTAText:      "See the leaderboard",
			CTAUrl:       base.url + "/leagues",
		}
		r.dispatch(ctx, res, ncol, u, "results_recap", "results_recap:"+u.Id+":"+dateKey, data)
	}
	return nil
}
