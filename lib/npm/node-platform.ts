import fs = require('fs');
import os = require('os');
import path = require('path');

declare const ESBUILD_VERSION: string;

// This feature was added to give external code a way to modify the binary
// path without modifying the code itself. Do not remove this because
// external code relies on this.
export var ESBUILD_BINARY_PATH: string | undefined = process.env.ESBUILD_BINARY_PATH || ESBUILD_BINARY_PATH;

const packageDarwin_arm64 = '@esbuild/darwin-arm64'
const packageDarwin_x64 = '@esbuild/darwin-x64'

export const knownWindowsPackages: Record<string, string> = {
  'win32 arm64 LE': '@esbuild/win32-arm64',
  'win32 ia32 LE': '@esbuild/win32-ia32',
  'win32 x64 LE': '@esbuild/win32-x64',
};

export const knownUnixlikePackages: Record<string, string> = {
  'android arm64 LE': '@esbuild/android-arm64',
  'darwin arm64 LE': '@esbuild/darwin-arm64',
  'darwin x64 LE': '@esbuild/darwin-x64',
  'freebsd arm64 LE': '@esbuild/freebsd-arm64',
  'freebsd x64 LE': '@esbuild/freebsd-x64',
  'linux arm LE': '@esbuild/linux-arm',
  'linux arm64 LE': '@esbuild/linux-arm64',
  'linux ia32 LE': '@esbuild/linux-ia32',
  'linux mips64el LE': '@esbuild/linux-mips64el',
  'linux ppc64 LE': '@esbuild/linux-ppc64',
  'linux riscv64 LE': '@esbuild/linux-riscv64',
  'linux s390x BE': '@esbuild/linux-s390x',
  'linux x64 LE': '@esbuild/linux-x64',
  'linux loong64 LE': '@esbuild/linux-loong64',
  'netbsd x64 LE': '@esbuild/netbsd-x64',
  'openbsd x64 LE': '@esbuild/openbsd-x64',
  'sunos x64 LE': '@esbuild/sunos-x64',
};

export const knownWebAssemblyFallbackPackages: Record<string, string> = {
  'android arm LE': '@esbuild/android-arm',
  'android x64 LE': '@esbuild/android-x64',
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

  // This code below guards against the unlikely case that the user is using
  // Yarn 2+ in PnP mode and that version is old enough that it doesn't support
  // the "preferUnplugged" setting. If that's the case, then the path to the
  // binary executable that we got above isn't actually a real path. Instead
  // it's a path to a zip file with some extra stuff appended to it.
  //
  // Yarn's PnP mode tries hard to patch Node's file system APIs to pretend
  // that these fake paths are real. So we can't check whether it's a real file
  // or not by using Node's file system APIs (e.g. "fs.existsSync") because
  // they have been patched to lie. But we can't return this fake path because
  // Yarn hasn't patched "child_process.execFileSync" to work with fake paths,
  // so attempting to execute the binary will fail.
  //
  // As a hack, we use Node's file system APIs to copy the file from the fake
  // path to a real path. This will cause Yarn's hacked file system to extract
  // the binary executable from the zip file and turn it into a real file that
  // we can execute.
  //
  // This is only done when both ".zip/" is present in the path and the
  // "pnpapi" package is present, which is a strong indication that Yarn PnP is
  // being used. There is no API at all for telling whether something is a real
  // file or not as far as I can tell. Even Yarn's own code just checks for
  // whether ".zip/" is present in the path or not.
  //
  // Note to self: One very hacky way to tell if a path is under the influence
  // of Yarn's file system hacks is to stat the file and check for the "crc"
  // property in the result. If that's present, then the file is inside a zip
  // file. However, I haven't done that here because it's not intended to be
  // used that way. Who knows what Yarn versions it does or does not work on
  // (including future versions).
  if (/\.zip\//.test(binPath)) {
    let pnpapi: any;
    try {
      pnpapi = require('pnpapi');
    } catch (e) {
    }
    if (pnpapi) {
      // Copy the executable to ".cache/esbuild". The official recommendation
      // of the Yarn team is to use the directory "node_modules/.cache/esbuild":
      // https://yarnpkg.com/advanced/rulebook/#packages-should-never-write-inside-their-own-folder-outside-of-postinstall
      // People that use Yarn in PnP mode find this really annoying because they
      // don't like seeing the "node_modules" directory. These people should
      // either work with Yarn to change their recommendation, or upgrade their
      // version of Yarn, since newer versions of Yarn shouldn't stick esbuild's
      // binary executables in a zip file due to the "preferUnplugged" setting.
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
  }

  return { binPath, isWASM };
}
