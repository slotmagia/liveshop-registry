package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateReusesExistingTrustBundle(t *testing.T) {
	output := t.TempDir()
	if err := generate(output, false, -1); err != nil {
		t.Fatalf("generate initial bundle: %v", err)
	}
	before, err := os.ReadFile(filepath.Join(output, "ca.pem"))
	if err != nil {
		t.Fatalf("read initial CA: %v", err)
	}

	if err := generate(output, false, -1); err != nil {
		t.Fatalf("reuse bundle: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(output, "ca.pem"))
	if err != nil {
		t.Fatalf("read reused CA: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("non-force generation rotated the existing CA")
	}
}

func TestGenerateRejectsCorruptExistingTrustBundle(t *testing.T) {
	output := t.TempDir()
	if err := generate(output, false, -1); err != nil {
		t.Fatalf("generate initial bundle: %v", err)
	}
	if err := os.WriteFile(filepath.Join(output, "identity-client.pem"), []byte("corrupt"), 0o600); err != nil {
		t.Fatalf("corrupt client certificate: %v", err)
	}

	if err := generate(output, false, -1); err == nil {
		t.Fatal("expected corrupt bundle to be rejected")
	}
}

func TestGenerateRotatesWhenRequiredFileMissing(t *testing.T) {
	output := t.TempDir()
	if err := generate(output, false, -1); err != nil {
		t.Fatalf("generate initial bundle: %v", err)
	}
	before, err := os.ReadFile(filepath.Join(output, "ca.pem"))
	if err != nil {
		t.Fatalf("read initial CA: %v", err)
	}
	if err := os.Remove(filepath.Join(output, "registry-server.pem")); err != nil {
		t.Fatalf("remove registry server certificate: %v", err)
	}

	if err := generate(output, false, -1); err != nil {
		t.Fatalf("rotate incomplete bundle: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(output, "ca.pem"))
	if err != nil {
		t.Fatalf("read rotated CA: %v", err)
	}
	if bytes.Equal(before, after) {
		t.Fatal("incomplete bundle was reused instead of rotated")
	}
	if _, err := os.Stat(filepath.Join(output, "registry-server.pem")); err != nil {
		t.Fatalf("rotated bundle missing registry-server.pem: %v", err)
	}
}
