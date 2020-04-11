// This script parses all .ts and .tsx files in the current directory using
// esbuild. This is useful to check for parser bugs and/or crashes in esbuild.

const fs = require('fs');
const os = require('os');
const path = require('path');
const child_process = require('child_process');
const esbuildPath = path.join(path.dirname(__dirname), 'esbuild');

function walkDir(root, cb) {
  for (const entry of fs.readdirSync(root)) {
    const absolute = path.join(root, entry);
    if (fs.statSync(absolute).isDirectory()) {
      walkDir(absolute, cb);
    } else if ((entry.endsWith('.ts') && !entry.endsWith('.d.ts')) || entry.endsWith('.tsx')) {
      cb(absolute)
    }
  }
}

child_process.execSync('make', { cwd: path.dirname(__dirname) });

// Doing one file at a time is useful for debugging crashes
if (process.argv.includes('--individual')) {
  walkDir(process.cwd(), absolute => {
    console.log('Parsing', absolute);
    try {
      child_process.execFileSync(esbuildPath, [absolute, '--outfile=/dev/null'], { stdio: 'inherit' });
    } catch (e) {
    }
  });
}

// Otherwise it's much faster to do everything at once
else {
  const tempDir = path.join(os.tmpdir(), 'esbuild-parse-ts-files');
  try {
    fs.mkdirSync(tempDir);
  } catch (e) {
  }
  const all = [];
  walkDir(process.cwd(), absolute => all.push(absolute));
  try {
    child_process.execFileSync(esbuildPath, ['--outdir=' + tempDir].concat(all), { stdio: 'inherit' });
  } catch (e) {
  }
}
