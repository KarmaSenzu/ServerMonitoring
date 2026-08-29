// Package safeexec provides a thin wrapper around os/exec that enforces
// safe execution patterns: no shell interpolation, mandatory timeouts,
// captured stdout/stderr, and validation helpers for arguments that come
// from HTTP request payloads.
package safeexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// DefaultTimeout is applied via context.WithTimeout when the incoming
// context has no deadline of its own.
const DefaultTimeout = 15 * time.Second

// ErrTimeout is returned (wrapped) when a command is killed because its
// context deadline was exceeded.
var ErrTimeout = errors.New("safeexec: timeout")

var (
	containerNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)
	durationRE      = regexp.MustCompile(`^[0-9]+(s|m|h)?$`)
	posIntRE        = regexp.MustCompile(`^[1-9][0-9]{0,5}$`)
)

// Run executes name with args using exec.CommandContext. It NEVER routes
// through a shell interpreter. If the supplied context has no deadline a
// DefaultTimeout is applied. Stdout and stderr are captured separately
// and returned as trimmed strings even when the command exits non-zero.
//
// On non-zero exit the returned error wraps the underlying *exec.ExitError
// and includes the exit code. On context-deadline expiry the error is
// wrapped with ErrTimeout.
func Run(ctx context.Context, name string, args ...string) (string, string, error) {
	return RunWithInput(ctx, "", name, args...)
}

// RunWithInput is identical to Run except it pipes stdin to the child
// process before waiting for it to exit.
func RunWithInput(ctx context.Context, stdin string, name string, args ...string) (string, string, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, name, args...)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	runErr := cmd.Run()

	stdout := strings.TrimRight(outBuf.String(), "\r\n")
	stderr := strings.TrimRight(errBuf.String(), "\r\n")

	if runErr == nil {
		return stdout, stderr, nil
	}

	// Surface a context-deadline kill as ErrTimeout regardless of exit code.
	if ctx.Err() == context.DeadlineExceeded {
		return stdout, stderr, fmt.Errorf("%s %v: %w", name, args, ErrTimeout)
	}

	exitCode := -1
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		exitCode = exitErr.ExitCode()
	}

	return stdout, stderr, fmt.Errorf("%s %v: exit %d: %w", name, args, exitCode, runErr)
}

// ValidateContainerName enforces Docker's accepted name pattern:
// `[a-zA-Z0-9][a-zA-Z0-9_.-]*`, total length <= 128.
func ValidateContainerName(s string) error {
	if !containerNameRE.MatchString(s) {
		return fmt.Errorf("invalid container name")
	}
	return nil
}

// ValidateDuration accepts non-negative integers optionally followed by
// a single unit suffix (s, m, h). Used for log-tail durations.
func ValidateDuration(s string) error {
	if !durationRE.MatchString(s) {
		return fmt.Errorf("invalid duration")
	}
	return nil
}

// ValidatePosInt accepts a positive decimal integer with at most 6 digits
// (no leading zero). Used for line counts (e.g. tail -n).
func ValidatePosInt(s string) error {
	if !posIntRE.MatchString(s) {
		return fmt.Errorf("invalid positive integer")
	}
	return nil
}
