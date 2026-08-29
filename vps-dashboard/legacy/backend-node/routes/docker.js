const express = require("express");
const router = express.Router();
const { runCommand } = require("../services/exec");

// GET docker containers status (parsed)
router.get("/status", async (req, res) => {
  try {
    const raw = await runCommand(
      'docker ps -a --format "{{.ID}}|{{.Names}}|{{.Image}}|{{.Status}}|{{.Ports}}|{{.State}}"'
    );

    if (!raw) {
      return res.json({ containers: [], raw: "No containers found" });
    }

    const containers = raw.split("\n").map((line) => {
      const [id, name, image, status, ports, state] = line.split("|");
      return { id, name, image, status, ports, state };
    });

    res.json({ containers, raw });
  } catch (err) {
    res.status(500).json({ error: err });
  }
});

// POST start container
router.post("/start", async (req, res) => {
  const { name } = req.body;
  if (!name) return res.status(400).json({ error: "Container name required" });

  try {
    const result = await runCommand(`docker start ${name}`);
    res.json({ result, message: `Container ${name} started` });
  } catch (err) {
    res.status(500).json({ error: err });
  }
});

// POST stop container
router.post("/stop", async (req, res) => {
  const { name } = req.body;
  if (!name) return res.status(400).json({ error: "Container name required" });

  try {
    const result = await runCommand(`docker stop ${name}`);
    res.json({ result, message: `Container ${name} stopped` });
  } catch (err) {
    res.status(500).json({ error: err });
  }
});

// POST restart container
router.post("/restart", async (req, res) => {
  const { name } = req.body;
  if (!name) return res.status(400).json({ error: "Container name required" });

  try {
    const result = await runCommand(`docker restart ${name}`);
    res.json({ result, message: `Container ${name} restarted` });
  } catch (err) {
    res.status(500).json({ error: err });
  }
});

module.exports = router;
