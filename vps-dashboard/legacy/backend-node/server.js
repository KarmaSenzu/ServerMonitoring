const express = require("express");
const cors = require("cors");

const dockerRoutes = require("./routes/docker");
const systemRoutes = require("./routes/system");
const generatorRoutes = require("./routes/generator");

const app = express();
const PORT = process.env.PORT || 3001;

app.use(cors());
app.use(express.json());

// Routes
app.use("/docker", dockerRoutes);
app.use("/system", systemRoutes);
app.use("/generator", generatorRoutes);

// Health check
app.get("/health", (req, res) => {
  res.json({ status: "ok", timestamp: new Date().toISOString() });
});

app.listen(PORT, () => {
  console.log(`Backend running on http://localhost:${PORT}`);
});
