import { pkgAndBinForCurrentPlatform } from './node-platform';

import fs = require('fs');
import os = require('os');
import path = require('path');
import child_process = require('child_process');

declare const ESBUILD_VERSION: string;
const toPath = path.join(__dirname, 'bin', 'esbuild');

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

// This feature was added to give external code a way to modify the binary
// path without modifying the code itself. Do not remove this because
// external code relies on this (in addition to esbuild's own test suite).
if (process.env.ESBUILD_BINARY_PATH) {
  // Patch the CLI use case (the "esbuild" command)
  const pathString = JSON.stringify(process.env.ESBUILD_BINARY_PATH);
  fs.writeFileSync(toPath, `#!/usr/bin/env node\n` +
    `require('child_process').execFileSync(${pathString}, process.argv.slice(2), { stdio: 'inherit' });\n`);

  // Patch the JS API use case (the "require('esbuild')" workflow)
  const libMain = path.join(__dirname, 'lib', 'main.js');
  const code = fs.readFileSync(libMain, 'utf8');
  fs.writeFileSync(libMain, `var ESBUILD_BINARY_PATH = ${pathString};\n${code}`);

  // Windows needs "node" before this command since it's a JavaScript file
  validateBinaryVersion('node', toPath);
}

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
else if (os.platform() !== 'win32' && !isYarn2OrAbove()) {
  const { bin } = pkgAndBinForCurrentPlatform();
  try {
    fs.unlinkSync(toPath);
    fs.linkSync(bin, toPath);
  } catch (e) {
    // Ignore errors here since this optimization is optional
  }

  // This is no longer a JavaScript file so don't run it using "node"
  validateBinaryVersion(toPath);
}

else {
  // Windows needs "node" before this command since it's a JavaScript file
  validateBinaryVersion('node', toPath);
}
