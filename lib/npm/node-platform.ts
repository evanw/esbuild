import fs = require('fs');
import os = require('os');
import path = require('path');

// This feature was added to give external code a way to modify the binary
// path without modifying the code itself. Do not remove this because
// external code relies on this.
var ESBUILD_BINARY_PATH: string | undefined = process.env.ESBUILD_BINARY_PATH || ESBUILD_BINARY_PATH;

export const knownWindowsPackages: Record<string, string> = {
  'win32 arm64 LE': 'esbuild-windows-arm64',
  'win32 ia32 LE': 'esbuild-windows-32',
  'win32 x64 LE': 'esbuild-windows-64',
};

export const knownUnixlikePackages: Record<string, string> = {
  'android arm64 LE': 'esbuild-android-arm64',
  'darwin arm64 LE': 'esbuild-darwin-arm64',
  'darwin x64 LE': 'esbuild-darwin-64',
  'freebsd arm64 LE': 'esbuild-freebsd-arm64',
  'freebsd x64 LE': 'esbuild-freebsd-64',
  'linux arm LE': 'esbuild-linux-arm',
  'linux arm64 LE': 'esbuild-linux-arm64',
  'linux ia32 LE': 'esbuild-linux-32',
  'linux mips64el LE': 'esbuild-linux-mips64le',
  'linux ppc64 LE': 'esbuild-linux-ppc64le',
  'linux x64 LE': 'esbuild-linux-64',
  'netbsd x64 LE': 'esbuild-netbsd-64',
  'openbsd x64 LE': 'esbuild-openbsd-64',
  'sunos x64 LE': 'esbuild-sunos-64',
};

export function pkgAndSubpathForCurrentPlatform(): { pkg: string, subpath: string } {
  let pkg: string;
  let subpath: string;
  let platformKey = `${process.platform} ${os.arch()} ${os.endianness()}`;

  if (platformKey in knownWindowsPackages) {
    pkg = knownWindowsPackages[platformKey];
    subpath = 'esbuild.exe';
  }

  else if (platformKey in knownUnixlikePackages) {
    pkg = knownUnixlikePackages[platformKey];
    subpath = 'bin/esbuild';
  }

  else {
    throw new Error(`Unsupported platform: ${platformKey}`);
  }

  return { pkg, subpath };
}

export function downloadedBinPath(pkg: string, subpath: string): string {
  const esbuildLibDir = path.dirname(require.resolve('esbuild'));
  return path.join(esbuildLibDir, `downloaded-${pkg}-${path.basename(subpath)}`);
}

export function generateBinPath(): string {
  // This feature was added to give external code a way to modify the binary
  // path without modifying the code itself. Do not remove this because
  // external code relies on this (in addition to esbuild's own test suite).
  if (ESBUILD_BINARY_PATH) {
    return ESBUILD_BINARY_PATH;
  }

  const { pkg, subpath } = pkgAndSubpathForCurrentPlatform();
  let binPath: string;

  try {
    // First check for the binary package from our "optionalDependencies". This
    // package should have been installed alongside this package at install time.
    binPath = require.resolve(`${pkg}/${subpath}`);
  } catch (e) {
    // If that didn't work, then someone probably installed esbuild with the
    // "--no-optional" flag. Our install script attempts to compensate for this
    // by manually downloading the package instead. Check for that next.
    binPath = downloadedBinPath(pkg, subpath);
    if (!fs.existsSync(binPath)) {
      // If that didn't work too, then we're out of options. This can happen
      // when someone installs esbuild with both the "--no-optional" and the
      // "--ignore-scripts" flags. The fix for this is to just not do that.
      //
      // In that case we try to have a nice error message if we think we know
      // what's happening. Otherwise we just rethrow the original error message.
      try {
        require.resolve(pkg);
      } catch {
        throw new Error(`The package "${pkg}" could not be found, and is needed by esbuild.

If you are installing esbuild with npm, make sure that you don't specify the
"--no-optional" flag. The "optionalDependencies" package.json feature is used
by esbuild to install the correct binary executable for your current platform.`);
      }
      throw e;
    }
  }

  // The esbuild binary executable can't be used in Yarn 2 in PnP mode because
  // it's inside a virtual file system and the OS needs it in the real file
  // system. So we need to copy the file out of the virtual file system into
  // the real file system.
  let isYarnPnP = false;
  try {
    require('pnpapi');
    isYarnPnP = true;
  } catch (e) {
  }
  if (isYarnPnP) {
    const esbuildLibDir = path.dirname(require.resolve('esbuild'));
    const binTargetPath = path.join(esbuildLibDir, `pnpapi-${pkg}-${path.basename(subpath)}`);
    if (!fs.existsSync(binTargetPath)) {
      fs.copyFileSync(binPath, binTargetPath);
      fs.chmodSync(binTargetPath, 0o755);
    }
    return binTargetPath;
  }

  return binPath;
}
