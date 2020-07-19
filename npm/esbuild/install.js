const fs = require('fs');
const os = require('os');
const path = require('path');
const zlib = require('zlib');
const https = require('https');
const version = require('./package.json').version;
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

  // Download the package from npm
  let url = `https://registry.npmjs.org/${package}/-/${package}-${version}.tgz`
  downloadURL(url, buffer => {
    // Extract the binary executable from the package
    fs.writeFileSync(toPath, extractFileFromTarGzip(buffer, fromPath));

    // Mark the operation as successful so this script is idempotent
    fs.writeFileSync(stampPath, '');
  });
}

function downloadURL(url, done) {
  https.get(url, res => {
    let chunks = [];
    res.on('data', chunk => chunks.push(chunk));
    res.on('end', () => done(Buffer.concat(chunks)));
  }).on('error', err => { throw err; });
}

function extractFileFromTarGzip(buffer, file) {
  buffer = zlib.unzipSync(buffer);
  let str = (i, n) => String.fromCharCode(...buffer.subarray(i, i + n)).replace(/\0.*$/, '');
  let offset = 0;
  while (offset < buffer.length) {
    let name = str(offset, 100);
    let size = parseInt(str(offset + 124, 12), 8);
    offset += 512;
    if (!isNaN(size)) {
      if (name === file) return buffer.subarray(offset, offset + size);
      offset += (size + 511) & ~511;
    }
  }
  throw new Error(`Could not find ${JSON.stringify(file)}`)
}

function installOnUnix(package) {
  if (process.env.ESBUILD_BIN_PATH_FOR_TESTS) {
    fs.unlinkSync(binPath);
    fs.symlinkSync(process.env.ESBUILD_BIN_PATH_FOR_TESTS, binPath);
  } else {
    installBinaryFromPackage(package, 'package/bin/esbuild', binPath);
  }
}

function installOnWindows() {
  fs.writeFileSync(
    binPath,
    `#!/usr/bin/env node
const path = require('path');
const esbuild_exe = path.join(__dirname, '..', 'esbuild.exe');
const child_process = require('child_process');
child_process.spawnSync(esbuild_exe, process.argv.slice(2), { stdio: 'inherit' });
`);
  const exePath = path.join(__dirname, 'esbuild.exe');
  if (process.env.ESBUILD_BIN_PATH_FOR_TESTS) {
    fs.symlinkSync(process.env.ESBUILD_BIN_PATH_FOR_TESTS, exePath);
  } else {
    installBinaryFromPackage('esbuild-windows-64', 'package/esbuild.exe', exePath);
  }
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
