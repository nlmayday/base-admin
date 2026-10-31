// Checks that every <el-*> tag used in the pages, layouts and code generator
// templates is registered in src/plugins/elementPlus.ts; an unregistered
// component renders as nothing. Run by `pnpm lint`.
import { readFileSync, readdirSync, statSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const web = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const roots = [
  path.join(web, "src"),
  path.join(web, "../server/internal/devtools/codegen/templates")
];

function files(dir) {
  return readdirSync(dir).flatMap(name => {
    const full = path.join(dir, name);
    if (statSync(full).isDirectory()) return files(full);
    return /\.(vue|tsx|tmpl)$/.test(name) ? [full] : [];
  });
}

const used = new Map();
for (const root of roots) {
  for (const file of files(root)) {
    for (const [, tag] of readFileSync(file, "utf8").matchAll(
      /<(el-[a-z-]+)/g
    )) {
      if (!used.has(tag)) used.set(tag, path.relative(web, file));
    }
  }
}

const registry = readFileSync(
  path.join(web, "src/plugins/elementPlus.ts"),
  "utf8"
);
const registered = new Set(registry.match(/\bEl[A-Z][A-Za-z]+/g));
const toName = tag =>
  "El" +
  tag
    .slice(3)
    .split("-")
    .map(p => p[0].toUpperCase() + p.slice(1))
    .join("");

const missing = [...used].filter(([tag]) => !registered.has(toName(tag)));
if (missing.length) {
  for (const [tag, file] of missing) {
    console.error(
      `未注册的 Element Plus 组件 <${tag}>（${file}）：请加到 src/plugins/elementPlus.ts`
    );
  }
  process.exit(1);
}
console.log(`Element Plus 组件检查通过（${used.size} 个）`);
