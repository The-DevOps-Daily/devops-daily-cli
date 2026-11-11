#!/usr/bin/env node
import { createHash } from "node:crypto";
import { cp, chmod, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { basename, dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const templateDir = join(root, "devops-daily-cli");
const platforms = [
  { goos: "linux", goarch: "amd64", platform: "linux", cpu: "x64" },
  { goos: "linux", goarch: "arm64", platform: "linux", cpu: "arm64" },
  { goos: "darwin", goarch: "amd64", platform: "darwin", cpu: "x64" },
  { goos: "darwin", goarch: "arm64", platform: "darwin", cpu: "arm64" },
  { goos: "windows", goarch: "amd64", platform: "win32", cpu: "x64" },
  { goos: "windows", goarch: "arm64", platform: "win32", cpu: "arm64" },
];

function usage() {
  throw new Error("Usage: node npm/scripts/assemble.mjs --version <version> --dist <directory> --out <directory>");
}

function options(argv) {
  const result = {};
  for (let index = 0; index < argv.length; index += 2) {
    const flag = argv[index];
    const value = argv[index + 1];
    if (!flag || !value || !["--version", "--dist", "--out"].includes(flag)) usage();
    result[flag.slice(2)] = value;
  }
  if (!result.version || !result.dist || !result.out) usage();
  return result;
}

async function checksumMap(dist) {
  const text = await readFile(join(dist, "checksums.txt"), "utf8");
  const checksums = new Map();
  for (const line of text.split(/\r?\n/)) {
    const match = line.match(/^([a-fA-F0-9]{64})\s+\*?(.+)$/);
    if (match) checksums.set(match[2], match[1].toLowerCase());
  }
  return checksums;
}

async function verifyArchive(path, expected) {
  const hash = createHash("sha256").update(await readFile(path)).digest("hex");
  if (hash !== expected) {
    throw new Error(`Checksum mismatch for ${basename(path)}.`);
  }
}

function command(commandName, args, cwd) {
  const result = spawnSync(commandName, args, { cwd, stdio: "pipe", encoding: "utf8" });
  if (result.error) throw new Error(`Could not run ${commandName}: ${result.error.message}`);
  if (result.status !== 0) throw new Error(`${commandName} failed: ${result.stderr || result.stdout}`.trim());
}

async function filesUnder(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) files.push(...await filesUnder(path));
    else if (entry.isFile()) files.push(path);
  }
  return files;
}

async function extractBinary(archive, extension, binaryName) {
  const extractDir = await mkdtemp(join(tmpdir(), "devops-daily-npm-"));
  try {
    if (extension === ".tar.gz") {
      command("tar", ["-xzf", archive, "-C", extractDir]);
    } else {
      const unzip = spawnSync("unzip", ["-q", archive, "-d", extractDir], { stdio: "pipe", encoding: "utf8" });
      if (unzip.error && unzip.error.code === "ENOENT") command("tar", ["-xf", archive, "-C", extractDir]);
      else if (unzip.status !== 0) throw new Error(`unzip failed: ${unzip.stderr || unzip.stdout}`.trim());
    }
    const binary = (await filesUnder(extractDir)).find((path) => basename(path) === binaryName);
    if (!binary) throw new Error(`${basename(archive)} does not contain ${binaryName}.`);
    return await readFile(binary);
  } finally {
    await rm(extractDir, { recursive: true, force: true });
  }
}

function packageName(platform) {
  return `@devopsdaily/devops-daily-cli-${platform.platform}-${platform.cpu}`;
}

export async function assemble({ version, dist, out }) {
  dist = resolve(dist);
  out = resolve(out);
  const checksums = await checksumMap(dist);
  await rm(out, { recursive: true, force: true });
  await mkdir(out, { recursive: true });

  for (const platform of platforms) {
    const extension = platform.goos === "windows" ? ".zip" : ".tar.gz";
    const archiveName = `devops-daily_${version}_${platform.goos}_${platform.goarch}${extension}`;
    const expected = checksums.get(archiveName);
    if (!expected) throw new Error(`checksums.txt does not contain ${archiveName}.`);
    const archive = join(dist, archiveName);
    await verifyArchive(archive, expected);

    const binaryName = platform.goos === "windows" ? "devops-daily.exe" : "devops-daily";
    const binary = await extractBinary(archive, extension, binaryName);
    const destination = join(out, packageName(platform));
    await mkdir(join(destination, "bin"), { recursive: true });
    await writeFile(join(destination, "bin", binaryName), binary, { mode: 0o755 });
    await chmod(join(destination, "bin", binaryName), 0o755);
    await writeFile(join(destination, "package.json"), `${JSON.stringify({
      name: packageName(platform),
      version,
      os: [platform.platform],
      cpu: [platform.cpu],
      license: "MIT",
      repository: {
        type: "git",
        url: "git+https://github.com/The-DevOps-Daily/devops-daily-cli.git",
      },
      files: ["bin/"],
    }, null, 2)}\n`);
  }

  const main = join(out, "@devopsdaily", "devops-daily-cli");
  await cp(templateDir, main, { recursive: true });
  const manifestPath = join(main, "package.json");
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  manifest.version = version;
  manifest.optionalDependencies = Object.fromEntries(platforms.map((platform) => [packageName(platform), version]));
  await writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    await assemble(options(process.argv.slice(2)));
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
