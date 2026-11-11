#!/usr/bin/env node
"use strict";

const { spawnSync } = require("child_process");
const { packageFor } = require("./platform");

const packageName = packageFor(process.platform, process.arch);
if (!packageName) {
  console.error(`devops-daily does not support ${process.platform}/${process.arch}.`);
  console.error("Use the install script or a release archive instead.");
  process.exit(1);
}

const binaryName = process.platform === "win32" ? "devops-daily.exe" : "devops-daily";
let binary;
try {
  binary = require.resolve(`${packageName}/bin/${binaryName}`);
} catch (_) {
  console.error(`The optional package ${packageName} is missing.`);
  console.error("Reinstall without --no-optional or --omit=optional, or use the install script.");
  process.exit(1);
}

const result = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });
if (result.error) {
  console.error(`Could not run devops-daily: ${result.error.message}`);
  process.exit(1);
}
if (result.signal) {
  process.kill(process.pid, result.signal);
}
process.exit(result.status === null ? 1 : result.status);
