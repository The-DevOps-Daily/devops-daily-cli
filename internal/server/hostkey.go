package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	gossh "golang.org/x/crypto/ssh"
)

// LoadOrCreateHostKey reads the ed25519 host key at path. If there is no
// file yet it generates one and writes it (mode 0600) with a .pub next to it.
// created reports whether a new key was made.
func LoadOrCreateHostKey(path string) (signer gossh.Signer, created bool, err error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		if err := writeNewHostKey(path); err != nil {
			return nil, false, err
		}
		created = true
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, false, err
	}
	signer, err = gossh.ParsePrivateKey(b)
	if err != nil {
		return nil, false, fmt.Errorf("host key %s: %w", path, err)
	}
	if t := signer.PublicKey().Type(); t != gossh.KeyAlgoED25519 {
		return nil, false, fmt.Errorf("host key %s is %s, want ed25519", path, t)
	}
	return signer, created, nil
}

// LoadHostKey reads an existing host key without creating one.
func LoadHostKey(path string) (gossh.Signer, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return gossh.ParsePrivateKey(b)
}

func writeNewHostKey(path string) error {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	block, err := gossh.MarshalPrivateKey(priv, "ssh-site host key")
	if err != nil {
		return err
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := writeAtomic(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return err
	}
	return writeAtomic(path+".pub", gossh.MarshalAuthorizedKey(signer.PublicKey()), 0o644)
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// Fingerprint returns the SHA256 fingerprint as ssh-keygen -l prints it.
func Fingerprint(s gossh.Signer) string {
	return gossh.FingerprintSHA256(s.PublicKey())
}
