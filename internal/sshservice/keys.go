package sshservice

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"ssh-mcp/internal/secret"
)

var ErrKeyUnlockRequired = errors.New("SSH private key requires local unlock")
var ErrInvalidPrivateKey = errors.New("unable to read or unlock SSH private key")

type cachedKey struct {
	digest [32]byte
	signer ssh.Signer
}
type keyCache struct {
	mu         sync.Mutex
	generation uint64
	unlocked   map[string]cachedKey
	pending    map[string]bool
	opener     func() error
}

func keyPath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", ErrInvalidPrivateKey
		}
		path = filepath.Join(home, path[2:])
	}
	if !filepath.IsAbs(path) || strings.ContainsAny(path, "\x00\r\n") {
		return "", ErrInvalidPrivateKey
	}
	return filepath.Clean(path), nil
}

// SetKeyUnlockOpener connects only to the local console, never the MCP bridge.
func (s *Service) SetKeyUnlockOpener(open func() error) {
	s.keys.mu.Lock()
	defer s.keys.mu.Unlock()
	s.keys.opener = open
}

func (s *Service) PendingKeys() []string {
	s.keys.mu.Lock()
	defer s.keys.mu.Unlock()
	paths := make([]string, 0, len(s.keys.pending))
	for path := range s.keys.pending {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func (s *Service) UnlockKey(path string, passphrase []byte) error {
	path, err := keyPath(path)
	if err != nil {
		return err
	}
	s.keys.mu.Lock()
	pending := s.keys.pending[path]
	generation := s.keys.generation
	s.keys.mu.Unlock()
	if !pending {
		return ErrInvalidPrivateKey
	}
	_, err = s.parseKey(path, passphrase, generation)
	return err
}

func (s *Service) clearKeys() {
	s.keys.mu.Lock()
	defer s.keys.mu.Unlock()
	s.keys.generation++
	s.keys.unlocked = nil
	s.keys.pending = nil
}

func (s *Service) keySigner(path string, passphrase []byte) (ssh.Signer, error) {
	s.keys.mu.Lock()
	generation := s.keys.generation
	s.keys.mu.Unlock()
	return s.parseKey(path, passphrase, generation)
}

func (s *Service) parseKey(path string, passphrase []byte, generation uint64) (ssh.Signer, error) {
	path, err := keyPath(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, ErrInvalidPrivateKey
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1024*1024 {
		return nil, ErrInvalidPrivateKey
	}
	// Limit reads even if the file grows after Stat. Do not expose parser errors
	// or key material to remote results or audit logs.
	data, err := io.ReadAll(io.LimitReader(f, 1024*1024+1))
	if err != nil || len(data) > 1024*1024 {
		return nil, ErrInvalidPrivateKey
	}
	defer secret.Zero(data)
	digest := sha256.Sum256(data)
	s.keys.mu.Lock()
	if generation != s.keys.generation {
		s.keys.mu.Unlock()
		return nil, ErrKeyUnlockRequired
	}
	cached, ok := s.keys.unlocked[path]
	s.keys.mu.Unlock()
	if ok && cached.digest == digest && len(passphrase) == 0 {
		return cached.signer, nil
	}
	var signer ssh.Signer
	if len(passphrase) > 0 {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(data, passphrase)
	} else {
		signer, err = ssh.ParsePrivateKey(data)
	}
	if err != nil {
		var missing *ssh.PassphraseMissingError
		if !errors.As(err, &missing) {
			return nil, ErrInvalidPrivateKey
		}
		s.keys.mu.Lock()
		if generation != s.keys.generation {
			s.keys.mu.Unlock()
			return nil, ErrKeyUnlockRequired
		}
		alreadyPending := s.keys.pending[path]
		if s.keys.pending == nil {
			s.keys.pending = make(map[string]bool)
		}
		s.keys.pending[path] = true
		open := s.keys.opener
		s.keys.mu.Unlock()
		if open != nil && !alreadyPending {
			_ = open()
		}
		return nil, ErrKeyUnlockRequired
	}
	s.keys.mu.Lock()
	defer s.keys.mu.Unlock()
	if generation != s.keys.generation {
		return nil, fmt.Errorf("%w: session locked", ErrKeyUnlockRequired)
	}
	if s.keys.unlocked == nil {
		s.keys.unlocked = make(map[string]cachedKey)
	}
	s.keys.unlocked[path] = cachedKey{digest: digest, signer: signer}
	delete(s.keys.pending, path)
	return signer, nil
}
