const fs = require('fs');
const path = require('path');
const rimraf = require('rimraf');
const jsYaml = require('js-yaml');
const { installForTests } = require('./esbuild');
const test262Dir = path.join(__dirname, '..', 'demo', 'test262', 'test');

function findFiles() {
  function visit(dir) {
    for (const entry of fs.readdirSync(dir)) {
      const fullEntry = path.join(dir, entry);
      const stats = fs.statSync(fullEntry);
      if (stats.isDirectory()) {
        visit(fullEntry);
      } else if (stats.isFile() && entry.endsWith('.js') && !entry.endsWith('_FIXTURE.js')) {
        files.push(fullEntry);
      }
    }
  }

  const files = [];
  visit(test262Dir);
  return files;
}

function formatErrors(content, error) {
  if (error.errors) {
    return error.errors.map(({ text, location }) => {
      if (location) {
        const { line, column } = location;
        const contentLine = content.split(/(?:\r\n|\n|\r|\u2028|\u2029)/g)[line - 1];
        return `<stdin>:${line}:${column}: error: ${text}\n${contentLine}\n${' '.repeat(column)}^`;
      }
      return `error: ${text}`;
    }).join('\n');
  }
  return error + '';
}

async function main() {
  const testDir = path.join(__dirname, '.test262');
  const { startService } = installForTests(testDir);
  const service = await startService();
  const files = findFiles();
  let runCount = 0;
  let shouldHavePassed = 0;
  let shouldHaveFailed = 0;
  let reparseCount = 0;
  let reprintCount = 0;
  let minifyCount = 0;

  async function esbuildFile(input, options) {
    try {
      const { js } = await service.transform(input, options);
      return { success: true, output: js };
    } catch (error) {
      return { success: false, error };
    }
  }

  async function processFile(file) {
    const content = fs.readFileSync(file, 'utf8');
    const start = content.indexOf('/*---');
    const end = content.indexOf('---*/');

    if (start < 0 || end < 0) {
      console.warn(`Missing YAML metadata: ${file}`);
      return;
    }

    const yaml = jsYaml.safeLoad(content.slice(start + 5, end));
    const shouldParse = !yaml.negative || yaml.negative.phase === 'runtime';

    if (yaml.features) {
      if (yaml.features.includes('hashbang')) return
      if (yaml.features.includes('regexp-match-indices')) return
      if (yaml.features.includes('regexp-named-groups')) return
      if (yaml.features.includes('regexp-unicode-property-escapes')) return
    }

    const result = await esbuildFile(content, { minify: false });

    if (result.success !== shouldParse) {
      if (!result.success) shouldHavePassed++;
      else shouldHaveFailed++;
      const text = result.success
        ? (yaml.description || '').trim()
        : formatErrors(content, result.error);
      console.log('\n' + `${file}\n${text}`.replace(/\n/g, '\n  '));
    }

    else if (result.success) {
      const result2 = await esbuildFile(result.output, { minify: false });
      if (!result2.success) {
        console.log(`\n!!! REPARSE ERROR: ${file} !!!`);
        console.log(`${result2.error}`);
        reparseCount++;
      } else if (result2.output !== result.output) {
        console.log(`\n!!! REPRINT ERROR: ${file} !!!`);
        reprintCount++;
      } else {
        const result3 = await esbuildFile(result2.output, { minify: true });
        if (!result3.success) {
          throw new Error('This should have succeeded');
        }
        const result4 = await esbuildFile(result3.output, { minify: true });
        if (!result4.success) {
          console.log(`\n!!! MINIFY ERROR: ${file} !!!`);
          console.log(`${result4.error}`);
          minifyCount++;
        }
      }
    }
    runCount++;
  }

  // Process tests in parallel for speed
  await new Promise((resolve, reject) => {
    let inFlight = 0;
    let i = 0;

    function next() {
      if (i === files.length && inFlight === 0) {
        return resolve();
      }

      while (i < files.length && inFlight < 5) {
        inFlight++;
        processFile(files[i++]).then(() => {
          inFlight--;
          next();
        }, reject);
      }
    }

    next();
  });

  console.log(`tests ran: ${runCount}`);
  console.log(`  tests incorrectly failed: ${shouldHavePassed}`);
  console.log(`  tests incorrectly passed: ${shouldHaveFailed}`);
  console.log(`tests skipped: ${files.length - runCount}`);
  console.log(`reparse failures: ${reparseCount}`);
  console.log(`reprint failures: ${reprintCount}`);
  console.log(`minify failures: ${minifyCount}`);

  // Clean up after all tests finish
  rimraf.sync(testDir, { disableGlob: true });
  service.stop();
}

main().catch(e => setTimeout(() => {
  throw e
}));
