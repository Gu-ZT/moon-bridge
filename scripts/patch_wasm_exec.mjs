import { readFileSync, writeFileSync, copyFileSync, mkdirSync } from 'fs';
import { dirname } from 'path';
import { execSync } from 'child_process';

const dest = process.argv[2] || 'build/wasm_exec.js';

const tgr = execSync('tinygo env TINYGOROOT', { encoding: 'utf8' }).trim();
mkdirSync(dirname(dest), { recursive: true });
copyFileSync(`${tgr}/targets/wasm_exec.js`, dest);
console.log(`Copied TinyGo wasm_exec.js -> ${dest}`);

let s = readFileSync(dest, 'utf8');
const lines = s.split('\n');

// Helper: comment out a brace-delimited block starting at a given line
function commentBlock(startLine, marker) {
  for (let i = 0; i < lines.length; i++) {
    if (!lines[i].includes(marker)) continue;
    let depth = 0;
    for (let j = i; j < lines.length; j++) {
      depth += (lines[j].match(/{/g) || []).length - (lines[j].match(/}/g) || []).length;
      if (depth <= 0 && j > i) {
        lines[i] = '/* ' + lines[i];
        lines[j] = lines[j] + ' */';
        return true;
      }
    }
  }
  return false;
}

// Workers adaptations
commentBlock(0, 'if (!global.require && typeof require !== "undefined")');
commentBlock(0, 'if (!global.fs && global.require)');
commentBlock(0, 'if (!global.crypto)');
commentBlock(0, 'if (!global.TextEncoder)');
commentBlock(0, 'if (!global.TextDecoder)');

// Remove WASI error constants
for (let i = 0; i < lines.length; i++) {
  if (lines[i]?.includes('const wasi_EBADF = 8;') && lines[i + 1]?.includes('const wasi_ENOSYS = 52;')) {
    lines.splice(i, 2); break;
  }
}

// Remove unused WASI stubs
for (const stub of [
  'fd_read: () => wasi_ENOSYS,',
  'fd_prestat_get: () => wasi_EBADF',
  'fd_prestat_dir_name: () => wasi_ENOSYS,',
  'path_open: () => wasi_ENOSYS,',
]) {
  const idx = lines.findIndex(l => l.includes(stub.replace(/,\s*$/, '')));
  if (idx >= 0) lines.splice(idx, 1);
}

// Simplify remaining WASI stubs
for (let i = 0; i < lines.length; i++) {
  if (lines[i].includes('fd_close: () => wasi_ENOSYS')) lines[i] = lines[i].replace('wasi_ENOSYS,', '0,      // dummy');
  if (lines[i].includes('fd_fdstat_get: () => wasi_ENOSYS')) lines[i] = lines[i].replace('wasi_ENOSYS,', '0, // dummy');
  if (lines[i].includes('fd_prestat_get: () => wasi_EBADF')) lines[i] = lines[i].replace('wasi_EBADF,', '0, // dummy');
	if (lines[i].includes('fd_seek: () => wasi_ENOSYS')) lines[i] = lines[i].replace('wasi_ENOSYS,', '0,       // dummy');
}

// Update WASI link comment
for (let i = 0; i < lines.length; i++) {
  if (lines[i].includes('WASI/blob/snapshot-01')) {
    lines[i] = lines[i].replace('snapshot-01/phases/snapshot/docs.md', 'main/phases/snapshot/docs.md#fd_write');
    break;
  }
}

// Comment out Node.js CLI guard (starts with "if (global.require &&")
commentBlock(0, 'global.require.main === module');

// Add context parameter and globalProxy to run()
for (let i = 0; i < lines.length; i++) {
  if (lines[i].trim() === 'async run(instance) {') {
    lines[i] = [
      'async run(instance, context) {',
      '\t\t\tconst globalProxy = new Proxy(global, {',
      '\t\t\t\tget(target, prop) {',
      '\t\t\t\t\treturn Reflect.get(...arguments);',
      '\t\t\t\t}',
      '\t\t\t})',
    ].join('\n');
    break;
  }
}

// Replace global with globalProxy in the values array
for (let i = 0; i < lines.length; i++) {
  const pre = lines[i - 1] || '';
  if (lines[i].trim() === 'global,' && pre.includes('globalProxy')) {
    lines[i] = lines[i].replace('global,', 'globalProxy,');
    break;
  }
}

// Add runtime.getRandomData
if (!s.includes('"runtime.getRandomData"')) {
  for (let i = 0; i < lines.length; i++) {
    if (lines[i].includes('"runtime.sleepTicks"')) {
      for (let j = i; j < lines.length; j++) {
        if (lines[j].trim() === '},' && lines[j].startsWith('\t\t\t\t')) {
          lines[j] += [
            '\n\t\t\t\t"runtime.getRandomData": (sp) => {',
            '\t\t\t\t\tsp >>>= 0;',
            '\t\t\t\t\tcrypto.getRandomValues(loadSlice(sp + 8));',
            '\t\t\t\t},',
          ].join('\n');
          break;
        }
      }
      console.log('Added runtime.getRandomData');
      break;
    }
  }
}

// Patch worker.mjs to set __workersCtx before WASM instantiation
const workerPath = dest.replace('wasm_exec.js', 'worker.mjs');
try {
  let wmjs = readFileSync(workerPath, 'utf8');
  if (!wmjs.includes('globalThis.context = ctx')) {
    wmjs = wmjs.replace(
      'const instance = new WebAssembly.Instance(mod,',
      'globalThis.context = ctx;\n  const instance = new WebAssembly.Instance(mod,'
    );
    writeFileSync(workerPath, wmjs);
    console.log('Patched worker.mjs with __workersCtx');
  } else {
    console.log('worker.mjs already patched');
  }
} catch (e) {
  console.log('worker.mjs not found, skipping');
}

// Write result
writeFileSync(dest, lines.join('\n'));

// Verify comment balance
let depth = 0, errs = 0;
for (let i = 0; i < lines.length; i++) {
  const opens = (lines[i].match(/\/\*/g) || []).length;
  const closes = (lines[i].match(/\*\//g) || []).length;
  depth += opens - closes;
  if (depth < 0) { errs++; console.log(`Extra */ at line ${i+1}: ${lines[i].trim()}`); depth = 0; }
}
if (depth > 0) console.log(`Unclosed /* at EOF (depth=${depth})`);
console.log(errs === 0 && depth === 0 ? 'Comment balance OK' : 'Comment balance FAILED');

console.log(`Done: ${lines.length} lines`);
