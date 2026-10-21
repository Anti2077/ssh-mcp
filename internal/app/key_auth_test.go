package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/ssh"
	"ssh-mcp/internal/bridge"
	"ssh-mcp/internal/control"
	"ssh-mcp/internal/ipc"
	"ssh-mcp/internal/sshservice"
	"ssh-mcp/internal/store"
)

func TestRuntimeSharesKeyUnlockWithLocalConsoleOnly(t *testing.T) {
	ctx := context.Background()
	var opened atomic.Int32
	runtime := newStartedRuntime(t, RuntimeOptions{TUIOpener: func() error { opened.Add(1); return nil }})
	local := ipc.NewClient(runtime.SocketPath(), runtime.Token())
	var unlocked control.UnlockResult
	if err := local.Call(ctx, "unlock", control.UnlockParams{MasterPassword: "fixture-master-password"}, &unlocked); err != nil {
		t.Fatal(err)
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(key, "fixture", []byte("fixture-passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatal(err)
	}
	target := store.SSHTarget{IP: "127.0.0.1", Mode: store.SSHDirect, SSHPort: 22, LoginUsername: "ops", AuthMethod: "private_key", PrivateKeyPath: path, Enabled: true}
	var tested control.SSHTestResult
	if err := local.Call(ctx, "ssh.test_target", control.SSHTestParams{Target: target}, &tested); err != nil || !tested.RequiresKeyUnlock {
		t.Fatalf("candidate=%+v err=%v", tested, err)
	}
	if opened.Load() != 1 {
		t.Fatal("local console was not requested")
	}
	var status control.Status
	if err := local.Call(ctx, "status", nil, &status); err != nil || len(status.PendingKeys) != 1 || status.PendingKeys[0] != path {
		t.Fatalf("pending=%+v %v", status, err)
	}
	remote, err := bridge.Connect(ctx, runtime.BridgeSocketPath())
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close(ctx)
	executor := newBridgeExecutor(remote)
	bridgeStatus, err := executor.Status(ctx)
	if err != nil || len(bridgeStatus.Control.PendingKeys) != 0 {
		t.Fatal("bridge exposed local key paths", err)
	}
	params := struct {
		Path       string `json:"path"`
		Passphrase string `json:"passphrase"`
	}{path, "fixture-passphrase"}
	if err := remote.Call(ctx, "ssh.unlock_key", params, &struct{}{}); err == nil {
		t.Fatal("bridge allowed key unlock")
	}
	params.Passphrase = "wrong-fixture-passphrase"
	if err := local.Call(ctx, "ssh.unlock_key", params, &struct{}{}); err == nil {
		t.Fatal("wrong passphrase accepted")
	}
	params.Passphrase = "fixture-passphrase"
	if err := local.Call(ctx, "ssh.unlock_key", params, &struct{}{}); err != nil {
		t.Fatal(err)
	}
	service := runtime.ssh
	if len(service.PendingKeys()) != 0 {
		t.Fatal("control service did not share the runtime key cache")
	}
	if err := local.Call(ctx, "lock", nil, &struct{}{}); err != nil {
		t.Fatal(err)
	}
	if err := local.Call(ctx, "ssh.unlock_key", params, &struct{}{}); !errors.Is(err, ipc.ErrLocked) {
		t.Fatalf("unlock allowed while locked: %v", err)
	}
	if err := local.Call(ctx, "unlock", control.UnlockParams{MasterPassword: "fixture-master-password"}, &unlocked); err != nil {
		t.Fatal(err)
	}
	vault, err := runtime.sessions.Vault()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.TestTarget(ctx, vault, target, nil, ""); !errors.Is(err, sshservice.ErrKeyUnlockRequired) {
		t.Fatalf("signer survived vault lock: %v", err)
	}
}
