// This script parses all .ts and .tsx files in the current directory using
// esbuild. This is useful to check for parser bugs and/or crashes in esbuild.

const fs = require('fs');
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
walkDir(process.cwd(), absolute => {
  console.log('Parsing', absolute);
  try {
    child_process.execFileSync(esbuildPath, [absolute, '--outfile=/dev/null'], { stdio: 'inherit' });
  } catch (e) {
  }
});
