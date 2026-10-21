package approval

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOnlyOneDecisionCanResolveExactRequest(t *testing.T) {
	for _, approved := range []bool{false, true} {
		t.Run(map[bool]string{false: "deny", true: "allow"}[approved], func(t *testing.T) {
			b := New()
			var id string
			err := b.Wait(context.Background(), Request{Target: "192.0.2.10", Owner: "owner", Details: "touch /tmp/a"}, func() error {
				p := b.Pending()
				if len(p) != 1 || p[0].Details != "touch /tmp/a" {
					t.Fatalf("pending=%+v", p)
				}
				id = p[0].ID
				if b.Resolve("wrong-id", true) == nil {
					t.Fatal("unknown approval accepted")
				}
				if err := b.Resolve(id, approved); err != nil {
					t.Fatal(err)
				}
				if b.Resolve(id, true) == nil {
					t.Fatal("approval replay accepted")
				}
				return nil
			})
			if approved && err != nil || !approved && !errors.Is(err, ErrDenied) {
				t.Fatalf("approved=%t err=%v", approved, err)
			}
			if len(b.Pending()) != 0 || b.Resolve(id, true) == nil {
				t.Fatal("completed request remained reusable")
			}
		})
	}
}

func TestCancellationOwnerExitExpiryAndUnavailableUIFailClosed(t *testing.T) {
	for _, kind := range []string{"context", "owner", "lock", "expired", "ui_error", "no_ui"} {
		t.Run(kind, func(t *testing.T) {
			b := New()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			open := func() error {
				p := b.Pending()[0]
				switch kind {
				case "context":
					cancel()
					_ = b.Resolve(p.ID, true)
				case "owner":
					b.CancelOwner("other")
					if len(b.Pending()) != 1 {
						t.Fatal("other owner cancelled request")
					}
					b.CancelOwner("owner")
				case "lock":
					b.CancelAll()
				case "expired":
					b.mu.Lock()
					b.pending[p.ID].request.ExpiresAt = time.Now().Add(-time.Second)
					b.mu.Unlock()
					if b.Resolve(p.ID, true) == nil {
						t.Fatal("expired request approved")
					}
				case "ui_error":
					return errors.New("no terminal")
				}
				return nil
			}
			if kind == "no_ui" {
				open = nil
			}
			if err := b.Wait(ctx, Request{Owner: "owner"}, open); !errors.Is(err, ErrDenied) {
				t.Fatalf("err=%v", err)
			}
			if len(b.Pending()) != 0 {
				t.Fatal("pending request leaked")
			}
		})
	}
}

func TestBrokerWaitStopsAtCallerDeadline(t *testing.T) {
	b := New()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := b.Wait(ctx, Request{}, func() error { return nil }); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	if len(b.Pending()) != 0 {
		t.Fatal("pending request leaked")
	}
}
