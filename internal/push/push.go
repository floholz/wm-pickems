package push

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// ErrGone signals that a subscription endpoint is permanently dead (HTTP 404 /
// 410) and should be pruned by the caller.
var ErrGone = errors.New("push: subscription gone")

// Subscription is a single browser push endpoint.
type Subscription struct {
	Endpoint string
	P256dh   string
	Auth     string
}

// Notification is the payload delivered to the service worker.
type Notification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag,omitempty"`
}

// Sender delivers a Notification to one subscription.
type Sender interface {
	Enabled() bool
	Send(ctx context.Context, sub Subscription, n Notification) error
}

type webPushSender struct {
	keys Keys
}

// NewSender builds a Sender from the resolved VAPID keys. The returned Sender is
// always non-nil; when no keys are available Enabled() is false and Send errors.
func NewSender(keys Keys) Sender {
	keys.Subject = normalizeSubject(keys.Subject)
	return &webPushSender{keys: keys}
}

func (s *webPushSender) Enabled() bool { return s.keys.Enabled() }

func (s *webPushSender) Send(ctx context.Context, sub Subscription, n Notification) error {
	if !s.keys.Enabled() {
		return fmt.Errorf("push: no VAPID keys configured")
	}
	payload, err := json.Marshal(n)
	if err != nil {
		return err
	}
	resp, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, &webpush.Options{
		Subscriber:      s.keys.Subject,
		VAPIDPublicKey:  s.keys.Public,
		VAPIDPrivateKey: s.keys.Private,
		TTL:             24 * 60 * 60, // 1 day; reminders are time-bound
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return ErrGone
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	default:
		return fmt.Errorf("push: endpoint returned %d", resp.StatusCode)
	}
}
