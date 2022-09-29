import fs = require('fs');
import os = require('os');
import path = require('path');

declare const ESBUILD_VERSION: string;

// This feature was added to give external code a way to modify the binary
// path without modifying the code itself. Do not remove this because
// external code relies on this.
export var ESBUILD_BINARY_PATH: string | undefined = process.env.ESBUILD_BINARY_PATH || ESBUILD_BINARY_PATH;

const packageDarwin_arm64 = 'esbuild-darwin-arm64'
const packageDarwin_x64 = 'esbuild-darwin-64'

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
  'linux riscv64 LE': 'esbuild-linux-riscv64',
  'linux s390x BE': 'esbuild-linux-s390x',
  'linux x64 LE': 'esbuild-linux-64',
  'linux loong64 LE': '@esbuild/linux-loong64',
  'netbsd x64 LE': 'esbuild-netbsd-64',
  'openbsd x64 LE': 'esbuild-openbsd-64',
  'sunos x64 LE': 'esbuild-sunos-64',
};

export const knownWebAssemblyFallbackPackages: Record<string, string> = {
  'android arm LE': '@esbuild/android-arm',
  'android x64 LE': 'esbuild-android-64',
};

export function pkgAndSubpathForCurrentPlatform(): { pkg: string, subpath: string, isWASM: boolean } {
  let pkg: string;
  let subpath: string;
  let isWASM = false;
  let platformKey = `${process.platform} ${os.arch()} ${os.endianness()}`;

  if (platformKey in knownWindowsPackages) {
    pkg = knownWindowsPackages[platformKey];
    subpath = 'esbuild.exe';
  }

  else if (platformKey in knownUnixlikePackages) {
    pkg = knownUnixlikePackages[platformKey];
    subpath = 'bin/esbuild';
  }

  else if (platformKey in knownWebAssemblyFallbackPackages) {
    pkg = knownWebAssemblyFallbackPackages[platformKey];
    subpath = 'bin/esbuild';
    isWASM = true;
  }

  else {
    throw new Error(`Unsupported platform: ${platformKey}`);
  }

  return { pkg, subpath, isWASM };
}

function pkgForSomeOtherPlatform(): string | null {
  const libMainJS = require.resolve('esbuild');
  const nodeModulesDirectory = path.dirname(path.dirname(path.dirname(libMainJS)));

  if (path.basename(nodeModulesDirectory) === 'node_modules') {
    for (const unixKey in knownUnixlikePackages) {
      try {
        const pkg = knownUnixlikePackages[unixKey];
        if (fs.existsSync(path.join(nodeModulesDirectory, pkg))) return pkg;
      } catch {
      }
    }

    for (const windowsKey in knownWindowsPackages) {
      try {
        const pkg = knownWindowsPackages[windowsKey];
        if (fs.existsSync(path.join(nodeModulesDirectory, pkg))) return pkg;
      } catch {
      }
    }
  }

  return null;
}

export function downloadedBinPath(pkg: string, subpath: string): string {
  const esbuildLibDir = path.dirname(require.resolve('esbuild'));
  return path.join(esbuildLibDir, `downloaded-${pkg}-${path.basename(subpath)}`);
}

export function generateBinPath(): { binPath: string, isWASM: boolean } {
  // This feature was added to give external code a way to modify the binary
  // path without modifying the code itself. Do not remove this because
  // external code relies on this (in addition to esbuild's own test suite).
  if (ESBUILD_BINARY_PATH) {
    return { binPath: ESBUILD_BINARY_PATH, isWASM: false };
  }

  const { pkg, subpath, isWASM } = pkgAndSubpathForCurrentPlatform();
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
      // If that didn't work too, check to see whether the package is even there
      // at all. It may not be (for a few different reasons).
      try {
        require.resolve(pkg);
      } catch {
        // If we can't find the package for this platform, then it's possible
        // that someone installed this for some other platform and is trying
        // to use it without reinstalling. That won't work of course, but
        // people do this all the time with systems like Docker. Try to be
        // helpful in that case.
        const otherPkg = pkgForSomeOtherPlatform();
        if (otherPkg) {
          let suggestions = `
Specifically the "${otherPkg}" package is present but this platform
needs the "${pkg}" package instead. People often get into this
situation by installing esbuild on Windows or macOS and copying "node_modules"
into a Docker image that runs Linux, or by copying "node_modules" between
Windows and WSL environments.

If you are installing with npm, you can try not copying the "node_modules"
directory when you copy the files over, and running "npm ci" or "npm install"
on the destination platform after the copy. Or you could consider using yarn
instead of npm which has built-in support for installing a package on multiple
platforms simultaneously.

If you are installing with yarn, you can try listing both this platform and the
other platform in your ".yarnrc.yml" file using the "supportedArchitectures"
feature: https://yarnpkg.com/configuration/yarnrc/#supportedArchitectures
Keep in mind that this means multiple copies of esbuild will be present.
`

          // Use a custom message for macOS-specific architecture issues
          if (
            (pkg === packageDarwin_x64 && otherPkg === packageDarwin_arm64) ||
            (pkg === packageDarwin_arm64 && otherPkg === packageDarwin_x64)
          ) {
            suggestions = `
Specifically the "${otherPkg}" package is present but this platform
needs the "${pkg}" package instead. People often get into this
situation by installing esbuild with npm running inside of Rosetta 2 and then
trying to use it with node running outside of Rosetta 2, or vice versa (Rosetta
2 is Apple's on-the-fly x86_64-to-arm64 translation service).

If you are installing with npm, you can try ensuring that both npm and node are
not running under Rosetta 2 and then reinstalling esbuild. This likely involves
changing how you installed npm and/or node. For example, installing node with
the universal installer here should work: https://nodejs.org/en/download/. Or
you could consider using yarn instead of npm which has built-in support for
installing a package on multiple platforms simultaneously.

If you are installing with yarn, you can try listing both "arm64" and "x64"
in your ".yarnrc.yml" file using the "supportedArchitectures" feature:
https://yarnpkg.com/configuration/yarnrc/#supportedArchitectures
Keep in mind that this means multiple copies of esbuild will be present.
`
          }

          throw new Error(`
You installed esbuild for another platform than the one you're currently using.
This won't work because esbuild is written with native code and needs to
install a platform-specific binary executable.
${suggestions}
Another alternative is to use the "esbuild-wasm" package instead, which works
the same way on all platforms. But it comes with a heavy performance cost and
can sometimes be 10x slower than the "esbuild" package, so you may also not
want to do that.
`);
        }

        // If that didn't work too, then maybe someone installed esbuild with
        // both the "--no-optional" and the "--ignore-scripts" flags. The fix
        // for this is to just not do that. We don't attempt to handle this
        // case at all.
        //
        // In that case we try to have a nice error message if we think we know
        // what's happening. Otherwise we just rethrow the original error message.
        throw new Error(`The package "${pkg}" could not be found, and is needed by esbuild.

If you are installing esbuild with npm, make sure that you don't specify the
"--no-optional" or "--omit=optional" flags. The "optionalDependencies" feature
of "package.json" is used by esbuild to install the correct binary executable
for your current platform.`);
      }
      throw e;
    }
  }

  // The esbuild binary executable can't be used in Yarn 2 in PnP mode because
  // it's inside a virtual file system and the OS needs it in the real file
  // system. So we need to copy the file out of the virtual file system into
  // the real file system.
  //
  // You might think that we could use "preferUnplugged: true" in each of the
  // platform-specific packages for this instead, since that tells Yarn to not
  // use the virtual file system for those packages. This is not done because:
  //
  // * Really early versions of Yarn don't support "preferUnplugged", so package
  //   installation would break on those Yarn versions if we did this.
  //
  // * Earlier Yarn versions always installed all optional dependencies for all
  //   platforms even though most of them are incompatible. To minimize file
  //   system space, we want these useless packages to take up as little space
  //   as possible so they should remain unzipped inside their ".zip" files.
  //
  //   We have to explicitly pass "preferUnplugged: false" instead of leaving
  //   it up to Yarn's default behavior because Yarn's heuristics otherwise
  //   automatically unzip packages containing ".exe" files, and we don't want
  //   our Windows-specific packages to be unzipped either.
  //
  let pnpapi: any;
  try {
    pnpapi = require('pnpapi');
  } catch (e) {
  }
  if (pnpapi) {
    const root = pnpapi.getPackageInformation(pnpapi.topLevel).packageLocation;
    const binTargetPath = path.join(
      root,
      'node_modules',
      '.cache',
      'esbuild',
      `pnpapi-${pkg}-${ESBUILD_VERSION}-${path.basename(subpath)}`,
    );
    if (!fs.existsSync(binTargetPath)) {
      fs.mkdirSync(path.dirname(binTargetPath), { recursive: true });
      fs.copyFileSync(binPath, binTargetPath);
      fs.chmodSync(binTargetPath, 0o755);
    }
    return { binPath: binTargetPath, isWASM };
  }

  return { binPath, isWASM };
}
