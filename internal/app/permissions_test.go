package app

import (
	"context"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"path/filepath"
	"testing"
	"time"

	"ssh-mcp/internal/approval"
	"ssh-mcp/internal/bridge"
	"ssh-mcp/internal/control"
	"ssh-mcp/internal/ipc"
	"ssh-mcp/internal/paths"
	"ssh-mcp/internal/runner"
	"ssh-mcp/internal/store"
)

type confirmationMCPTestTargets struct{ mcpTestTargets }

func (t confirmationMCPTestTargets) SSHTarget(ctx context.Context, target string) (store.SSHTarget, error) {
	result, err := t.mcpTestTargets.SSHTarget(ctx, target)
	result.PermissionTier = store.PermissionConfirm
	return result, err
}

func TestMCPCallWaitsForLocalUserDecisionBeforeDispatch(t *testing.T) {
	for _, approved := range []bool{false, true} {
		t.Run(map[bool]string{false: "deny", true: "allow"}[approved], func(t *testing.T) {
			base := shortRuntimeDir(t)
			roots, err := paths.Prepare(filepath.Join(base, "config"), filepath.Join(base, "runtime"))
			if err != nil {
				t.Fatal(err)
			}
			r, err := NewRuntime(RuntimeOptions{Roots: roots})
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			if err := r.Start(); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			local := ipc.NewClient(r.SocketPath(), r.Token())
			var unlock control.UnlockResult
			if err := local.Call(ctx, "unlock", control.UnlockParams{MasterPassword: "test-approval-vault-password"}, &unlock); err != nil {
				t.Fatal(err)
			}
			ready := make(chan struct{})
			remote := &countingMCPTestSSH{}
			engine := runner.New(runner.Dependencies{Targets: confirmationMCPTestTargets{}, Sessions: r.sessions, SSH: remote, Approvals: r.approvals, Audit: r.audit, OpenTUI: func() error { close(ready); return nil }})
			server := NewServer("permission-integration", engine)
			st, ct := mcp.NewInMemoryTransports()
			serverDone := make(chan error, 1)
			go func() { serverDone <- server.Run(ctx, st) }()
			defer func() { cancel(); <-serverDone }()
			client := mcp.NewClient(&mcp.Implementation{Name: "permission-test", Version: "1"}, nil)
			session, err := client.Connect(ctx, ct, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			type response struct {
				result *mcp.CallToolResult
				err    error
			}
			done := make(chan response, 1)
			go func() {
				result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "run_ssh", Arguments: map[string]any{"target": "192.0.2.10", "command": "python3 -c 'print(1)'"}})
				done <- response{result, err}
			}()
			select {
			case <-ready:
			case <-ctx.Done():
				t.Fatal("MCP did not reach local confirmation", ctx.Err())
			}
			if remote.calls != 0 {
				t.Fatal("remote operation dispatched before confirmation")
			}
			var status control.Status
			if err := local.Call(ctx, "status", nil, &status); err != nil {
				t.Fatal(err)
			}
			if len(status.PendingApprovals) != 1 {
				t.Fatalf("pending=%+v", status.PendingApprovals)
			}
			params := struct {
				ID       string `json:"id"`
				Approved bool   `json:"approved"`
			}{status.PendingApprovals[0].ID, approved}
			if err := local.Call(ctx, "approval.resolve", params, &struct{}{}); err != nil {
				t.Fatal(err)
			}
			result := <-done
			want := runner.StatusRejected
			calls := 0
			if approved {
				want = runner.StatusCompleted
				calls = 1
			}
			if result.err != nil || result.result == nil || mcpResultStatus(t, result.result) != want || remote.calls != calls {
				t.Fatalf("result=%+v err=%v calls=%d", result.result, result.err, remote.calls)
			}
		})
	}
}

func TestApprovalOnlyResolvesThroughUnlockedAuthenticatedControl(t *testing.T) {
	base := shortRuntimeDir(t)
	roots, err := paths.Prepare(filepath.Join(base, "config"), filepath.Join(base, "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewRuntime(RuntimeOptions{Roots: roots})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- r.approvals.Wait(ctx, approval.Request{Owner: "test-owner", Target: "192.0.2.10", Details: "private approval details"}, func() error { close(ready); return nil })
	}()
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	p := r.approvals.Pending()[0]
	params := struct {
		ID       string `json:"id"`
		Approved bool   `json:"approved"`
	}{p.ID, true}
	client, err := bridge.Connect(ctx, r.BridgeSocketPath())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close(context.Background())
	if err := client.Call(ctx, "approval.resolve", params, &struct{}{}); !errors.Is(err, bridge.ErrMethodNotFound) {
		t.Fatalf("bridge approval=%v", err)
	}
	var status DaemonStatus
	if err := client.Call(ctx, bridgeMethodStatus, struct{}{}, &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Control.PendingApprovals) != 0 {
		t.Fatal("bridge leaked control-plane approval requests")
	}
	bad := ipc.NewClient(r.SocketPath(), "wrong-control-token")
	if err := bad.Call(ctx, "approval.resolve", params, &struct{}{}); err == nil {
		t.Fatal("unauthenticated approval accepted")
	}
	local := ipc.NewClient(r.SocketPath(), r.Token())
	if err := local.Call(ctx, "approval.resolve", params, &struct{}{}); err == nil {
		t.Fatal("locked approval accepted")
	}
	var unlocked control.UnlockResult
	if err := local.Call(ctx, "unlock", control.UnlockParams{MasterPassword: "test-approval-vault-password"}, &unlocked); err != nil {
		t.Fatal(err)
	}
	var localStatus control.Status
	if err := local.Call(ctx, "status", nil, &localStatus); err != nil {
		t.Fatal(err)
	}
	if len(localStatus.PendingApprovals) != 1 || localStatus.PendingApprovals[0].ID != p.ID {
		t.Fatalf("local status=%+v", localStatus)
	}
	if err := local.Call(ctx, "approval.resolve", params, &struct{}{}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := local.Call(ctx, "approval.resolve", params, &struct{}{}); err == nil {
		t.Fatal("confirmation replay accepted")
	}
}

func TestTUIDisconnectCancelsPendingApproval(t *testing.T) {
	base := shortRuntimeDir(t)
	roots, err := paths.Prepare(filepath.Join(base, "config"), filepath.Join(base, "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewRuntime(RuntimeOptions{Roots: roots})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	ready := make(chan struct{})
	go func() { done <- r.approvals.Wait(ctx, approval.Request{}, func() error { close(ready); return nil }) }()
	<-ready
	r.clearTUIActivity(time.Now())
	if err := <-done; !errors.Is(err, approval.ErrDenied) {
		t.Fatalf("disconnect=%v", err)
	}
	if len(r.approvals.Pending()) != 0 {
		t.Fatal("disconnected approval retained")
	}
}
