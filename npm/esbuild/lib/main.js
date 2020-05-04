const child_process = require('child_process');
const path = require('path');
const os = require('os');

exports.build = options => {
  return new Promise((resolve, reject) => {
    let binPath;

    if ((process.platform === 'linux' || process.platform === 'darwin') && os.arch() === 'x64') {
      binPath = path.join(__dirname, '..', 'bin', 'esbuild');
    } else if (process.platform === 'win32' && os.arch() === 'x64') {
      binPath = path.join(__dirname, '..', '.install', 'node_modules', 'esbuild-windows-64', 'esbuild.exe');
    } else {
      throw new Error(`Unsupported platform: ${process.platform} ${os.arch()}`);
    }

    const flags = [`--error-limit=${options.errorLimit || 0}`];
    const stdio = options.stdio;

    if (options.name) flags.push(`--name=${options.name}`);
    if (options.bundle) flags.push('--bundle');
    if (options.outfile) flags.push(`--outfile=${options.outfile}`);
    if (options.outdir) flags.push(`--outdir=${options.outdir}`);
    if (options.sourcemap) flags.push('--sourcemap');
    if (options.target) flags.push(`--target=${options.target}`);
    if (options.platform) flags.push(`--platform=${options.platform}`);
    if (options.format) flags.push(`--format=${options.format}`);
    if (options.color) flags.push(`--color=${options.color}`);
    if (options.external) for (const name of options.external) flags.push(`--external:${name}`);

    if (options.minify) flags.push('--minify');
    if (options.minifySyntax) flags.push('--minify-syntax');
    if (options.minifyWhitespace) flags.push('--minify-whitespace');
    if (options.minifyIdentifiers) flags.push('--minify-identifiers');

    if (options.jsxFactory) flags.push(`--jsx-factory=${options.jsxFactory}`);
    if (options.jsxFragment) flags.push(`--jsx-fragment=${options.jsxFragment}`);
    if (options.define) for (const key in options.define) flags.push(`--define:${key}=${options.define[key]}`);
    if (options.loader) for (const ext in options.loader) flags.push(`--loader:${ext}=${options.loader[ext]}`);

    for (const entryPoint of options.entryPoints) {
      if (entryPoint.startsWith('-')) throw new Error(`Invalid entry point: ${entryPoint}`);
      flags.push(entryPoint);
    }

    const child = child_process.spawn(binPath, flags, { cwd: process.cwd(), windowsHide: true, stdio });
    child.on('error', error => reject(error));

    // The stderr pipe won't be available if "stdio" is set to "inherit"
    const stderrChunks = [];
    if (child.stderr) child.stderr.on('data', chunk => stderrChunks.push(chunk));

    child.on('close', code => {
      const fullRegex = /^(.+):(\d+):(\d+): (warning|error): (.+)$/;
      const smallRegex = /^(warning|error): (.+)$/;
      const errors = [];
      const warnings = [];
      const stderr = Buffer.concat(stderrChunks).toString();

      for (const line of stderr.split('\n')) {
        let match = fullRegex.exec(line);
        if (match) {
          const [, file, line, column, kind, text] = match;
          (kind === 'error' ? errors : warnings).push({ text, location: { file, line: +line, column: +column } });
        }

        else {
          match = smallRegex.exec(line);
          if (match) {
            const [, kind, text] = match;
            (kind === 'error' ? errors : warnings).push({ text, location: null });
          }
        }
      }

      if (errors.length === 0 && code === 0) {
        resolve({ stderr, warnings });
      }

      else {
        // The error array will be empty if "stdio" is set to "inherit"
        const summary = errors.length < 1 ? '' : ` with ${errors.length} error${errors.length < 2 ? '' : 's'}`;
        const error = new Error(`Build failed${summary}`);
        error.stderr = stderr;
        error.errors = errors;
        error.warnings = warnings;
        reject(error);
      }
    });
  });
}
