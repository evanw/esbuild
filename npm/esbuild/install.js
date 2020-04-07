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

// Run "npm install" recursively to install this specific package
const tempDir = path.join(__dirname, '.temp');
fs.mkdirSync(tempDir);
fs.writeFileSync(path.join(tempDir, 'package.json'), '{}');
child_process.execSync(`npm install --silent --prefer-offline --no-audit --progress=false ${package}@${version}`,
  { cwd: tempDir, stdio: 'inherit' });

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
      fs.rmdirSync(sourceEntry);
    } else if (entry !== 'package.json') {
      fs.renameSync(sourceEntry, targetEntry);
    } else {
      fs.unlinkSync(sourceEntry);
    }
  }
}
