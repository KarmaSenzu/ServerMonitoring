package auth

import (
	"strings"
	"testing"
	"time"
)

func TestIssueParseRoundtrip(t *testing.T) {
	secret := []byte("test-secret-please-change")
	tok, err := Issue("user-123", "alice", "admin", secret, time.Minute)
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	if tok == "" {
		t.Fatal("Issue returned empty token")
	}

	claims, err := Parse(tok, secret)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if claims.Subject != "user-123" {
		t.Errorf("subject: got %q want %q", claims.Subject, "user-123")
	}
	if claims.Username != "alice" {
		t.Errorf("username: got %q want %q", claims.Username, "alice")
	}
	if claims.Role != "admin" {
		t.Errorf("role: got %q want %q", claims.Role, "admin")
	}
	if claims.ExpiresAt == nil {
		t.Fatal("ExpiresAt is nil")
	}
}

func TestParseWrongSecret(t *testing.T) {
	tok, err := Issue("u", "u", "viewer", []byte("right"), time.Minute)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := Parse(tok, []byte("wrong")); err == nil {
		t.Fatal("expected error parsing with wrong secret, got nil")
	}
}

func TestParseExpiredToken(t *testing.T) {
	secret := []byte("expiry-test")
	tok, err := Issue("u", "u", "viewer", secret, time.Millisecond)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	_, err = Parse(tok, secret)
	if err == nil {
		t.Fatal("expected error parsing expired token, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "expir") {
		t.Logf("expiry error message: %v", err)
	}
}

func TestIssueRejectsEmptySecret(t *testing.T) {
	if _, err := Issue("u", "u", "viewer", []byte(""), time.Minute); err == nil {
		t.Fatal("expected error issuing with empty secret")
	}
}

func TestIssueRejectsNonPositiveTTL(t *testing.T) {
	if _, err := Issue("u", "u", "viewer", []byte("s"), 0); err == nil {
		t.Fatal("expected error issuing with zero ttl")
	}
}
