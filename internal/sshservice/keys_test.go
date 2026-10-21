package sshservice

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"ssh-mcp/internal/sshtransport"
	"ssh-mcp/internal/store"
)

func testPrivateKey(t *testing.T, path string, encrypted bool) ssh.Signer {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(key, "test-only")
	if encrypted {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(key, "test-only", []byte("test passphrase"))
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func TestEncryptedKeyUnlockLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	expected := testPrivateKey(t, path, true)
	service := New(nil, nil)
	defer service.Close()
	opened := 0
	service.SetKeyUnlockOpener(func() error { opened++; return nil })
	for range 2 {
		if _, err := service.keySigner(path, nil); !errors.Is(err, ErrKeyUnlockRequired) {
			t.Fatalf("locked: %v", err)
		}
	}
	if opened != 1 || len(service.PendingKeys()) != 1 {
		t.Fatalf("opened=%d pending=%v", opened, service.PendingKeys())
	}
	if err := service.UnlockKey(path, []byte("wrong")); !errors.Is(err, ErrInvalidPrivateKey) {
		t.Fatalf("wrong passphrase: %v", err)
	}
	if err := service.UnlockKey(filepath.Join(t.TempDir(), "unrequested"), []byte("test passphrase")); !errors.Is(err, ErrInvalidPrivateKey) {
		t.Fatal(err)
	}
	if err := service.UnlockKey(path, []byte("test passphrase")); err != nil {
		t.Fatal(err)
	}
	signer, err := service.keySigner(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := signer.Sign(rand.Reader, []byte("challenge"))
	if err != nil {
		t.Fatal(err)
	}
	if err := expected.PublicKey().Verify([]byte("challenge"), signature); err != nil {
		t.Fatal(err)
	}
	if len(service.PendingKeys()) != 0 {
		t.Fatal("key still pending")
	}
	// File replacement must never reuse the old signer.
	testPrivateKey(t, path, true)
	if _, err := service.keySigner(path, nil); !errors.Is(err, ErrKeyUnlockRequired) {
		t.Fatalf("replacement: %v", err)
	}
	if err := service.UnlockKey(path, []byte("test passphrase")); err != nil {
		t.Fatal(err)
	}
	service.Suspend()
	if err := service.UnlockKey(path, []byte("test passphrase")); !errors.Is(err, ErrInvalidPrivateKey) {
		t.Fatalf("pending survived lock: %v", err)
	}
	service.Resume()
	if _, err := service.keySigner(path, nil); !errors.Is(err, ErrKeyUnlockRequired) {
		t.Fatalf("cache survived lock: %v", err)
	}
	// A parse started before lock cannot repopulate the cache or pending paths.
	service.keys.mu.Lock()
	oldGeneration := service.keys.generation
	service.keys.mu.Unlock()
	service.Suspend()
	if _, err := service.parseKey(path, []byte("test passphrase"), oldGeneration); !errors.Is(err, ErrKeyUnlockRequired) {
		t.Fatal(err)
	}
	if len(service.PendingKeys()) != 0 {
		t.Fatal("stale parse queued key")
	}
}

func TestPrivateKeyLoginAgainstLoopbackSSH(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	dir := t.TempDir()
	path := filepath.Join(dir, "client key")
	clientKey := testPrivateKey(t, path, true)
	hostKey := testPrivateKey(t, filepath.Join(dir, "host"), false)
	var authenticated, executed atomic.Int32
	config := &ssh.ServerConfig{PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if meta.User() != "ops" || !bytes.Equal(key.Marshal(), clientKey.PublicKey().Marshal()) {
			return nil, fmt.Errorf("unrecognized public key")
		}
		authenticated.Add(1)
		return nil, nil
	}}
	config.AddHostKey(hostKey)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			raw, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer raw.Close()
				connection, channels, requests, err := ssh.NewServerConn(raw, config)
				if err != nil {
					return
				}
				defer connection.Close()
				go ssh.DiscardRequests(requests)
				for channel := range channels {
					if channel.ChannelType() != "session" {
						channel.Reject(ssh.UnknownChannelType, "session only")
						continue
					}
					ch, reqs, err := channel.Accept()
					if err != nil {
						continue
					}
					go func() {
						defer ch.Close()
						for req := range reqs {
							if req.Type != "exec" {
								req.Reply(false, nil)
								continue
							}
							executed.Add(1)
							req.Reply(true, nil)
							io.WriteString(ch, "key-auth-ok\n")
							ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
							return
						}
					}()
				}
			}()
		}
	}()
	database, err := store.Open(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	vault, err := database.Initialize(ctx, []byte("test master password"))
	if err != nil {
		t.Fatal(err)
	}
	defer vault.Lock()
	service := New(database, NativeTransport{})
	defer service.Close()
	target := store.SSHTarget{IP: "127.0.0.1", Mode: store.SSHDirect, SSHPort: listener.Addr().(*net.TCPAddr).Port, LoginUsername: "ops", AuthMethod: "private_key", PrivateKeyPath: path, Enabled: true}
	if _, err := service.TestTarget(ctx, vault, target, nil, ""); !errors.Is(err, ErrKeyUnlockRequired) || !errors.Is(err, sshtransport.ErrNotDispatched) {
		t.Fatalf("locked candidate: %v", err)
	}
	if authenticated.Load() != 0 || executed.Load() != 0 {
		t.Fatal("connected before unlock")
	}
	if err := service.UnlockKey(path, []byte("test passphrase")); err != nil {
		t.Fatal(err)
	}
	result, err := service.TestTarget(ctx, vault, target, nil, "")
	if err != nil || !result.RequiresFingerprintConfirmation {
		t.Fatalf("probe: %+v %v", result, err)
	}
	if result.Fingerprint != ssh.FingerprintSHA256(hostKey.PublicKey()) || executed.Load() != 0 {
		t.Fatal("probe authenticated or wrong fingerprint")
	}
	result, err = service.TestTarget(ctx, vault, target, nil, result.Fingerprint)
	if err != nil || result.RequiresFingerprintConfirmation {
		t.Fatalf("confirmed: %+v %v", result, err)
	}
	saved, err := vault.CommitValidatedSSHTargetConfiguration(ctx, store.ValidatedSSHTargetConfiguration{Target: target, ConfirmedFingerprint: result.Fingerprint})
	if err != nil {
		t.Fatal(err)
	}
	output, err := service.Execute(ctx, vault, saved, "printf key-auth-ok", false, 1024)
	if err != nil || output.Stdout != "key-auth-ok\n" || authenticated.Load() == 0 {
		t.Fatalf("execute: %+v %v", output, err)
	}
	service.Suspend()
	service.Resume()
	before := executed.Load()
	if _, err := service.Execute(ctx, vault, saved, "printf no-replay", false, 1024); !errors.Is(err, ErrKeyUnlockRequired) {
		t.Fatal(err)
	}
	if executed.Load() != before {
		t.Fatal("remote command dispatched after lock")
	}
}
