// This script parses all .ts and .tsx files in the current directory using
// esbuild. This is useful to check for parser bugs and/or crashes in esbuild.

const fs = require('fs');
const os = require('os');
const path = require('path');
const ts = require('typescript');
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
    let output = child_process.spawnSync(esbuildPath, [absolute, '--outfile=/dev/null'], { stdio: ['inherit', 'pipe', 'pipe'] });
    if (output.status) {
      let result;
      try {
        result = ts.transpileModule(fs.readFileSync(absolute, 'utf8'), { reportDiagnostics: true });
      } catch (e) {
        // Ignore this file if the TypeScript compiler crashes on it
        return
      }
      if (result.diagnostics.length > 0) {
        // Ignore this file if the TypeScript compiler has parse errors
        return
      }
      console.log('-'.repeat(80));
      console.log('Failure:', absolute);
      console.log('-'.repeat(20) + ' esbuild output:');
      console.log(output.stdout + output.stderr);
    }
  });
}

// Otherwise it's much faster to do everything at once
else {
  const tempDir = path.join(os.tmpdir(), '@cspotcode-esbuild-parse-ts-files');
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
