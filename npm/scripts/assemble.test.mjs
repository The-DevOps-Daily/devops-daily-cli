import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { chmod, mkdtemp, mkdir, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { execFileSync, spawnSync } from "node:child_process";
import test from "node:test";
import { assemble } from "./assemble.mjs";

const here = resolve(fileURLToPath(new URL(".", import.meta.url)));
const repo = resolve(here, "..", "..");
const shimPlatform = await import(new URL("../devops-daily-cli/bin/platform.js", import.meta.url));
const targets = [
  ["linux", "amd64", "linux", "x64"],
  ["linux", "arm64", "linux", "arm64"],
  ["darwin", "amd64", "darwin", "x64"],
  ["darwin", "arm64", "darwin", "arm64"],
  ["windows", "amd64", "win32", "x64"],
  ["windows", "arm64", "win32", "arm64"],
];
const canRunArchiveTools = !spawnSync("tar", ["--version"], { stdio: "ignore" }).error;

function hash(value) {
  return createHash("sha256").update(value).digest("hex");
}

async function fakeRelease(version) {
  const base = await mkdtemp(join(tmpdir(), "devops-daily-release-"));
  const dist = join(base, "dist");
  const source = join(base, "source");
  await mkdir(dist);
  await mkdir(source);
  const checksums = [];
  for (const [goos, goarch] of targets) {
    const binary = goos === "windows" ? "devops-daily.exe" : "devops-daily";
    await writeFile(join(source, binary), "#!/bin/sh\necho devops-daily test\n");
    await chmod(join(source, binary), 0o755);
    const extension = goos === "windows" ? ".zip" : ".tar.gz";
    const archive = `devops-daily_${version}_${goos}_${goarch}${extension}`;
    if (extension === ".zip") execFileSync("zip", ["-q", join(dist, archive), binary], { cwd: source });
    else execFileSync("tar", ["-czf", join(dist, archive), binary], { cwd: source });
    const contents = await readFile(join(dist, archive));
    checksums.push(`${hash(contents)}  ${archive}`);
  }
  await writeFile(join(dist, "checksums.txt"), `${checksums.join("\n")}\n`);
  return { base, dist };
}

test("assembles verified platform and main packages", { skip: !canRunArchiveTools }, async () => {
  const version = "1.2.3-test.1";
  const { base, dist } = await fakeRelease(version);
  const out = join(base, "out");
  await assemble({ version, dist, out });
  const manifest = JSON.parse(await readFile(join(out, "@devopsdaily", "devops-daily-cli", "package.json"), "utf8"));
  assert.equal(manifest.version, version);
  assert.equal(manifest.optionalDependencies["@devopsdaily/devops-daily-cli-linux-x64"], version);
  assert.deepEqual(JSON.parse(await readFile(join(out, "@devopsdaily", "devops-daily-cli-linux-x64", "package.json"), "utf8")).os, ["linux"]);
  const binary = await readFile(join(out, "@devopsdaily", "devops-daily-cli-linux-x64", "bin", "devops-daily"), "utf8");
  assert.match(binary, /devops-daily test/);
});

test("refuses an archive with a bad checksum", { skip: !canRunArchiveTools }, async () => {
  const version = "1.2.3";
  const { base, dist } = await fakeRelease(version);
  const checksums = join(dist, "checksums.txt");
  await writeFile(checksums, (await readFile(checksums, "utf8")).replace(/^[a-f0-9]/, "0"));
  await assert.rejects(assemble({ version, dist, out: join(base, "out") }), /Checksum mismatch/);
});

test("selects platform package names", () => {
  for (const [, , platform, arch] of targets) {
    assert.equal(shimPlatform.packageFor(platform, arch), `@devopsdaily/devops-daily-cli-${platform}-${arch}`);
  }
  assert.equal(shimPlatform.packageFor("freebsd", "x64"), null);
});
