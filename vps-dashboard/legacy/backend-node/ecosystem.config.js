module.exports = {
  apps: [
    {
      name: "vps-dashboard-api",
      script: "server.js",
      cwd: "/home/ubuntu/vps-dashboard/backend",
      instances: 1,
      autorestart: true,
      watch: false,
      max_memory_restart: "200M",
      env: {
        NODE_ENV: "production",
        PORT: 3001,
      },
    },
  ],
};
