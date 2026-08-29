const express = require("express");
const os = require("os");
const router = express.Router();
const { runCommand } = require("../services/exec");

// GET system stats
router.get("/stats", async (req, res) => {
  const totalMem = os.totalmem();
  const freeMem = os.freemem();
  const usedMem = totalMem - freeMem;

  // Get disk usage
  let disk = { total: 0, used: 0, free: 0, usagePercent: "0" };
  try {
    const dfOutput = await runCommand("df -B1 / | tail -1");
    const parts = dfOutput.trim().split(/\s+/);
    if (parts.length >= 4) {
      const total = parseInt(parts[1]);
      const used = parseInt(parts[2]);
      const free = parseInt(parts[3]);
      disk = {
        total,
        used,
        free,
        usagePercent: ((used / total) * 100).toFixed(1),
      };
    }
  } catch (e) {
    // disk info unavailable
  }

  res.json({
    cpu: {
      loadAvg: os.loadavg(),
      cores: os.cpus().length,
      model: os.cpus()[0]?.model || "Unknown",
    },
    ram: {
      total: totalMem,
      free: freeMem,
      used: usedMem,
      usagePercent: ((usedMem / totalMem) * 100).toFixed(1),
    },
    disk,
    uptime: os.uptime(),
    hostname: os.hostname(),
    platform: os.platform(),
    arch: os.arch(),
  });
});

// GET cloudflare tunnel status (simple)
router.get("/tunnel", async (req, res) => {
  try {
    const result = await runCommand(
      "systemctl status cloudflared --no-pager 2>&1 || echo 'cloudflared not found'"
    );
    const isActive = result.includes("active (running)");
    res.json({
      status: isActive ? "running" : "stopped",
      raw: result,
    });
  } catch (err) {
    res.json({ status: "unknown", error: err });
  }
});

// GET all cloudflare tunnels with details
router.get("/tunnels", async (req, res) => {
  try {
    const tunnels = [];

    // Get all cloudflared services
    const services = await runCommand(
      "systemctl list-units --type=service --all --no-pager 2>/dev/null | grep cloudflared || echo ''"
    );

    // Find all cloudflared config files
    const configFiles = await runCommand(
      "find /etc/cloudflared -name '*.yml' -not -name '*.bak*' 2>/dev/null || echo ''"
    );

    const configs = configFiles.trim().split("\n").filter(Boolean);

    for (const configPath of configs) {
      try {
        // Read config file
        const configContent = await runCommand(`cat ${configPath} 2>/dev/null`);

        // Parse tunnel ID
        const tunnelIdMatch = configContent.match(/tunnel:\s*(.+)/);
        const tunnelId = tunnelIdMatch ? tunnelIdMatch[1].trim() : "unknown";

        // Parse ingress rules
        const ingressRules = [];
        const lines = configContent.split("\n");
        let inIngress = false;

        for (let i = 0; i < lines.length; i++) {
          const line = lines[i];
          if (line.match(/^ingress:/)) {
            inIngress = true;
            continue;
          }
          if (inIngress) {
            const hostnameMatch = line.match(/hostname:\s*(.+)/);
            const serviceMatch = lines[i + 1]
              ? lines[i + 1].match(/service:\s*(.+)/)
              : null;

            if (hostnameMatch) {
              ingressRules.push({
                hostname: hostnameMatch[1].trim(),
                service: serviceMatch ? serviceMatch[1].trim() : "unknown",
              });
            }

            // Catch-all rule (no hostname)
            if (line.match(/^\s*-\s*service:/) && !line.includes("hostname")) {
              const catchService = line.match(/service:\s*(.+)/);
              if (catchService) {
                ingressRules.push({
                  hostname: "*",
                  service: catchService[1].trim(),
                });
              }
            }
          }
        }

        // Determine which systemd service runs this config
        let serviceName = "cloudflared";
        if (configPath.includes("dashboard")) {
          serviceName = "cloudflared-dashboard";
        }

        // Get service status
        let status = "unknown";
        let pid = null;
        let uptime = null;
        try {
          const statusOutput = await runCommand(
            `systemctl show ${serviceName} --property=ActiveState,MainPID,ActiveEnterTimestamp --no-pager 2>/dev/null`
          );
          const activeMatch = statusOutput.match(/ActiveState=(\w+)/);
          const pidMatch = statusOutput.match(/MainPID=(\d+)/);
          const timestampMatch = statusOutput.match(
            /ActiveEnterTimestamp=(.+)/
          );

          if (activeMatch) status = activeMatch[1];
          if (pidMatch && pidMatch[1] !== "0") pid = pidMatch[1];
          if (timestampMatch && timestampMatch[1].trim())
            uptime = timestampMatch[1].trim();
        } catch (e) {
          // ignore
        }

        // Get connections info from metrics if available
        let connections = 0;
        try {
          const metricsPort = serviceName === "cloudflared" ? "20241" : "20242";
          const metrics = await runCommand(
            `curl -s --max-time 2 http://127.0.0.1:${metricsPort}/metrics 2>/dev/null | grep "cloudflared_tunnel_active_streams" | head -1 || echo ""`
          );
          const connMatch = metrics.match(/(\d+)$/);
          if (connMatch) connections = parseInt(connMatch[1]);
        } catch (e) {
          // ignore
        }

        tunnels.push({
          id: tunnelId,
          name: serviceName.replace("cloudflared-", "").replace("cloudflared", "main"),
          configPath,
          serviceName,
          status: status === "active" ? "running" : status === "inactive" ? "stopped" : status,
          pid,
          uptime,
          connections,
          ingress: ingressRules,
        });
      } catch (e) {
        // Skip broken config
      }
    }

    res.json({ tunnels });
  } catch (err) {
    res.status(500).json({ error: err.toString(), tunnels: [] });
  }
});

module.exports = router;
