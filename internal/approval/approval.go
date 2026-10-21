// Package approval owns ephemeral user decisions. Only the authenticated local
// TUI control channel can resolve a pending request; no MCP approval tool exists.
package approval

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"
)

var ErrDenied = errors.New("local user did not approve the operation")

const Timeout = 2 * time.Minute

type Request struct {
	ID        string    `json:"id"`
	Owner     string    `json:"owner"`
	Target    string    `json:"target"`
	Protocol  string    `json:"protocol"`
	Revision  int64     `json:"revision"`
	Operation string    `json:"operation"`
	Details   string    `json:"details"`
	ExpiresAt time.Time `json:"expires_at"`
}

type pending struct {
	request Request
	answer  chan bool
}
type Broker struct {
	mu      sync.Mutex
	pending map[string]*pending
}

func New() *Broker { return &Broker{pending: make(map[string]*pending)} }

// Wait holds the exact operation's caller stack, including deployment source
// snapshot and session lease. Nothing is serialized for a later AI retry.
func (b *Broker) Wait(ctx context.Context, request Request, open func() error) error {
	if b == nil || ctx.Err() != nil {
		return ErrDenied
	}
	var id [24]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	request.ID = hex.EncodeToString(id[:])
	request.ExpiresAt = time.Now().Add(Timeout)
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(request.ExpiresAt) {
		request.ExpiresAt = deadline
	}
	p := &pending{request: request, answer: make(chan bool, 1)}
	b.mu.Lock()
	if len(b.pending) >= 32 {
		b.mu.Unlock()
		return ErrDenied
	}
	b.pending[request.ID] = p
	b.mu.Unlock()
	defer func() { b.mu.Lock(); delete(b.pending, request.ID); b.mu.Unlock() }()
	if open == nil {
		return ErrDenied
	}
	if err := open(); err != nil {
		return ErrDenied
	}
	timer := time.NewTimer(time.Until(request.ExpiresAt))
	defer timer.Stop()
	select {
	case approved := <-p.answer:
		if approved && ctx.Err() == nil && time.Now().Before(request.ExpiresAt) {
			return nil
		}
	case <-ctx.Done():
	case <-timer.C:
	}
	return ErrDenied
}

func (b *Broker) Pending() []Request {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var result []Request
	for _, p := range b.pending {
		if time.Now().Before(p.request.ExpiresAt) {
			result = append(result, p.request)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ExpiresAt.Before(result[j].ExpiresAt) })
	return result
}

// Resolve consumes the decision before waking the operation. Unknown, expired,
// duplicate and cancelled requests fail closed.
func (b *Broker) Resolve(id string, approved bool) error {
	if b == nil {
		return ErrDenied
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	p, ok := b.pending[id]
	if !ok {
		return ErrDenied
	}
	delete(b.pending, id)
	if !time.Now().Before(p.request.ExpiresAt) {
		p.answer <- false
		return ErrDenied
	}
	p.answer <- approved
	return nil
}

func (b *Broker) CancelOwner(owner string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, p := range b.pending {
		if p.request.Owner == owner {
			delete(b.pending, id)
			p.answer <- false
		}
	}
}

func (b *Broker) CancelAll() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, p := range b.pending {
		delete(b.pending, id)
		p.answer <- false
	}
}
