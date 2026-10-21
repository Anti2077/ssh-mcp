package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestSSHAuthenticationTransitions(t *testing.T) {
	ctx := context.Background()
	database, vault := openSSHConfigurationTestVault(t)
	target := saveSSHConfigurationTestTarget(t, database, vault, SSHTarget{IP: "192.0.2.90", Mode: SSHDirect, SSHPort: 22, LoginUsername: "ops", Enabled: true}, []byte("old-password"), "SHA256:host")
	oldID, oldRevision := target.CredentialID, target.Revision
	target.AuthMethod, target.PrivateKeyPath = "private_key", filepath.Join(t.TempDir(), "key")
	target, err := vault.CommitValidatedSSHTargetConfiguration(ctx, ValidatedSSHTargetConfiguration{Target: target, ConfirmedFingerprint: "SHA256:host"})
	if err != nil {
		t.Fatal(err)
	}
	if target.CredentialID != "" || target.Revision <= oldRevision {
		t.Fatalf("key configuration: %+v", target)
	}
	if _, err := vault.Credential(ctx, oldID); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("old password retained: %v", err)
	}
	listed, err := database.ListSSHTargets(ctx)
	if err != nil || len(listed) != 1 || listed[0].PrivateKeyPath != target.PrivateKeyPath || listed[0].AuthMethod != "private_key" {
		t.Fatalf("list: %+v %v", listed, err)
	}
	snapshot, err := database.SSHTarget(ctx, target.IP)
	if err != nil || snapshot.PrivateKeyPath != target.PrivateKeyPath {
		t.Fatal(err)
	}
	oldRevision = target.Revision
	target.AuthMethod = "password"
	target, err = vault.CommitValidatedSSHTargetConfiguration(ctx, ValidatedSSHTargetConfiguration{Target: target, Password: []byte("new-password"), ConfirmedFingerprint: "SHA256:host"})
	if err != nil {
		t.Fatal(err)
	}
	if target.CredentialID == "" || target.PrivateKeyPath != "" || target.Revision <= oldRevision {
		t.Fatalf("password configuration: %+v", target)
	}
	password, err := vault.Credential(ctx, target.CredentialID)
	if err != nil || string(password) != "new-password" {
		t.Fatal("password transition failed", err)
	}
}
