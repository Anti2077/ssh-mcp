package tui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"ssh-mcp/internal/approval"
	"ssh-mcp/internal/control"
	"ssh-mcp/internal/store"
)

type permissionCaller struct {
	id       string
	approved bool
	calls    int
}

func (c *permissionCaller) Call(_ context.Context, method string, params any, _ any) error {
	if method == "approval.resolve" {
		c.calls++
		raw, _ := json.Marshal(params)
		var p struct {
			ID       string `json:"id"`
			Approved bool   `json:"approved"`
		}
		_ = json.Unmarshal(raw, &p)
		c.id = p.ID
		c.approved = p.Approved
	}
	return nil
}

func TestOperationConfirmationDefaultsToDenyAndBindsID(t *testing.T) {
	for _, allow := range []bool{false, true} {
		c := &permissionCaller{}
		m := NewModel(c)
		m.width = 80
		m.height = 32
		m.applyRPC(rpcMsg{action: "status", value: control.Status{Unlocked: true, PendingApprovals: []approval.Request{{ID: "exact-id", Target: "192.0.2.10", Protocol: "ssh", Details: "command=touch /tmp/a", ExpiresAt: time.Now().Add(time.Minute)}}}})
		if m.screen != screenOperationConfirm || m.confirmAccept {
			t.Fatal("confirmation did not default to deny")
		}
		if !strings.Contains(m.View().Content, "touch /tmp/a") {
			t.Fatal("operation not shown")
		}
		if allow {
			press(m, tea.KeyTab)
		}
		cmd := press(m, tea.KeyEnter)
		if cmd == nil {
			t.Fatal("no decision sent")
		}
		_ = cmd()
		if c.calls != 1 || c.id != "exact-id" || c.approved != allow {
			t.Fatalf("decision=%+v", c)
		}
	}
}

func TestOperationConfirmationRequiresFullReviewAndEscRejects(t *testing.T) {
	c := &permissionCaller{}
	m := NewModel(c)
	m.width = 60
	m.height = 24
	m.applyRPC(rpcMsg{action: "status", value: control.Status{Unlocked: true, PendingApprovals: []approval.Request{{ID: "long", Details: strings.Repeat("command line\n", 100), ExpiresAt: time.Now().Add(time.Minute)}}}})
	press(m, tea.KeyTab)
	if cmd := press(m, tea.KeyEnter); cmd != nil || c.calls != 0 {
		t.Fatal("approved unreviewed operation")
	}
	for range 110 {
		press(m, tea.KeyDown)
	}
	_ = m.View()
	cmd := press(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("final page cannot be approved")
	}
	_ = cmd()
	if !c.approved {
		t.Fatal("approval lost")
	}
	m.approvalBusy = false
	m.applyRPC(rpcMsg{action: "status", value: control.Status{Unlocked: true, PendingApprovals: []approval.Request{{ID: "escape", Details: "test", ExpiresAt: time.Now().Add(time.Minute)}}}})
	cmd = press(m, tea.KeyEsc)
	if cmd == nil {
		t.Fatal("Esc did not deny")
	}
	_ = cmd()
	if c.approved || c.id != "escape" {
		t.Fatalf("escape=%+v", c)
	}
}

func TestPermissionSelectorsAndTerminalEscaping(t *testing.T) {
	m := NewModel(nil)
	m.beginSSHForm(store.SSHTarget{})
	m.form.index = 10
	m.loadCurrentField()
	if m.form.fields[10].value != "confirm" {
		t.Fatal("unsafe default")
	}
	press(m, tea.KeyRight)
	if m.form.fields[10].value != "full" {
		t.Fatal("full tier not selectable")
	}
	press(m, tea.KeyRight)
	if m.form.fields[10].value != "read_only" {
		t.Fatal("readonly tier not selectable")
	}
	m.beginDatabaseForm(store.DatabaseInstance{})
	m.form.index = 12
	m.loadCurrentField()
	press(m, tea.KeyLeft)
	if m.form.fields[12].value != "read_only" {
		t.Fatal("database tier not selectable")
	}
	raw := "safe\x1b[2J\x1b[H\u202esecret\rhidden\nnext"
	displayed := approvalText(raw)
	if strings.ContainsAny(displayed, "\x1b\r\u202e") || !strings.Contains(displayed, "\\u001b") || !strings.Contains(displayed, "\nnext") {
		t.Fatalf("unsafe rendering=%q", displayed)
	}
}

func TestLateStatusCannotReopenConsumedConfirmation(t *testing.T) {
	c := &permissionCaller{}
	m := NewModel(c)
	m.width = 80
	m.height = 30
	status := control.Status{Unlocked: true, PendingApprovals: []approval.Request{{ID: "consumed", Details: "touch /tmp/a", ExpiresAt: time.Now().Add(time.Minute)}}}
	m.applyRPC(rpcMsg{action: "status", value: status})
	cmd := press(m, tea.KeyEsc)
	if cmd == nil {
		t.Fatal("no denial")
	}
	if press(m, tea.KeyEnter) != nil {
		t.Fatal("repeated Enter accepted while decision in flight")
	}
	m.applyRPC(rpcMsg{action: "approval_resolved", value: struct{}{}})
	m.applyRPC(rpcMsg{action: "status", value: status})
	if m.operationApproval != nil || m.screen == screenOperationConfirm {
		t.Fatal("late status reopened consumed confirmation")
	}
}
