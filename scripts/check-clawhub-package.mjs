#!/usr/bin/env node
import { pathToFileURL } from "node:url";
import { join, resolve } from "node:path";

const SHIPPABLE = [
  /^SKILL\.md$/,
  /^README\.md$/,
  /^LICENSE$/,
  /^SECURITY\.md$/,
  /^config\.example\.json$/,
  /^docs\/MACHINE_CONTRACT\.md$/,
  /^go\.(mod|sum)$/,
  /^(cmd|internal)\/.+(?<!_test)\.go$/,
];

const REQUIRED = ["SKILL.md", "go.mod", "cmd/speediance-cli/main.go"];

function usage() {
  console.error("usage: check-clawhub-package.mjs <unpacked clawhub package dir> [repo root]");
  process.exit(2);
}

const [clawhubDir, repoArg] = process.argv.slice(2);
if (!clawhubDir) usage();
const repoRoot = resolve(repoArg ?? ".");

const walkerURL = pathToFileURL(join(resolve(clawhubDir), "dist", "publishFiles.js")).href;
const { forEachPublishFile } = await import(walkerURL);

const shipped = [];
await forEachPublishFile(repoRoot, { includeDotfiles: false, respectGitignore: true }, (_abs, rel) => {
  shipped.push(rel);
});
shipped.sort();

const unexpected = shipped.filter((f) => !SHIPPABLE.some((re) => re.test(f)));
const missing = REQUIRED.filter((f) => !shipped.includes(f));

console.log(`clawhub would publish ${shipped.length} files:`);
for (const f of shipped) console.log(`  ${f}`);

if (unexpected.length || missing.length) {
  for (const f of unexpected) console.error(`not shippable (add it to .clawhubignore or SHIPPABLE): ${f}`);
  for (const f of missing) console.error(`required file missing from the package: ${f}`);
  process.exit(1);
}
