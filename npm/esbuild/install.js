const fs = require('fs');
const os = require('os');
const path = require('path');
const child_process = require('child_process');
const version = require('./package.json').version;

// Pick the name of the package to install
let package = 'esbuild-wasm';
if (os.arch() === 'x64') {
  switch (os.platform()) {
    case 'linux': package = 'esbuild-linux-64'; break;
    case 'darwin': package = 'esbuild-darwin-64'; break;
    case 'win32': package = 'esbuild-windows-64'; break;
  }
}

// Clone the environment without "npm_" environment variables. If we don't do
// this, invoking this script via "npm install -g esbuild" will hang because
// our call to "npm install" below will magically be transformed into
// "npm install -g" and, I assume, deadlock waiting for the global lock.
const env = {};
for (const key in process.env) {
  if (!key.startsWith('npm_')) {
    env[key] = process.env[key];
  }
}

// Run "npm install" recursively to install this specific package
const tempDir = path.join(__dirname, '.temp');
fs.mkdirSync(tempDir);
fs.writeFileSync(path.join(tempDir, 'package.json'), '{}');
child_process.execSync(`npm install --silent --prefer-offline --no-audit --progress=false ${package}@${version}`,
  { cwd: tempDir, stdio: 'inherit', env });

// Move the installed files into the node_modules folder we're in
moveFilesRecursive(path.join(tempDir, 'node_modules'), path.dirname(__dirname));

function moveFilesRecursive(source, target) {
  for (const entry of fs.readdirSync(source)) {
    const sourceEntry = path.join(source, entry);
    const targetEntry = path.join(target, entry);
    if (fs.statSync(sourceEntry).isDirectory()) {
      try {
        fs.mkdirSync(targetEntry);
      } catch (e) {
      }
      moveFilesRecursive(sourceEntry, targetEntry);
    } else if (entry !== 'package.json') {
      fs.renameSync(sourceEntry, targetEntry);
    } else {
      fs.unlinkSync(sourceEntry);
    }
  }
  fs.rmdirSync(source);
}
