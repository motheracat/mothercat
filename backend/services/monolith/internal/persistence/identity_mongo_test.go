package persistence

import (
	"encoding/hex"
	"testing"
)

func TestHashToken(t *testing.T) {
	const token = "session-secret"
	const expected = "135fa7d67146ca540f17e51e101d45de5b1edde8ea8a13f9c7b93b71aa21f8a4"
	if got := hashToken(token); got != expected {
		t.Fatalf("hashToken(%q) = %q, want %q", token, got, expected)
	}
	if hashToken(token) == token {
		t.Fatal("hashToken returned the plaintext token")
	}
}

func TestNewUUID(t *testing.T) {
	value, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		t.Fatalf("newUUID() returned malformed UUID %q", value)
	}
	raw, err := hex.DecodeString(value[:8] + value[9:13] + value[14:18] + value[19:23] + value[24:])
	if err != nil || len(raw) != 16 {
		t.Fatalf("newUUID() returned invalid hexadecimal UUID %q", value)
	}
	if value[14] != '4' {
		t.Fatalf("newUUID() version = %q, want 4", value[14])
	}
}
