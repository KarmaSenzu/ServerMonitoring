const { exec } = require("child_process");

const runCommand = (command) => {
  return new Promise((resolve, reject) => {
    exec(command, { timeout: 15000 }, (err, stdout, stderr) => {
      if (err) return reject(err.message);
      resolve(stdout.trim());
    });
  });
};

module.exports = { runCommand };
