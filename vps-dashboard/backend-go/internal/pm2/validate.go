package pm2

import (
	"fmt"
	"regexp"
)

// processNameRE constrains pm2 process names accepted from the API.
// First character must be alphanumeric; remaining characters may include
// underscore, dot, or hyphen. Total length 1..64.
var processNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.\-]{0,63}$`)

// ValidateProcessName enforces the PM2 process name pattern. It is the
// only sanitizer applied before the name is passed to pm2 over exec.
func ValidateProcessName(s string) error {
	if !processNameRE.MatchString(s) {
		return fmt.Errorf("invalid pm2 process name")
	}
	return nil
}
