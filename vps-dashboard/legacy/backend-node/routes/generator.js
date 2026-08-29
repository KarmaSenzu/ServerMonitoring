const express = require("express");
const router = express.Router();

// Generate docker run command
router.post("/docker", (req, res) => {
  const { name, port, image, hostPort, env, volumes, restart, network } = req.body;

  if (!name || !image) {
    return res.status(400).json({ error: "name and image are required" });
  }

  let command = "docker run -d";
  command += ` --name ${name}`;

  if (restart) command += ` --restart ${restart}`;
  if (network) command += ` --network ${network}`;
  if (port) command += ` -p ${hostPort || port}:${port}`;

  if (env && Array.isArray(env)) {
    env.forEach((e) => {
      command += ` -e ${e}`;
    });
  }

  if (volumes && Array.isArray(volumes)) {
    volumes.forEach((v) => {
      command += ` -v ${v}`;
    });
  }

  command += ` ${image}`;

  res.json({ command });
});

// Generate pm2 command
router.post("/pm2", (req, res) => {
  const { name, file, interpreter, instances, watch } = req.body;

  if (!name || !file) {
    return res.status(400).json({ error: "name and file are required" });
  }

  let command = `pm2 start ${file} --name ${name}`;

  if (interpreter) command += ` --interpreter ${interpreter}`;
  if (instances) command += ` -i ${instances}`;
  if (watch) command += " --watch";

  res.json({ command });
});

// Generate docker-compose template
router.post("/compose", (req, res) => {
  const { services } = req.body;

  if (!services || !Array.isArray(services)) {
    return res.status(400).json({ error: "services array is required" });
  }

  let yaml = "version: '3.8'\nservices:\n";

  services.forEach((svc) => {
    yaml += `  ${svc.name}:\n`;
    yaml += `    image: ${svc.image}\n`;
    if (svc.ports) {
      yaml += "    ports:\n";
      svc.ports.forEach((p) => {
        yaml += `      - "${p}"\n`;
      });
    }
    if (svc.restart) {
      yaml += `    restart: ${svc.restart}\n`;
    }
    yaml += "\n";
  });

  res.json({ command: yaml });
});

// Generate nginx reverse proxy config
router.post("/nginx", (req, res) => {
  const { domain, proxyPort, ssl } = req.body;

  if (!domain || !proxyPort) {
    return res.status(400).json({ error: "domain and proxyPort are required" });
  }

  let config = `server {\n`;
  if (ssl) {
    config += `    listen 443 ssl;\n`;
    config += `    ssl_certificate /etc/letsencrypt/live/${domain}/fullchain.pem;\n`;
    config += `    ssl_certificate_key /etc/letsencrypt/live/${domain}/privkey.pem;\n`;
  } else {
    config += `    listen 80;\n`;
  }
  config += `    server_name ${domain};\n\n`;
  config += `    location / {\n`;
  config += `        proxy_pass http://127.0.0.1:${proxyPort};\n`;
  config += `        proxy_set_header Host $host;\n`;
  config += `        proxy_set_header X-Real-IP $remote_addr;\n`;
  config += `        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n`;
  config += `        proxy_set_header X-Forwarded-Proto $scheme;\n`;
  config += `    }\n`;
  config += `}\n`;

  res.json({ command: config });
});

module.exports = router;
