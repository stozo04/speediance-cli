#!/usr/bin/env node
import { readFile } from "node:fs/promises";
import { pathToFileURL } from "node:url";
import { join, resolve } from "node:path";

const SHIPPABLE = [
  /^SKILL\.md$/,
  /^SECURITY\.md$/,
  /^config\.example\.json$/,
  /^docs\/MACHINE_CONTRACT\.md$/,
];

const REQUIRED = ["SKILL.md"];

const PUBLISH_WORKFLOW = ".github/workflows/publish-clawhub.yml";

function globToRegExp(glob) {
  let source = "";
  for (let i = 0; i < glob.length; i++) {
    const c = glob[i];
    if (c === "*" && glob[i + 1] === "*") {
      source += glob[i + 2] === "/" ? "(?:.*/)?" : ".*";
      i += glob[i + 2] === "/" ? 2 : 1;
    } else if (c === "*") {
      source += "[^/]*";
    } else if (c === "?") {
      source += "[^/]";
    } else {
      source += c.replace(/[.+^${}()|[\]\\]/g, "\\$&");
    }
  }
  return new RegExp(`^${source}$`);
}

function publishTriggerPaths(workflow) {
  const lines = workflow.split("\n");
  const start = lines.findIndex((line) => /^ {4}paths:\s*$/.test(line));
  if (start === -1) return null;
  const patterns = [];
  for (const line of lines.slice(start + 1)) {
    const item = line.match(/^ {6}- (.+?)\s*$/);
    if (!item) break;
    patterns.push(item[1].replace(/^(["'])(.*)\1$/, "$2"));
  }
  return patterns;
}

function triggersPublish(file, patterns) {
  let triggered = false;
  for (const pattern of patterns) {
    const negated = pattern.startsWith("!");
    if (globToRegExp(negated ? pattern.slice(1) : pattern).test(file)) triggered = !negated;
  }
  return triggered;
}

function usage() {
  console.error("usage: check-clawhub-package.mjs <unpacked clawhub package dir> [repo root]");
  process.exit(2);
}

const [clawhubDir, repoArg] = process.argv.slice(2);
if (!clawhubDir) usage();
const repoRoot = resolve(repoArg ?? ".");

const triggerSelfTest = [
  ["internal/cli/root.go", true],
  ["internal/cli/root_test.go", false],
  ["internal/cli/testdata/x.json", false],
  ["README.md", true],
];
for (const [file, want] of triggerSelfTest) {
  if (triggersPublish(file, ["README.md", "internal/**", "!**/*_test.go", "!**/testdata/**"]) !== want) {
    console.error(`publish-trigger matcher is wrong for ${file}`);
    process.exit(2);
  }
}

const walkerURL = pathToFileURL(join(resolve(clawhubDir), "dist", "publishFiles.js")).href;
const { forEachPublishFile } = await import(walkerURL);

const shipped = [];
await forEachPublishFile(repoRoot, { includeDotfiles: false, respectGitignore: true }, (_abs, rel) => {
  shipped.push(rel);
});
shipped.sort();

const unexpected = shipped.filter((f) => !SHIPPABLE.some((re) => re.test(f)));
const missing = REQUIRED.filter((f) => !shipped.includes(f));

const triggerPaths = publishTriggerPaths(await readFile(join(repoRoot, PUBLISH_WORKFLOW), "utf8"));
const untriggered = triggerPaths === null ? [] : shipped.filter((f) => !triggersPublish(f, triggerPaths));
if (triggerPaths === null) {
  console.error(`no on.push.paths list found in ${PUBLISH_WORKFLOW}`);
  process.exit(1);
}


console.log(`clawhub would publish ${shipped.length} files:`);
for (const f of shipped) console.log(`  ${f}`);

if (unexpected.length || missing.length || untriggered.length) {
  for (const f of unexpected) console.error(`not shippable (add it to .clawhubignore or SHIPPABLE): ${f}`);
  for (const f of missing) console.error(`required file missing from the package: ${f}`);
  for (const f of untriggered) console.error(`shipped but a change to it would not republish (add it to on.push.paths in ${PUBLISH_WORKFLOW}): ${f}`);
  process.exit(1);
}
