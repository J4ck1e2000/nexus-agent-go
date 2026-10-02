// One-off provenance script: re-extracts the i18n dictionaries from the legacy
// web frontend into desktop/src/i18n/{en,zh,login}.ts. Run from the repo root:
//   node desktop/scripts/extract-dict.mjs
// The dashboard dictionary (lines 588-1147 of web/dashboard/index.html) is pure
// data and gets reprinted via JSON; the login dictionary (lines 95-244 of
// web/login/login-app.js) contains function values and is copied as-is.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..');
const outDir = path.join(root, 'desktop', 'src', 'i18n');

// --- dashboard dictionary -------------------------------------------------
const dashboardLines = fs
  .readFileSync(path.join(root, 'web', 'dashboard', 'index.html'), 'utf8')
  .split('\n');
const dictSource = ['module.exports = {', ...dashboardLines.slice(588, 1147)].join('\n');
const tmpPath = '/tmp/nexus-dash-dict.js';
fs.writeFileSync(tmpPath, dictSource);
const dict = createRequire(import.meta.url)(tmpPath);

const quote = (value) => value.replace(/\\/g, '\\\\').replace(/'/g, "\\'").replace(/\n/g, '\\n');

// Pretty-print with single quotes, unquoted keys where safe.
const fmt = (value, indent = 0) => {
  const pad = '    '.repeat(indent);
  const padInner = '    '.repeat(indent + 1);
  if (Array.isArray(value)) {
    if (value.length === 0) return '[]';
    return `[\n${value.map((v) => `${padInner}${fmt(v, indent + 1)}`).join(',\n')}\n${pad}]`;
  }
  if (value === null || typeof value !== 'object') {
    return typeof value === 'string' ? `'${quote(value)}'` : String(value);
  }
  const entries = Object.entries(value).map(([key, v]) => {
    const safeKey = /^[A-Za-z0-9_$]+$/.test(key) ? key : `'${quote(key)}'`;
    return `${padInner}${safeKey}: ${fmt(v, indent + 1)}`;
  });
  return `{\n${entries.join(',\n')}\n${pad}}`;
};

for (const lang of ['en', 'zh']) {
  const body = fmt(dict[lang]);
  fs.writeFileSync(
    path.join(outDir, `${lang}.ts`),
    `import type { Dictionary } from './types';\n\n/** Auto-extracted from web/dashboard/index.html — the behavioral baseline dict. */\nexport const ${lang}: Dictionary = ${body};\n`,
  );
  console.log(lang, Object.keys(dict[lang]).join(','));
}

// --- login dictionary -------------------------------------------------------
const loginLines = fs
  .readFileSync(path.join(root, 'web', 'login', 'login-app.js'), 'utf8')
  .split('\n');
const loginBody = ['/** Auto-extracted from web/login/login-app.js (keeps its function values verbatim). */']
  .concat(loginLines.slice(94, 244))
  .join('\n')
  .replace('const messages = {', 'export const loginMessages = {')
  // The web source relies on implicit-any destructuring; annotate for strict TS.
  .replaceAll('({ errorCode })', '({ errorCode }: { errorCode: string })');
fs.writeFileSync(path.join(outDir, 'login.ts'), `${loginBody}\n`);
console.log('login ok');
