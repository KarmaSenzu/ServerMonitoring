package commands_test

import (
	"testing"

	"vps-dashboard-api/internal/commands"
)

func TestClassifyDanger(t *testing.T) {
	cases := []struct {
		command string
		want    string
	}{
		{"uptime", "safe"},
		{"docker ps", "safe"},
		{"free -m", "safe"},
		{"df -h", "safe"},
		// Caution
		{"docker restart api", "caution"},
		{"docker stop nginx", "caution"},
		{"kill -9 1234", "caution"},
		{"pkill node", "caution"},
		// Dangerous
		{"rm -rf /", "dangerous"},
		{"rm -rf /var/lib", "dangerous"},
		{"shutdown -h now", "dangerous"},
		{"reboot", "dangerous"},
		{"systemctl stop nginx", "dangerous"},
		{"docker rm -f api", "dangerous"},
		{"docker system prune -af", "dangerous"},
		{"mkfs.ext4 /dev/sda1", "dangerous"},
		{"dd if=/dev/zero of=/dev/sda", "dangerous"},
	}

	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			got := commands.ClassifyDanger(tc.command)
			if got != tc.want {
				t.Errorf("ClassifyDanger(%q) = %q, want %q", tc.command, got, tc.want)
			}
		})
	}
}
