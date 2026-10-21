package runner

import (
	"context"
	"fmt"
	"testing"

	"ssh-mcp/internal/sshservice"
	"ssh-mcp/internal/sshtransport"
)

func TestKeyUnlockHandoffIsNotDispatchedAndAudited(t *testing.T) {
	deps := newFakeDependencies()
	deps.ssh.err = fmt.Errorf("%w: %w", sshtransport.ErrNotDispatched, sshservice.ErrKeyUnlockRequired)
	result, err := deps.engine().RunSSH(context.Background(), SSHRequest{Target: "192.0.2.10", Command: "free -m"})
	if err != nil || result.Status != StatusUnlockRequired || result.ExecutionOutcome != StatusNotDispatched || result.RemoteExecuted || result.FailureKind != "ssh_key_unlock_required" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if deps.ssh.calls != 1 {
		t.Fatal("operation automatically retried")
	}
	if len(deps.audit.entries) < 2 {
		t.Fatal("missing terminal audit")
	}
	last := deps.audit.entries[len(deps.audit.entries)-1]
	if last.RemoteExecuted || last.Result.Status != StatusUnlockRequired {
		t.Fatalf("audit=%+v", last)
	}
}
