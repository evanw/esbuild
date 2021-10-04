import { downloadedBinPath, pkgAndSubpathForCurrentPlatform } from './node-platform';

import fs = require('fs');
import os = require('os');
import path = require('path');
import zlib = require('zlib');
import https = require('https');
import child_process = require('child_process');

declare const ESBUILD_VERSION: string;
const toPath = path.join(__dirname, 'bin', 'esbuild');
let isToPathJS = true;

function validateBinaryVersion(...command: string[]): void {
  command.push('--version');
  const stdout = child_process.execFileSync(command.shift()!, command).toString().trim();
  if (stdout !== ESBUILD_VERSION) {
    throw new Error(`Expected ${JSON.stringify(ESBUILD_VERSION)} but got ${JSON.stringify(stdout)}`);
  }
}

function isYarn2OrAbove(): boolean {
  const { npm_config_user_agent } = process.env;
  if (npm_config_user_agent) {
    const match = npm_config_user_agent.match(/yarn\/(\d+)/);
    if (match && match[1]) {
      return parseInt(match[1], 10) >= 2;
    }
  }
  return false;
}

function fetch(url: string): Promise<Buffer> {
  return new Promise((resolve, reject) => {
    https.get(url, res => {
      if ((res.statusCode === 301 || res.statusCode === 302) && res.headers.location)
        return fetch(res.headers.location).then(resolve, reject);
      if (res.statusCode !== 200)
        return reject(new Error(`Server responded with ${res.statusCode}`));
      let chunks: Buffer[] = [];
      res.on('data', chunk => chunks.push(chunk));
      res.on('end', () => resolve(Buffer.concat(chunks)));
    }).on('error', reject);
  });
}

function extractFileFromTarGzip(buffer: Buffer, subpath: string): Buffer {
  try {
    buffer = zlib.unzipSync(buffer);
  } catch (err: any) {
    throw new Error(`Invalid gzip data in archive: ${err && err.message || err}`);
  }
  let str = (i: number, n: number) => String.fromCharCode(...buffer.subarray(i, i + n)).replace(/\0.*$/, '');
  let offset = 0;
  subpath = `package/${subpath}`;
  while (offset < buffer.length) {
    let name = str(offset, 100);
    let size = parseInt(str(offset + 124, 12), 8);
    offset += 512;
    if (!isNaN(size)) {
      if (name === subpath) return buffer.subarray(offset, offset + size);
      offset += (size + 511) & ~511;
    }
  }
  throw new Error(`Could not find ${JSON.stringify(subpath)} in archive`);
}

function installUsingNPM(pkg: string, subpath: string, binPath: string): void {
  // Erase "npm_config_global" so that "npm install --global esbuild" works.
  // Otherwise this nested "npm install" will also be global, and the install
  // will deadlock waiting for the global installation lock.
  const env = { ...process.env, npm_config_global: undefined };

  // Create a temporary directory inside the "esbuild" package with an empty
  // "package.json" file. We'll use this to run "npm install" in.
  const esbuildLibDir = path.dirname(require.resolve('esbuild'));
  const installDir = path.join(esbuildLibDir, 'npm-install');
  fs.mkdirSync(installDir);
  try {
    fs.writeFileSync(path.join(installDir, 'package.json'), '{}');

    // Run "npm install" in the temporary directory which should download the
    // desired package. Try to avoid unnecessary log output. This uses the "npm"
    // command instead of a HTTP request so that it hopefully works in situations
    // where HTTP requests are blocked but the "npm" command still works due to,
    // for example, a custom configured npm registry and special firewall rules.
    child_process.execSync(`npm install --loglevel=error --prefer-offline --no-audit --progress=false ${pkg}@${ESBUILD_VERSION}`,
      { cwd: installDir, stdio: 'pipe', env });

    // Move the downloaded binary executable into place. The destination path
    // is the same one that the JavaScript API code uses so it will be able to
    // find the binary executable here later.
    const installedBinPath = path.join(installDir, 'node_modules', pkg, subpath);
    fs.renameSync(installedBinPath, binPath);
  } finally {
    // Try to clean up afterward so we don't unnecessarily waste file system
    // space. Leaving nested "node_modules" directories can also be problematic
    // for certain tools that scan over the file tree and expect it to have a
    // certain structure.
    try {
      removeRecursive(installDir);
    } catch {
      // Removing a file or directory can randomly break on Windows, returning
      // EBUSY for an arbitrary length of time. I think this happens when some
      // other program has that file or directory open (e.g. an anti-virus
      // program). This is fine on Unix because the OS just unlinks the entry
      // but keeps the reference around until it's unused. There's nothing we
      // can do in this case so we just leave the directory there.
    }
  }
}

function removeRecursive(dir: string): void {
  for (const entry of fs.readdirSync(dir)) {
    const entryPath = path.join(dir, entry);
    let stats;
    try {
      stats = fs.lstatSync(entryPath);
    } catch {
      continue; // Guard against https://github.com/nodejs/node/issues/4760
    }
    if (stats.isDirectory()) removeRecursive(entryPath);
    else fs.unlinkSync(entryPath);
  }
  fs.rmdirSync(dir);
}

function applyManualBinaryPathOverride(overridePath: string): void {
  // Patch the CLI use case (the "esbuild" command)
  const pathString = JSON.stringify(overridePath);
  fs.writeFileSync(toPath, `#!/usr/bin/env node\n` +
    `require('child_process').execFileSync(${pathString}, process.argv.slice(2), { stdio: 'inherit' });\n`);

  // Patch the JS API use case (the "require('esbuild')" workflow)
  const libMain = path.join(__dirname, 'lib', 'main.js');
  const code = fs.readFileSync(libMain, 'utf8');
  fs.writeFileSync(libMain, `var ESBUILD_BINARY_PATH = ${pathString};\n${code}`);
}

function maybeOptimizePackage(binPath: string): void {
  // This package contains a "bin/esbuild" JavaScript file that finds and runs
  // the appropriate binary executable. However, this means that running the
  // "esbuild" command runs another instance of "node" which is way slower than
  // just running the binary executable directly.
  //
  // Here we optimize for this by replacing the JavaScript file with the binary
  // executable at install time. This optimization does not work on Windows
  // because on Windows the binary executable must be called "esbuild.exe"
  // instead of "esbuild". This also doesn't work with Yarn 2+ because the Yarn
  // developers don't think binary modules should be used. See this thread for
  // details: https://github.com/yarnpkg/berry/issues/882. This optimization also
  // doesn't apply when npm's "--ignore-scripts" flag is used since in that case
  // this install script will not be run.
  if (os.platform() !== 'win32' && !isYarn2OrAbove()) {
    const tempPath = path.join(__dirname, 'bin-esbuild');
    try {
      // First link the binary with a temporary file. If this fails and throws an
      // error, then we'll just end up doing nothing. This uses a hard link to
      // avoid taking up additional space on the file system.
      fs.linkSync(binPath, tempPath);

      // Then use rename to atomically replace the target file with the temporary
      // file. If this fails and throws an error, then we'll just end up leaving
      // the temporary file there, which is harmless.
      fs.renameSync(tempPath, toPath);

      // If we get here, then we know that the target location is now a binary
      // executable instead of a JavaScript file.
      isToPathJS = false;
    } catch {
      // Ignore errors here since this optimization is optional
    }
  }
}

async function downloadDirectlyFromNPM(pkg: string, subpath: string, binPath: string): Promise<void> {
  // If that fails, the user could have npm configured incorrectly or could not
  // have npm installed. Try downloading directly from npm as a last resort.
  const url = `https://registry.npmjs.org/${pkg}/-/${pkg}-${ESBUILD_VERSION}.tgz`;
  console.error(`[esbuild] Trying to download ${JSON.stringify(url)}`);
  try {
    fs.writeFileSync(binPath, extractFileFromTarGzip(await fetch(url), subpath));
    fs.chmodSync(binPath, 0o755);
  } catch (e: any) {
    console.error(`[esbuild] Failed to download ${JSON.stringify(url)}: ${e && e.message || e}`);
    throw e;
  }
}

async function checkAndPreparePackage(): Promise<void> {
  // This feature was added to give external code a way to modify the binary
  // path without modifying the code itself. Do not remove this because
  // external code relies on this (in addition to esbuild's own test suite).
  if (process.env.ESBUILD_BINARY_PATH) {
    applyManualBinaryPathOverride(process.env.ESBUILD_BINARY_PATH);
    return;
  }

  const { pkg, subpath } = pkgAndSubpathForCurrentPlatform();

  let binPath: string;
  try {
    // First check for the binary package from our "optionalDependencies". This
    // package should have been installed alongside this package at install time.
    binPath = require.resolve(`${pkg}/${subpath}`);
  } catch (e) {
    console.error(`[esbuild] Failed to find package "${pkg}" on the file system

This can happen if you use the "--no-optional" flag. The "optionalDependencies"
package.json feature is used by esbuild to install the correct binary executable
for your current platform. This install script will now attempt to work around
this. If that fails, you need to remove the "--no-optional" flag to use esbuild.
`);

    // If that didn't work, then someone probably installed esbuild with the
    // "--no-optional" flag. Attempt to compensate for this by downloading the
    // package using a nested call to "npm" instead.
    //
    // THIS MAY NOT WORK. Package installation uses "optionalDependencies" for
    // a reason: manually downloading the package has a lot of obscure edge
    // cases that fail because people have customized their environment in
    // some strange way that breaks downloading. This code path is just here
    // to be helpful but it's not the supported way of installing esbuild.
    binPath = downloadedBinPath(pkg, subpath);
    try {
      console.error(`[esbuild] Trying to install package "${pkg}" using npm`);
      installUsingNPM(pkg, subpath, binPath);
    } catch (e2: any) {
      console.error(`[esbuild] Failed to install package "${pkg}" using npm: ${e2 && e2.message || e2}`);

      // If that didn't also work, then something is likely wrong with the "npm"
      // command. Attempt to compensate for this by manually downloading the
      // package from the npm registry over HTTP as a last resort.
      try {
        await downloadDirectlyFromNPM(pkg, subpath, binPath);
      } catch (e3: any) {
        throw new Error(`Failed to install package "${pkg}"`);
      }
    }
  }

  maybeOptimizePackage(binPath);
}

checkAndPreparePackage().then(() => {
  if (isToPathJS) {
    // We need "node" before this command since it's a JavaScript file
    validateBinaryVersion('node', toPath);
  } else {
    // This is no longer a JavaScript file so don't run it using "node"
    validateBinaryVersion(toPath);
  }
});
