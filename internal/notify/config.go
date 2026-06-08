package notify

import (
	"os"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// config holds the resolved (defaults-applied) notify settings.
type config struct {
	LeadHours        int // reminder lead time before a deadline
	RecapHourUTC     int // hour (UTC) the daily results recap fires
	CountdownHourUTC int // hour (UTC) the daily pre-tournament countdown fires
	// Allowlist gates delivery to specific email addresses (lowercased) for a
	// gradual rollout. Empty = send to everyone.
	Allowlist []string
	// Channels are the master per-channel kill switches. Either false suppresses
	// every notification on that channel platform-wide (e.g. flip Email off while
	// the mail provider is suspended). Default: both on.
	Channels channelFlags
	// Disabled holds per-event overrides: Disabled[event][channel] == true
	// suppresses that one event's channel even when the master switch is on.
	Disabled map[string]map[string]bool
}

// channelFlags is the resolved master on/off for each delivery channel.
type channelFlags struct {
	Email bool
	Push  bool
}

// channelAllowed reports whether the platform currently permits delivery of
// `event` on `channel`. The master switch gates everything on that channel; a
// per-event override suppresses a single event's channel. User-level prefs are
// applied separately (and on top) at each send site.
func (c config) channelAllowed(event, channel string) bool {
	switch channel {
	case "email":
		if !c.Channels.Email {
			return false
		}
	case "push":
		if !c.Channels.Push {
			return false
		}
	}
	if ev := c.Disabled[event]; ev != nil && ev[channel] {
		return false
	}
	return true
}

// storedConfig is the on-disk shape (app_meta "notify_config"). Pointers let us
// tell "unset" apart from a legitimate zero (e.g. recapHourUTC = 0 = midnight).
type storedConfig struct {
	LeadHours        *int                       `json:"leadHours"`
	RecapHourUTC     *int                       `json:"recapHourUTC"`
	CountdownHourUTC *int                       `json:"countdownHourUTC"`
	Allowlist        []string                   `json:"allowlist"`
	Channels         *storedChannels            `json:"channels"`
	Disabled         map[string]map[string]bool `json:"disabled"`
}

// storedChannels is the on-disk master-switch shape. Pointers distinguish
// "unset" (defaults to on) from an explicit false.
type storedChannels struct {
	Email *bool `json:"email"`
	Push  *bool `json:"push"`
}

const (
	defaultLeadHours        = 12
	defaultRecapHourUTC     = 8
	defaultCountdownHourUTC = 9
	metaKey                 = "notify_config"
)

// readConfig loads the notify config from app_meta, applying defaults for any
// unset field. Settings are runtime-tunable from the PocketBase dashboard
// without a redeploy.
func readConfig(app core.App) config {
	rec, err := app.FindFirstRecordByFilter("app_meta",
		"key = {:k}", map[string]any{"k": metaKey})
	if err != nil {
		return applyConfigDefaults(storedConfig{})
	}
	var stored storedConfig
	if err := rec.UnmarshalJSONField("value", &stored); err != nil {
		return applyConfigDefaults(storedConfig{})
	}
	return applyConfigDefaults(stored)
}

// applyConfigDefaults fills unset/out-of-range fields with the defaults and
// resolves the allowlist (app_meta value, else the NOTIFY_ALLOWLIST env seed).
// Pure except for the env read, so the numeric defaults stay unit-testable.
func applyConfigDefaults(s storedConfig) config {
	c := config{
		LeadHours:        defaultLeadHours,
		RecapHourUTC:     defaultRecapHourUTC,
		CountdownHourUTC: defaultCountdownHourUTC,
		Channels:         channelFlags{Email: true, Push: true},
	}
	if s.Channels != nil {
		if s.Channels.Email != nil {
			c.Channels.Email = *s.Channels.Email
		}
		if s.Channels.Push != nil {
			c.Channels.Push = *s.Channels.Push
		}
	}
	c.Disabled = s.Disabled
	if s.LeadHours != nil && *s.LeadHours > 0 {
		c.LeadHours = *s.LeadHours
	}
	if s.RecapHourUTC != nil && *s.RecapHourUTC >= 0 && *s.RecapHourUTC <= 23 {
		c.RecapHourUTC = *s.RecapHourUTC
	}
	if s.CountdownHourUTC != nil && *s.CountdownHourUTC >= 0 && *s.CountdownHourUTC <= 23 {
		c.CountdownHourUTC = *s.CountdownHourUTC
	}
	c.Allowlist = normalizeEmails(s.Allowlist)
	if len(c.Allowlist) == 0 {
		c.Allowlist = normalizeEmails(strings.Split(os.Getenv("NOTIFY_ALLOWLIST"), ","))
	}
	return c
}

// normalizeEmails lowercases, trims, and drops empties.
func normalizeEmails(in []string) []string {
	out := make([]string, 0, len(in))
	for _, e := range in {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			out = append(out, e)
		}
	}
	return out
}
