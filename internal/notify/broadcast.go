package notify

import (
	"context"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// Broadcast fans one announcement out as a real notification (email + push) to
// every eligible user, reusing the per-channel dispatch (prefs, dedup ledger,
// delivery). The dedupKey is keyed off the announcement id, so the unique
// (dedupKey, channel) index makes a second "Send" a no-op rather than a re-spam.
//
// It builds a fresh Runner (selecting the active mail/push providers) so it can
// be driven straight from the owner/admin endpoint without sharing the cron's.
func Broadcast(ctx context.Context, app core.App, ann *core.Record) (*Result, error) {
	r := New(app)
	res := &Result{}

	ncol, err := r.notificationsCol()
	if err != nil {
		return res, fmt.Errorf("notifications collection: %w", err)
	}
	recipients, err := r.eligibleUsers(readConfig(app).Allowlist)
	if err != nil {
		return res, fmt.Errorf("load recipients: %w", err)
	}

	base := r.base()
	dedupKey := "announcement:" + ann.Id
	highPriority := ann.GetBool("highPriority")
	for _, u := range recipients {
		data := tplData{
			Title:        ann.GetString("title"),
			Body:         ann.GetString("body"),
			HighPriority: highPriority,
			CTAText:      "Open WM Tips",
			CTAUrl:       base.url + "/",
		}
		r.dispatch(ctx, res, ncol, u, "announcement", dedupKey, data)
	}
	return res, nil
}
