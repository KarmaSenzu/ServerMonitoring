package safeexec

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"
)

func TestRunEcho(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("echo semantics differ on windows")
	}
	stdout, _, err := Run(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatalf("Run echo: %v", err)
	}
	if stdout != "hello" {
		t.Errorf("stdout: got %q want %q", stdout, "hello")
	}
}

func TestRunFalseNonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("false not available on windows")
	}
	_, _, err := Run(context.Background(), "false")
	if err == nil {
		t.Fatal("expected non-nil error for `false`")
	}
}

func TestRunUnknownBinary(t *testing.T) {
	_, _, err := Run(context.Background(), "definitely-not-a-real-binary-xyz")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestRunRespectsContextTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep not available on windows")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, _, err := Run(ctx, "sleep", "5")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("expected ErrTimeout, got %v", err)
	}
}

func TestRunWithInput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cat semantics differ on windows")
	}
	stdout, _, err := RunWithInput(context.Background(), "hello stdin\n", "cat")
	if err != nil {
		t.Fatalf("RunWithInput: %v", err)
	}
	if stdout != "hello stdin" {
		t.Errorf("stdout: got %q want %q", stdout, "hello stdin")
	}
}

func TestValidateContainerName(t *testing.T) {
	good := []string{
		"app",
		"my_container",
		"my-container.1",
		"a",
		"A1B2",
	}
	for _, s := range good {
		if err := ValidateContainerName(s); err != nil {
			t.Errorf("ValidateContainerName(%q) unexpected err: %v", s, err)
		}
	}

	bad := []string{
		"",
		"-leading-dash",
		"_leading-underscore",
		".leading-dot",
		"; rm -rf /",
		"with space",
		"with$dollar",
		"with/slash",
		"with`backtick",
		"with\"quote",
		"with'quote",
		// 129 chars
		"a" + makeString(128, 'b'),
	}
	for _, s := range bad {
		if err := ValidateContainerName(s); err == nil {
			t.Errorf("ValidateContainerName(%q) expected error", s)
		}
	}
}

func TestValidateDuration(t *testing.T) {
	good := []string{"0", "10", "30s", "5m", "2h"}
	for _, s := range good {
		if err := ValidateDuration(s); err != nil {
			t.Errorf("ValidateDuration(%q) unexpected err: %v", s, err)
		}
	}
	bad := []string{"", "-1", "1d", "abc", "1 m", "1.5s", ";rm"}
	for _, s := range bad {
		if err := ValidateDuration(s); err == nil {
			t.Errorf("ValidateDuration(%q) expected error", s)
		}
	}
}

func TestValidatePosInt(t *testing.T) {
	good := []string{"1", "9", "100", "999999"}
	for _, s := range good {
		if err := ValidatePosInt(s); err != nil {
			t.Errorf("ValidatePosInt(%q) unexpected err: %v", s, err)
		}
	}
	bad := []string{"", "0", "01", "-1", "1000000", "abc", "1a"}
	for _, s := range bad {
		if err := ValidatePosInt(s); err == nil {
			t.Errorf("ValidatePosInt(%q) expected error", s)
		}
	}
}

func makeString(n int, ch byte) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = ch
	}
	return string(b)
}
