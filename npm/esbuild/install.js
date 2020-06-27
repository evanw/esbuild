const fs = require('fs');
const os = require('os');
const path = require('path');
const child_process = require('child_process');
const version = require('./package.json').version;
const installDir = path.join(__dirname, '.install');
const binPath = path.join(__dirname, 'bin', 'esbuild');

function installPackage(package) {
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

  // It turns out that some package managers (e.g. yarn) sometimes re-run the
  // postinstall script for this package after we have already been installed.
  // That means this script must be idempotent. Let's skip the install if it's
  // already happened.
  if (fs.existsSync(installDir)) {
    return false;
  }

  // Run "npm install" recursively to install this specific package
  fs.mkdirSync(installDir);
  fs.writeFileSync(path.join(installDir, 'package.json'), '{}');
  child_process.execSync(`npm install --silent --prefer-offline --no-audit --progress=false ${package}@${version}`,
    { cwd: installDir, stdio: 'inherit', env });
  return true;
}

function installOnUnix(package) {
  if (process.env.ESBUILD_BIN_PATH_FOR_TESTS) {
    fs.unlinkSync(binPath);
    fs.symlinkSync(process.env.ESBUILD_BIN_PATH_FOR_TESTS, binPath);
  } else if (installPackage(package)) {
    fs.renameSync(
      path.join(installDir, 'node_modules', package, 'bin', 'esbuild'),
      binPath
    );
  }
}

function installOnWindows() {
  const exePath = path.join(__dirname, 'esbuild.exe');
  if (process.env.ESBUILD_BIN_PATH_FOR_TESTS) {
    fs.symlinkSync(process.env.ESBUILD_BIN_PATH_FOR_TESTS, exePath);
  } else if (installPackage('esbuild-windows-64')) {
    fs.renameSync(
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

function tryCompileAndInstall() {
  try {
    const env = {};
    for (const key in process.env) {
      if (!key.startsWith('npm_')) {
        env[key] = process.env[key];
      }
    }
    env['GOPATH'] = installDir;
    env['GOCACHE'] = installDir;
    env['GO111MODULE'] = 'on';

    // It turns out that some package managers (e.g. yarn) sometimes re-run the
    // postinstall script for this package after we have already been installed.
    // That means this script must be idempotent. Let's skip the install if it's
    // already happened.
    if (fs.existsSync(installDir)) {
      return true;
    }

    fs.mkdirSync(installDir);

    child_process.execSync(`go get github.com/evanw/esbuild/...@v${version}`,
      { cwd: installDir, stdio: 'inherit', env });

    fs.renameSync(
      path.join(installDir, 'bin', 'esbuild'),
      binPath
    );

    // Clean installDir contents
    child_process.execSync(`go clean -modcache -cache`,
      { cwd: installDir, stdio: 'inherit', env });

    return true;
  } catch (e) {
    console.error(e);
    return false;
  }
}

// Pick a package to install
if (process.platform === 'linux' && os.arch() === 'x64') {
  installOnUnix('esbuild-linux-64');
} else if (process.platform === 'linux' && os.arch() === 'arm64') {
  installOnUnix('esbuild-linux-arm64');
} else if (process.platform === 'linux' && os.arch() === 'ppc64' && os.endianness() === 'LE') {
  installOnUnix('esbuild-linux-ppc64le');
} else if (process.platform === 'darwin' && os.arch() === 'x64') {
  installOnUnix('esbuild-darwin-64');
} else if (process.platform === 'win32' && os.arch() === 'x64') {
  installOnWindows();
} else if (!tryCompileAndInstall()) {
  console.error(`error: Unsupported platform: ${process.platform} ${os.arch()}`);
  process.exit(1);
}
