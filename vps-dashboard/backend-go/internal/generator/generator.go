// Package generator turns small struct payloads into ready-to-paste
// shell/YAML snippets for Docker, PM2, docker-compose, and Nginx. None
// of these functions execute anything; the resulting strings are sent
// back to the operator as text only.
package generator

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// EnvVar represents a single name=value pair.
type EnvVar struct {
	Key   string `json:"key" binding:"required"`
	Value string `json:"value"`
}

// PortMapping is a host->container port pair.
type PortMapping struct {
	Host      int    `json:"host" binding:"required,min=1,max=65535"`
	Container int    `json:"container" binding:"required,min=1,max=65535"`
	Protocol  string `json:"protocol"`
}

// VolumeMapping is a host->container volume bind.
type VolumeMapping struct {
	Host      string `json:"host" binding:"required"`
	Container string `json:"container" binding:"required"`
	Mode      string `json:"mode"`
}

// DockerRunOpts is the input shape for DockerRun.
type DockerRunOpts struct {
	Name      string          `json:"name" binding:"required"`
	Image     string          `json:"image" binding:"required"`
	Detached  bool            `json:"detached"`
	Restart   string          `json:"restart"`
	Network   string          `json:"network"`
	Ports     []PortMapping   `json:"ports"`
	Volumes   []VolumeMapping `json:"volumes"`
	Env       []EnvVar        `json:"env"`
	Command   string          `json:"command"`
	ExtraArgs []string        `json:"extra_args"`
}

// allowed values for `docker run --restart`.
var allowedRestart = map[string]struct{}{
	"":               {},
	"no":             {},
	"always":         {},
	"on-failure":     {},
	"unless-stopped": {},
}

// DockerRun renders a `docker run` command. The output is multi-line
// for readability and uses single-quoted shell-safe args.
func DockerRun(opts DockerRunOpts) (string, error) {
	if strings.TrimSpace(opts.Name) == "" {
		return "", fmt.Errorf("name: required")
	}
	if strings.TrimSpace(opts.Image) == "" {
		return "", fmt.Errorf("image: required")
	}
	if _, ok := allowedRestart[opts.Restart]; !ok {
		return "", fmt.Errorf("restart: must be one of no|always|on-failure|unless-stopped")
	}

	for i, p := range opts.Ports {
		if p.Host <= 0 || p.Host > 65535 {
			return "", fmt.Errorf("ports[%d].host: out of range", i)
		}
		if p.Container <= 0 || p.Container > 65535 {
			return "", fmt.Errorf("ports[%d].container: out of range", i)
		}
		if p.Protocol != "" && p.Protocol != "tcp" && p.Protocol != "udp" {
			return "", fmt.Errorf("ports[%d].protocol: must be tcp or udp", i)
		}
	}
	for i, v := range opts.Volumes {
		if strings.TrimSpace(v.Host) == "" {
			return "", fmt.Errorf("volumes[%d].host: required", i)
		}
		if strings.TrimSpace(v.Container) == "" {
			return "", fmt.Errorf("volumes[%d].container: required", i)
		}
	}
	for i, e := range opts.Env {
		if strings.TrimSpace(e.Key) == "" {
			return "", fmt.Errorf("env[%d].key: required", i)
		}
	}

	parts := []string{"docker run"}
	if opts.Detached {
		parts[0] = "docker run -d"
	}

	parts = append(parts, "--name "+shellQuote(opts.Name))
	if opts.Restart != "" {
		parts = append(parts, "--restart "+opts.Restart)
	}
	if opts.Network != "" {
		parts = append(parts, "--network "+shellQuote(opts.Network))
	}
	for _, p := range opts.Ports {
		spec := fmt.Sprintf("%d:%d", p.Host, p.Container)
		if p.Protocol != "" {
			spec += "/" + p.Protocol
		}
		parts = append(parts, "-p "+shellQuote(spec))
	}
	for _, v := range opts.Volumes {
		spec := v.Host + ":" + v.Container
		if v.Mode != "" {
			spec += ":" + v.Mode
		}
		parts = append(parts, "-v "+shellQuote(spec))
	}
	for _, e := range opts.Env {
		parts = append(parts, "-e "+shellQuote(e.Key+"="+e.Value))
	}
	for _, a := range opts.ExtraArgs {
		parts = append(parts, shellQuote(a))
	}
	parts = append(parts, shellQuote(opts.Image))
	if strings.TrimSpace(opts.Command) != "" {
		parts = append(parts, shellQuote(opts.Command))
	}

	return strings.Join(parts, " \\\n  "), nil
}

// PM2Opts is the input shape for PM2Start.
type PM2Opts struct {
	Name             string   `json:"name" binding:"required"`
	Script           string   `json:"script" binding:"required"`
	Interpreter      string   `json:"interpreter"`
	Instances        int      `json:"instances"`
	Watch            bool     `json:"watch"`
	MaxMemoryRestart string   `json:"max_memory_restart"`
	Cwd              string   `json:"cwd"`
	Env              []EnvVar `json:"env"`
}

// PM2Start renders a `pm2 start ...` command.
func PM2Start(opts PM2Opts) (string, error) {
	if strings.TrimSpace(opts.Name) == "" {
		return "", fmt.Errorf("name: required")
	}
	if strings.TrimSpace(opts.Script) == "" {
		return "", fmt.Errorf("script: required")
	}
	if opts.Instances < 0 {
		return "", fmt.Errorf("instances: must be >= 0")
	}
	for i, e := range opts.Env {
		if strings.TrimSpace(e.Key) == "" {
			return "", fmt.Errorf("env[%d].key: required", i)
		}
	}

	parts := []string{
		"pm2 start " + shellQuote(opts.Script),
		"--name " + shellQuote(opts.Name),
	}
	if opts.Interpreter != "" {
		parts = append(parts, "--interpreter "+shellQuote(opts.Interpreter))
	}
	if opts.Instances > 0 {
		parts = append(parts, fmt.Sprintf("-i %d", opts.Instances))
	}
	if opts.Watch {
		parts = append(parts, "--watch")
	}
	if opts.MaxMemoryRestart != "" {
		parts = append(parts, "--max-memory-restart "+shellQuote(opts.MaxMemoryRestart))
	}
	if opts.Cwd != "" {
		parts = append(parts, "--cwd "+shellQuote(opts.Cwd))
	}
	for _, e := range opts.Env {
		parts = append(parts, "--env "+shellQuote(e.Key+"="+e.Value))
	}

	return strings.Join(parts, " \\\n  "), nil
}

// ComposeService is one entry under `services:` in docker-compose.
type ComposeService struct {
	Name          string          `json:"name" binding:"required"`
	Image         string          `json:"image" binding:"required"`
	ContainerName string          `json:"container_name"`
	Restart       string          `json:"restart"`
	Network       string          `json:"network"`
	Networks      []string        `json:"networks"`
	Ports         []PortMapping   `json:"ports"`
	Volumes       []VolumeMapping `json:"volumes"`
	Env           []EnvVar        `json:"env"`
	Command       string          `json:"command"`
	DependsOn     []string        `json:"depends_on"`
}

// ComposeOpts is the input shape for DockerCompose.
type ComposeOpts struct {
	Services []ComposeService `json:"services" binding:"required,min=1"`
}

// DockerCompose renders a docker-compose.yml v3.8 document. We build a
// map[string]any then encode with yaml.v3 to avoid hand-rolled escaping.
func DockerCompose(opts ComposeOpts) (string, error) {
	if len(opts.Services) == 0 {
		return "", fmt.Errorf("services: required")
	}

	doc := map[string]any{
		"version":  "3.8",
		"services": map[string]any{},
	}
	services := doc["services"].(map[string]any)

	for i, svc := range opts.Services {
		if strings.TrimSpace(svc.Name) == "" {
			return "", fmt.Errorf("services[%d].name: required", i)
		}
		if strings.TrimSpace(svc.Image) == "" {
			return "", fmt.Errorf("services[%d].image: required", i)
		}
		if _, ok := allowedRestart[svc.Restart]; !ok {
			return "", fmt.Errorf("services[%d].restart: invalid", i)
		}
		for j, p := range svc.Ports {
			if p.Host <= 0 || p.Host > 65535 {
				return "", fmt.Errorf("services[%d].ports[%d].host: out of range", i, j)
			}
			if p.Container <= 0 || p.Container > 65535 {
				return "", fmt.Errorf("services[%d].ports[%d].container: out of range", i, j)
			}
			if p.Protocol != "" && p.Protocol != "tcp" && p.Protocol != "udp" {
				return "", fmt.Errorf("services[%d].ports[%d].protocol: invalid", i, j)
			}
		}
		for j, e := range svc.Env {
			if strings.TrimSpace(e.Key) == "" {
				return "", fmt.Errorf("services[%d].env[%d].key: required", i, j)
			}
		}

		entry := map[string]any{
			"image": svc.Image,
		}
		if svc.ContainerName != "" {
			entry["container_name"] = svc.ContainerName
		}
		if svc.Restart != "" {
			entry["restart"] = svc.Restart
		}
		if len(svc.Ports) > 0 {
			ports := make([]string, 0, len(svc.Ports))
			for _, p := range svc.Ports {
				spec := fmt.Sprintf("%d:%d", p.Host, p.Container)
				if p.Protocol != "" {
					spec += "/" + p.Protocol
				}
				ports = append(ports, spec)
			}
			entry["ports"] = ports
		}
		if len(svc.Volumes) > 0 {
			vols := make([]string, 0, len(svc.Volumes))
			for _, v := range svc.Volumes {
				spec := v.Host + ":" + v.Container
				if v.Mode != "" {
					spec += ":" + v.Mode
				}
				vols = append(vols, spec)
			}
			entry["volumes"] = vols
		}
		if len(svc.Env) > 0 {
			env := make(map[string]string, len(svc.Env))
			keys := make([]string, 0, len(svc.Env))
			for _, e := range svc.Env {
				env[e.Key] = e.Value
				keys = append(keys, e.Key)
			}
			sort.Strings(keys)
			ordered := make([]string, 0, len(keys))
			for _, k := range keys {
				ordered = append(ordered, k+"="+env[k])
			}
			entry["environment"] = ordered
		}
		if svc.Command != "" {
			entry["command"] = svc.Command
		}
		if len(svc.DependsOn) > 0 {
			entry["depends_on"] = svc.DependsOn
		}
		if len(svc.Networks) > 0 {
			entry["networks"] = svc.Networks
		} else if svc.Network != "" {
			entry["networks"] = []string{svc.Network}
		}

		services[svc.Name] = entry
	}

	var buf strings.Builder
	buf.WriteString("# generated by vps-dashboard\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		_ = enc.Close()
		return "", fmt.Errorf("compose: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		return "", fmt.Errorf("compose: close: %w", err)
	}
	return buf.String(), nil
}

// NginxOpts is the input shape for NginxReverseProxy.
type NginxOpts struct {
	Domain            string `json:"domain" binding:"required"`
	UpstreamHost      string `json:"upstream_host" binding:"required"`
	UpstreamPort      int    `json:"upstream_port" binding:"required,min=1,max=65535"`
	EnableSSL         bool   `json:"enable_ssl"`
	SSLCertPath       string `json:"ssl_cert_path"`
	SSLKeyPath        string `json:"ssl_key_path"`
	EnableWebsocket   bool   `json:"enable_websocket"`
	ClientMaxBodySize string `json:"client_max_body_size"`
}

// NginxReverseProxy renders an nginx server block (or a pair of blocks
// when SSL is enabled, with an http->https redirect on :80).
func NginxReverseProxy(opts NginxOpts) (string, error) {
	if strings.TrimSpace(opts.Domain) == "" {
		return "", fmt.Errorf("domain: required")
	}
	if strings.TrimSpace(opts.UpstreamHost) == "" {
		return "", fmt.Errorf("upstream_host: required")
	}
	if opts.UpstreamPort < 1 || opts.UpstreamPort > 65535 {
		return "", fmt.Errorf("upstream_port: out of range")
	}
	if opts.EnableSSL {
		if strings.TrimSpace(opts.SSLCertPath) == "" {
			return "", fmt.Errorf("ssl_cert_path: required when enable_ssl is true")
		}
		if strings.TrimSpace(opts.SSLKeyPath) == "" {
			return "", fmt.Errorf("ssl_key_path: required when enable_ssl is true")
		}
	}

	upstream := fmt.Sprintf("http://%s:%d", opts.UpstreamHost, opts.UpstreamPort)

	var b strings.Builder
	if opts.EnableSSL {
		// HTTP -> HTTPS redirect.
		b.WriteString("server {\n")
		b.WriteString("    listen 80;\n")
		b.WriteString(fmt.Sprintf("    server_name %s;\n", opts.Domain))
		b.WriteString("    return 301 https://$host$request_uri;\n")
		b.WriteString("}\n\n")

		// HTTPS server.
		b.WriteString("server {\n")
		b.WriteString("    listen 443 ssl;\n")
		b.WriteString(fmt.Sprintf("    server_name %s;\n\n", opts.Domain))
		b.WriteString(fmt.Sprintf("    ssl_certificate %s;\n", opts.SSLCertPath))
		b.WriteString(fmt.Sprintf("    ssl_certificate_key %s;\n\n", opts.SSLKeyPath))
		writeProxyBlock(&b, upstream, opts)
		b.WriteString("}\n")
	} else {
		b.WriteString("server {\n")
		b.WriteString("    listen 80;\n")
		b.WriteString(fmt.Sprintf("    server_name %s;\n\n", opts.Domain))
		writeProxyBlock(&b, upstream, opts)
		b.WriteString("}\n")
	}
	return b.String(), nil
}

func writeProxyBlock(b *strings.Builder, upstream string, opts NginxOpts) {
	if strings.TrimSpace(opts.ClientMaxBodySize) != "" {
		b.WriteString(fmt.Sprintf("    client_max_body_size %s;\n\n", opts.ClientMaxBodySize))
	}
	b.WriteString("    location / {\n")
	b.WriteString(fmt.Sprintf("        proxy_pass %s;\n", upstream))
	b.WriteString("        proxy_set_header Host $host;\n")
	b.WriteString("        proxy_set_header X-Real-IP $remote_addr;\n")
	b.WriteString("        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
	b.WriteString("        proxy_set_header X-Forwarded-Proto $scheme;\n")
	if opts.EnableWebsocket {
		b.WriteString("        proxy_http_version 1.1;\n")
		b.WriteString("        proxy_set_header Upgrade $http_upgrade;\n")
		b.WriteString("        proxy_set_header Connection \"upgrade\";\n")
	}
	b.WriteString("    }\n")
}

// shellQuote wraps s in single quotes, escaping any embedded single
// quotes via the standard '\'' trick. Always safe to drop into a POSIX
// shell as a single argument.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
