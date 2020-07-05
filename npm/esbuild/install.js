const fs = require('fs');
const os = require('os');
const path = require('path');
const child_process = require('child_process');
const version = require('./package.json').version;
const installDir = path.join(__dirname, '.install');
const binPath = path.join(__dirname, 'bin', 'esbuild');
const stampPath = path.join(__dirname, 'stamp.txt');

function installBinaryFromPackage(package, fromPath, toPath) {
  // It turns out that some package managers (e.g. yarn) sometimes re-run the
  // postinstall script for this package after we have already been installed.
  // That means this script must be idempotent. Let's skip the install if it's
  // already happened.
  if (fs.existsSync(stampPath)) {
    return;
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
  fs.mkdirSync(installDir);
  fs.writeFileSync(path.join(installDir, 'package.json'), '{}');
  child_process.execSync(`npm install --silent --prefer-offline --no-audit --progress=false ${package}@${version}`,
    { cwd: installDir, stdio: 'inherit', env });

  // Move the binary from the nested package into our package
  fs.renameSync(fromPath, toPath);

  // Remove the install directory afterwards to avoid tripping up tools that scan
  // for nested directories named "node_modules" and make assumptions. See this
  // issue for an example: https://github.com/ds300/patch-package/issues/243
  removeRecursive(installDir);

  // Mark the operation as successful so this script is idempotent
  fs.writeFileSync(stampPath, '');
}

function removeRecursive(dir) {
  for (const entry of fs.readdirSync(dir)) {
    const entryPath = path.join(dir, entry);
    let stats;
    try {
      stats = fs.lstatSync(entryPath);
    } catch (e) {
      continue; // Guard against https://github.com/nodejs/node/issues/4760
    }
    if (stats.isDirectory()) {
      removeRecursive(entryPath);
    } else {
      fs.unlinkSync(entryPath);
    }
  }
  fs.rmdirSync(dir);
}

function installOnUnix(package) {
  if (process.env.ESBUILD_BIN_PATH_FOR_TESTS) {
    fs.unlinkSync(binPath);
    fs.symlinkSync(process.env.ESBUILD_BIN_PATH_FOR_TESTS, binPath);
  } else {
    installBinaryFromPackage(
      package,
      path.join(installDir, 'node_modules', package, 'bin', 'esbuild'),
      binPath
    );
  }
}

function installOnWindows() {
  const exePath = path.join(__dirname, 'esbuild.exe');
  if (process.env.ESBUILD_BIN_PATH_FOR_TESTS) {
    fs.symlinkSync(process.env.ESBUILD_BIN_PATH_FOR_TESTS, exePath);
  } else {
    installBinaryFromPackage(
      'esbuild-windows-64',
      path.join(installDir, 'node_modules', 'esbuild-windows-64', 'esbuild.exe'),
      exePath
    );
  }
  fs.writeFileSync(
    binPath,
    `#!/usr/bin/env node
const path = require('path');
const esbuild_exe = path.join(__dirname, '..', 'esbuild.exe');
const child_process = require('child_process');
child_process.spawnSync(esbuild_exe, process.argv.slice(2), { stdio: 'inherit' });
`
  );
}

const knownUnixlikePackages = {
  'darwin x64 LE': 'esbuild-darwin-64',
  'freebsd x64 LE': 'esbuild-freebsd-64',
  'freebsd arm64 LE': 'esbuild-freebsd-arm64',
  'linux x64 LE': 'esbuild-linux-64',
  'linux arm64 LE': 'esbuild-linux-arm64',
  'linux ppc64 LE': 'esbuild-linux-ppc64le',
};

// Pick a package to install
if (process.platform === 'win32' && os.arch() === 'x64') {
  installOnWindows();
} else {
  const key = `${process.platform} ${os.arch()} ${os.endianness()}`;
  const package = knownUnixlikePackages[key];
  if (package) {
    installOnUnix(package);
  } else {
    console.error(`error: Unsupported platform: ${key}`);
    process.exit(1);
  }
}
