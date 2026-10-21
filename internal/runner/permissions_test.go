package runner

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"ssh-mcp/internal/approval"
	"ssh-mcp/internal/sshtransport"
	"ssh-mcp/internal/store"
)

func TestConfirmationRequiresBrokerAndSuccessfulAudit(t *testing.T) {
	for _, kind := range []string{"no_broker", "request_audit", "approval_audit"} {
		t.Run(kind, func(t *testing.T) {
			deps := newFakeDependencies()
			deps.targets.sshTarget.PermissionTier = store.PermissionConfirm
			engine := deps.engine()
			b := approval.New()
			if kind != "no_broker" {
				engine.deps.Approvals = b
			}
			if kind == "request_audit" {
				deps.audit.err = errors.New("audit unavailable")
			}
			engine.deps.OpenTUI = func() error {
				p := b.Pending()[0]
				if kind == "approval_audit" {
					deps.audit.err = errors.New("audit unavailable")
				}
				return b.Resolve(p.ID, true)
			}
			r, err := engine.RunSSH(context.Background(), SSHRequest{Target: "192.0.2.10", Command: "touch /tmp/a"})
			if err != nil || r.RemoteExecuted || deps.ssh.calls != 0 || r.Status != StatusRejected {
				t.Fatalf("result=%+v err=%v", r, err)
			}
		})
	}
}

func TestReadOnlyDeploymentNeverInspectsOrUploadsSource(t *testing.T) {
	deps := newFakeDependencies()
	deps.targets.sshTarget.PermissionTier = store.PermissionReadOnly
	engine := deps.engine()
	deployer := &fakeSSHBinaryDeployer{}
	engine.deps.Deployer = deployer
	r, err := engine.DeploySSHBinary(context.Background(), SSHBinaryDeploymentRequest{Target: "192.0.2.10", SourcePath: "/nonexistent/source", RemotePath: "/srv/app/service"})
	if err != nil || r.Reason != "permission_read_only" || r.RemoteExecuted || deployer.calls != 0 {
		t.Fatalf("result=%+v err=%v", r, err)
	}
}

func TestSSHPermissionTiersEnforceBeforeDispatch(t *testing.T) {
	for _, tc := range []struct {
		name        string
		tier        store.PermissionTier
		command     string
		root        bool
		approved    bool
		wantCalls   int
		wantPrompts int
	}{
		{"read diagnostics", store.PermissionReadOnly, "free -m", false, false, 1, 0},
		{"read denies write", store.PermissionReadOnly, "touch /tmp/a", false, false, 0, 0},
		{"read denies elevation", store.PermissionReadOnly, "free -m", true, false, 0, 0},
		{"read denies script", store.PermissionReadOnly, "python3 -c 'print(1)'", false, true, 0, 0},
		{"default denies without confirmation", "", "touch /tmp/a", false, false, 0, 1},
		{"confirm diagnostics", store.PermissionConfirm, "free -m", false, false, 1, 0},
		{"confirm approved write", store.PermissionConfirm, "touch /tmp/a", false, true, 1, 1},
		{"confirm approved script", store.PermissionConfirm, "python3 -c 'print(1)'", false, true, 1, 1},
		{"confirm denied script", store.PermissionConfirm, "bash /srv/run.sh", false, false, 0, 1},
		{"confirm elevation", store.PermissionConfirm, "free -m", true, true, 1, 1},
		{"full script", store.PermissionFull, "python3 -c 'print(1)'", false, false, 1, 0},
		{"full destructive hard stop", store.PermissionFull, "rm -rf /usr", false, true, 0, 0},
		{"invalid tier", store.PermissionTier("other"), "free -m", false, true, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := newFakeDependencies()
			deps.targets.sshTarget.PermissionTier = tc.tier
			engine := deps.engine()
			b := approval.New()
			engine.deps.Approvals = b
			prompts := 0
			engine.deps.OpenTUI = func() error {
				prompts++
				p := b.Pending()
				if len(p) != 1 || p[0].Target != "192.0.2.10" || !strings.Contains(p[0].Details, tc.command) || deps.ssh.calls != 0 {
					t.Fatalf("pending=%+v calls=%d", p, deps.ssh.calls)
				}
				return b.Resolve(p[0].ID, tc.approved)
			}
			result, err := engine.RunSSH(context.Background(), SSHRequest{Target: "192.0.2.10", Command: tc.command, AsRoot: tc.root})
			if err != nil || deps.ssh.calls != tc.wantCalls || prompts != tc.wantPrompts || result.RemoteExecuted != (tc.wantCalls > 0) {
				t.Fatalf("result=%+v err=%v calls=%d prompts=%d", result, err, deps.ssh.calls, prompts)
			}
			if len(b.Pending()) != 0 {
				t.Fatal("approval leaked")
			}
		})
	}
}

func TestConfirmationCannotSurviveTargetChangeOrCancellation(t *testing.T) {
	for _, kind := range []string{"revision", "tier", "disabled", "cancel", "vault"} {
		t.Run(kind, func(t *testing.T) {
			deps := newFakeDependencies()
			deps.targets.sshTarget.PermissionTier = store.PermissionConfirm
			engine := deps.engine()
			b := approval.New()
			engine.deps.Approvals = b
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			engine.deps.OpenTUI = func() error {
				p := b.Pending()[0]
				switch kind {
				case "revision":
					deps.targets.sshTarget.Revision++
				case "tier":
					deps.targets.sshTarget.PermissionTier = store.PermissionFull
				case "disabled":
					deps.targets.sshTarget.Enabled = false
				case "cancel":
					cancel()
				case "vault":
					deps.sessions.err = store.ErrLocked
				}
				return b.Resolve(p.ID, true)
			}
			result, err := engine.RunSSH(ctx, SSHRequest{Target: "192.0.2.10", Command: "touch /tmp/a"})
			if err != nil || result.RemoteExecuted || deps.ssh.calls != 0 {
				t.Fatalf("stale approval dispatched: %+v %v", result, err)
			}
		})
	}
}

func TestSQLPermissionTiers(t *testing.T) {
	for _, tier := range []store.PermissionTier{store.PermissionReadOnly, store.PermissionConfirm, store.PermissionFull} {
		for _, approved := range []bool{false, true} {
			t.Run(string(tier)+map[bool]string{false: "/deny", true: "/allow"}[approved], func(t *testing.T) {
				deps := newFakeDependencies()
				deps.targets.databaseTarget.PermissionTier = tier
				engine := deps.engine()
				b := approval.New()
				engine.deps.Approvals = b
				prompts := 0
				engine.deps.OpenTUI = func() error {
					prompts++
					p := b.Pending()[0]
					if p.Protocol != "sql" || !strings.Contains(p.Details, "database=app") {
						t.Fatalf("pending=%+v", p)
					}
					return b.Resolve(p.ID, approved)
				}
				r, err := engine.RunSQL(context.Background(), SQLRequest{Target: "192.0.2.20:5432", Statement: "UPDATE jobs SET state='ready' WHERE id=1"})
				want := tier == store.PermissionFull || tier == store.PermissionConfirm && approved
				if err != nil || r.RemoteExecuted != want {
					t.Fatalf("result=%+v err=%v", r, err)
				}
				if tier == store.PermissionConfirm && prompts != 1 || tier != store.PermissionConfirm && prompts != 0 {
					t.Fatalf("prompts=%d", prompts)
				}
			})
		}
	}
}

func TestWorkSessionPermissionBindsDeclaredContext(t *testing.T) {
	deps := newFakeDependencies()
	deps.targets.sshTarget.PermissionTier = store.PermissionConfirm
	engine := deps.engine()
	b := approval.New()
	engine.deps.Approvals = b
	opened, err := engine.OpenSSHSession(context.Background(), OpenSSHSessionRequest{Target: "192.0.2.10"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.SetSSHSessionContext(context.Background(), SetSSHSessionContextRequest{SessionID: opened.Session.ID, Context: SSHSessionContext{WorkingDirectory: "/srv/app", Environment: map[string]string{"APP_ENV": "staging"}}})
	if err != nil {
		t.Fatal(err)
	}
	engine.deps.OpenTUI = func() error {
		p := b.Pending()[0]
		if p.Operation != "execute_ssh_session" || !strings.Contains(p.Details, "/srv/app") || !strings.Contains(p.Details, "APP_ENV:staging") || !strings.Contains(p.Details, opened.Session.ID) {
			t.Fatalf("pending=%+v", p)
		}
		return b.Resolve(p.ID, true)
	}
	r, err := engine.ExecuteSSHSession(context.Background(), ExecuteSSHSessionRequest{SessionID: opened.Session.ID, Command: "free -m"})
	if err != nil || !r.RemoteExecuted {
		t.Fatalf("result=%+v err=%v", r, err)
	}
}

func TestDeploymentApprovalBindsImmutableSource(t *testing.T) {
	deps := newFakeDependencies()
	deps.targets.sshTarget.PermissionTier = store.PermissionConfirm
	content := []byte("approved bytes")
	source := writeDeploymentSource(t, content)
	b := approval.New()
	deployer := &fakeSSHBinaryDeployer{result: sshtransport.BinaryDeploymentResult{RemotePath: "/srv/app/service", BackupPath: "/srv/app/.ssh-mcp-backup-1", BytesUploaded: int64(len(content)), SHA256: digest(content), Activated: true}}
	engine := deps.engine()
	engine.deps.Approvals = b
	engine.deps.Deployer = deployer
	engine.deps.OpenTUI = func() error {
		p := b.Pending()[0]
		if !strings.Contains(p.Details, digest(content)) || !strings.Contains(p.Details, "/srv/app/service") {
			t.Fatalf("pending=%+v", p)
		}
		if err := os.WriteFile(source, []byte("changed after review"), 0600); err != nil {
			t.Fatal(err)
		}
		return b.Resolve(p.ID, true)
	}
	r, err := engine.DeploySSHBinary(context.Background(), SSHBinaryDeploymentRequest{Target: "192.0.2.10", SourcePath: source, RemotePath: "/srv/app/service"})
	if err != nil || r.Status != StatusCompleted || !bytes.Equal(deployer.source, content) {
		t.Fatalf("result=%+v err=%v uploaded=%q", r, err, deployer.source)
	}
}
