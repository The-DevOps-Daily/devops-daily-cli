"use strict";

const packageFor = (platform, arch) => {
  const supported = {
    darwin: { x64: "darwin-x64", arm64: "darwin-arm64" },
    linux: { x64: "linux-x64", arm64: "linux-arm64" },
    win32: { x64: "win32-x64", arm64: "win32-arm64" },
  };
  const target = supported[platform] && supported[platform][arch];
  return target ? `@devopsdaily/devops-daily-cli-${target}` : null;
};

module.exports = { packageFor };
