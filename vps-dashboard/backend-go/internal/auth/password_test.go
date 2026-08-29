package auth

import "testing"

func TestHashAndVerify(t *testing.T) {
	hash, err := Hash("Sup3rSecret!")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if hash == "" {
		t.Fatal("Hash returned empty string")
	}
	if !Verify(hash, "Sup3rSecret!") {
		t.Fatal("Verify returned false for matching password")
	}
}

func TestVerifyWrongPassword(t *testing.T) {
	hash, err := Hash("rightpassword")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if Verify(hash, "wrongpassword") {
		t.Fatal("Verify returned true for non-matching password")
	}
}

func TestHashRejectsEmpty(t *testing.T) {
	if _, err := Hash(""); err == nil {
		t.Fatal("Hash accepted empty password")
	}
}

func TestVerifyRejectsEmpty(t *testing.T) {
	if Verify("", "x") {
		t.Fatal("Verify accepted empty hash")
	}
	if Verify("$2a$12$invalidhash", "") {
		t.Fatal("Verify accepted empty plaintext")
	}
}
