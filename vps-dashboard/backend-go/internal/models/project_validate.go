package models

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"vps-dashboard-api/internal/safeexec"
)

var (
	projectNameRE   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.\- ]{0,63}$`)
	projectPM2RE    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.\-]{0,63}$`)
	projectTunnelRE = regexp.MustCompile(`^cloudflared(-[a-zA-Z0-9_.\-]+)?$`)
	projectTagRE    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_\- ]{0,31}$`)
	// Liberal hostname check: dotted labels of letters/digits/hyphens.
	hostnameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?)*$`)
	// Absolute path with no spaces and no traversal.
	deployCwdRE = regexp.MustCompile(`^/[a-zA-Z0-9._/\-]+$`)
)

// deployCommandMaxLen caps the persisted deploy command length.
const deployCommandMaxLen = 4096

// Validate enforces the field-level rules described in the design doc.
// It mutates p in place to apply trivial canonicalisations
// (e.g. lowercasing the domain, deduplicating tags).
func (p *Project) Validate() error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return fmt.Errorf("name: required")
	}
	if !projectNameRE.MatchString(p.Name) {
		return fmt.Errorf("name: invalid format")
	}

	p.Domain = strings.TrimSpace(strings.ToLower(p.Domain))
	if p.Domain != "" {
		if len(p.Domain) > 253 {
			return fmt.Errorf("domain: too long")
		}
		if !hostnameRE.MatchString(p.Domain) {
			return fmt.Errorf("domain: invalid hostname")
		}
	}

	if p.Port < 0 || p.Port > 65535 {
		return fmt.Errorf("port: must be between 0 and 65535")
	}

	p.ContainerName = strings.TrimSpace(p.ContainerName)
	if p.ContainerName != "" {
		if err := safeexec.ValidateContainerName(p.ContainerName); err != nil {
			return fmt.Errorf("container_name: %s", err.Error())
		}
	}

	p.PM2Name = strings.TrimSpace(p.PM2Name)
	if p.PM2Name != "" && !projectPM2RE.MatchString(p.PM2Name) {
		return fmt.Errorf("pm2_name: invalid format")
	}

	p.TunnelService = strings.TrimSpace(p.TunnelService)
	if p.TunnelService != "" && !projectTunnelRE.MatchString(p.TunnelService) {
		return fmt.Errorf("tunnel_service: must be cloudflared or cloudflared-<name>")
	}

	p.HealthURL = strings.TrimSpace(p.HealthURL)
	if p.HealthURL != "" {
		u, err := url.Parse(p.HealthURL)
		if err != nil {
			return fmt.Errorf("health_url: %s", err.Error())
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("health_url: scheme must be http or https")
		}
		if u.Host == "" {
			return fmt.Errorf("health_url: host required")
		}
	}

	if len(p.Tags) > 20 {
		return fmt.Errorf("tags: max 20")
	}
	cleaned := make([]string, 0, len(p.Tags))
	seen := make(map[string]struct{}, len(p.Tags))
	for _, t := range p.Tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if !projectTagRE.MatchString(t) {
			return fmt.Errorf("tags: %q is invalid", t)
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		cleaned = append(cleaned, t)
	}
	p.Tags = cleaned

	// Wave 4: environment + deploy fields.
	p.Environment = strings.TrimSpace(strings.ToLower(p.Environment))
	if p.Environment == "" {
		p.Environment = ProjectEnvProduction
	}
	switch p.Environment {
	case ProjectEnvDevelopment, ProjectEnvStaging, ProjectEnvProduction:
	default:
		return fmt.Errorf("environment: must be development|staging|production")
	}

	p.DeployCommand = strings.TrimSpace(p.DeployCommand)
	if len(p.DeployCommand) > deployCommandMaxLen {
		return fmt.Errorf("deploy_command: too long (max %d)", deployCommandMaxLen)
	}
	if p.DeployCommand != "" {
		if err := validateDeployCommand(p.DeployCommand); err != nil {
			return err
		}
	}
	if p.DeployEnabled && p.DeployCommand == "" {
		return fmt.Errorf("deploy_command: required when deploy_enabled=true")
	}

	if p.DeployTimeoutSeconds == 0 {
		p.DeployTimeoutSeconds = 300
	}
	if p.DeployTimeoutSeconds < 30 || p.DeployTimeoutSeconds > 3600 {
		return fmt.Errorf("deploy_timeout_seconds: must be between 30 and 3600")
	}

	p.DeployWorkingDir = strings.TrimSpace(p.DeployWorkingDir)
	if p.DeployWorkingDir != "" {
		if !deployCwdRE.MatchString(p.DeployWorkingDir) {
			return fmt.Errorf("deploy_working_dir: must be an absolute path with no spaces or traversal")
		}
		if strings.Contains(p.DeployWorkingDir, "..") {
			return fmt.Errorf("deploy_working_dir: must not contain ..")
		}
	}

	// WebhookSecret is intentionally not range-checked here; the
	// regenerate handler is the only writer and it always produces a
	// 64-character hex string. An empty value is allowed because deploy
	// admins may toggle deploy_enabled before generating one — the
	// webhook handler refuses calls until the secret is present.

	return nil
}

// validateDeployCommand bans common shell-injection patterns. The
// command will be exec'd via `bash -c <cmd>` because real-world deploy
// flows chain steps (git pull && pm2 restart). To keep that path safe:
//   - configuration is admin-only (enforced by the HTTP layer);
//   - commands run as the dashboard process user (not root);
//   - validation here removes the most dangerous metacharacters before
//     persistence;
//   - execution is bounded by a per-project timeout and project-scoped
//     cwd; and
//   - every invocation lands in the deployments audit table.
//
// The ban list rejects: ";", "&&", "||", "|", backticks and "$(".
// A trailing single "&" is permitted (some teams use it deliberately).
func validateDeployCommand(cmd string) error {
	if strings.Contains(cmd, ";") {
		return fmt.Errorf("deploy_command: ';' is not allowed")
	}
	if strings.Contains(cmd, "&&") {
		return fmt.Errorf("deploy_command: '&&' is not allowed")
	}
	if strings.Contains(cmd, "||") {
		return fmt.Errorf("deploy_command: '||' is not allowed")
	}
	if strings.Contains(cmd, "|") {
		return fmt.Errorf("deploy_command: '|' is not allowed")
	}
	if strings.Contains(cmd, "`") {
		return fmt.Errorf("deploy_command: backtick is not allowed")
	}
	if strings.Contains(cmd, "$(") {
		return fmt.Errorf("deploy_command: '$(' is not allowed")
	}
	// Stand-alone "&" is acceptable only when it's the last non-space
	// character of the command. Trim trailing spaces, then disallow any
	// inner '&'.
	trimmed := strings.TrimRight(cmd, " \t")
	body := trimmed
	if strings.HasSuffix(trimmed, "&") {
		body = strings.TrimRight(trimmed[:len(trimmed)-1], " \t")
	}
	if strings.Contains(body, "&") {
		return fmt.Errorf("deploy_command: '&' is only allowed as a trailing character")
	}
	return nil
}
