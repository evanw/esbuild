const { installForTests } = require('./esbuild');
const childProcess = require('child_process');
const assert = require('assert');
const path = require('path');
const fs = require('fs');

const repoDir = path.dirname(__dirname);
const testDir = path.join(repoDir, 'scripts', '.uglify-tests');
const uglifyDir = path.join(repoDir, 'demo', 'uglify');
const SKIP = {};
let U;

main().catch(e => setTimeout(() => { throw e }));

async function main() {
  // // Terser's stdout comparisons fail if this is true since stdout contains
  // // terminal color escape codes
  // process.stdout.isTTY = false;

  // Make sure the tests are installed
  childProcess.execSync('make demo/uglify', { cwd: repoDir, stdio: 'pipe' });
  U = require(path.join(uglifyDir, 'test', 'node'));

  // Create a fresh test directory
  childProcess.execSync(`rm -fr "${testDir}"`);
  fs.mkdirSync(testDir)

  // Start the esbuild service
  const esbuild = installForTests();

  // Find test files
  const compressDir = path.join(uglifyDir, 'test', 'compress');
  const files = fs.readdirSync(compressDir).filter(name => name.endsWith('.js'));

  // Run all tests concurrently
  let passedTotal = 0;
  let failedTotal = 0;
  let skippedTotal = 0;
  const runTest = file => test_file(esbuild, path.join(compressDir, file))
    .then(({ passed, failed, skipped }) => {
      passedTotal += passed;
      failedTotal += failed;
      skippedTotal += skipped;
    });
  await Promise.all(files.map(runTest));

  // Clean up test output
  childProcess.execSync(`rm -fr "${testDir}"`);

  console.log(`${failedTotal} failed out of ${passedTotal + failedTotal}, with ${skippedTotal} skipped`);
  if (failedTotal) {
    process.exit(1);
  }
}

async function test_file(esbuild, file) {
  let passed = 0;
  let failed = 0;
  let skipped = 0;
  const tests = parse_test(file);
  const runTest = name => test_case(esbuild, tests[name], path.basename(file))
    .then(x => {
      if (x === SKIP) {
        skipped++;
      } else {
        passed++;
      }
    })
    .catch(e => {
      failed++;
      console.error(`❌ ${file}: ${name}: ${(e && e.message || e).trim()}\n`);
      pass = false;
    });
  await Promise.all(Object.keys(tests).map(runTest));
  return { passed, failed, skipped };
}

// Modified from "uglify/demo/test/compress.js"
async function test_case(esbuild, test, basename) {
  const sandbox = require(path.join(uglifyDir, 'test', 'sandbox'));
  const log = (format, args) => { throw new Error(tmpl(format, args)); };

  var semver = require(path.join(uglifyDir, 'node_modules', 'semver'));

  // Generate the input code
  var input = to_toplevel(test.input, test.mangle);
  var input_code = make_code(input);
  var input_formatted = make_code(test.input, {
    beautify: true,
    comments: "all",
    keep_quoted_props: true,
    quote_style: 3,
  });

  // Make sure it's valid
  try {
    U.parse(input_code);
  } catch (ex) {
    log([
      "!!! Cannot parse input",
      "---INPUT---",
      "{input}",
      "--PARSE ERROR--",
      "{error}",
      "",
      "",
    ].join("\n"), {
      input: input_formatted,
      error: ex,
    });
  }

  // Ignore tests that no longer pass in modern versions of node. These tests
  // contain code that is now considered a syntax error. The relevant code is
  // this:
  //
  //   try{throw 42}catch(a){console.log(a);function a(){}}
  //
  if (test.node_version && !semver.satisfies(process.version, test.node_version)) {
    console.error("*** skipping test %j with node_version %j", test.name, test.node_version);
    return SKIP;
  }

  // Run esbuild as a minifier
  try {
    var { code: output } = await esbuild.transform(input_code, {
      minify: true,
      target: 'esnext',
    });
  } catch (e) {
    // These tests fail because they contain syntax errors. These test failures
    // do not indicate anything wrong with esbuild so the failures are ignored.
    // Here is one of the tests:
    //
    //   try{}catch(a){const a="aa"}
    //
    if ([
      'const.js: issue_4290_1',
      'const.js: issue_4305_2',
      'const.js: retain_catch',
      'const.js: skip_braces',
      'exports.js: defaults',
      'exports.js: drop_unused',
      'exports.js: hoist_exports_1',
      'exports.js: hoist_exports_2',
      'exports.js: keep_return_values',
      'exports.js: mangle_rename',
      'exports.js: mangle',
      'exports.js: refs',
      'imports.js: issue_4708_1',
      'imports.js: issue_4708_2',
      'let.js: issue_4290_1',
      'let.js: issue_4305_2',
      'let.js: retain_catch',
      'let.js: skip_braces',
      'reduce_vars.js: defun_catch_4',
      'reduce_vars.js: defun_catch_5',
      'templates.js: malformed_evaluate_1',
      'templates.js: malformed_evaluate_2',
      'templates.js: malformed_evaluate_3',
      'varify.js: issue_4290_1_const',
      'varify.js: issue_4290_1_let',
    ].indexOf(`${basename}: ${test.name}`) >= 0) {
      console.error(`*** skipping test with known syntax error: ${basename}: ${test.name}`);
      return SKIP;
    }

    // These tests fail because esbuild supports top-level await. Technically
    // top-level await is only allowed inside a module, and can be used as a
    // normal identifier in a script. But the script/module distinction causes
    // a lot of pain due to the need to configure every single tool to say
    // whether to parse the code as a script or a module, so esbuild mostly
    // does away with the distinction and enables top-level await everywhere.
    // This means it fails these tests but the failures are unlikely to matter
    // in real-world code, so they can be ignored. Here's one test case:
    //
    //   async function await(){console.log("PASS")}await();
    //
    if ([
      'awaits.js: defun_name',
      'awaits.js: drop_fname',
      'awaits.js: functions_anonymous',
      'awaits.js: functions_inner_var',
      'awaits.js: issue_4335_1',
      'awaits.js: keep_fname',
      'classes.js: await',
    ].indexOf(`${basename}: ${test.name}`) >= 0) {
      console.error(`*** skipping test with top-level await as identifier: ${basename}: ${test.name}`);
      return SKIP;
    }

    // These tests fail because esbuild makes assigning to an inlined constant
    // a compile error to avoid code with incorrect behavior. This is a limitation
    // due to esbuild's three-pass design but it shouldn't matter in practice. It
    // just means esbuild rejects bad code at compile time instead of at run time.
    if ([
      'const.js: issue_4212_1',
      'const.js: issue_4212_2',
    ].indexOf(`${basename}: ${test.name}`) >= 0) {
      console.error(`*** skipping test with assignment to an inlined constant: ${basename}: ${test.name}`);
      return SKIP;
    }

    log("!!! esbuild failed\n---INPUT---\n{input}\n---ERROR---\n{error}\n", {
      input: input_code,
      error: e && e.message || e,
    });
  }

  // Make sure esbuild generates valid JavaScript
  try {
    U.parse(output);
  } catch (ex) {
    log([
      "!!! Test matched expected result but cannot parse output",
      "---INPUT---",
      "{input}",
      "---OUTPUT---",
      "{output}",
      "--REPARSE ERROR--",
      "{error}",
      "",
      "",
    ].join("\n"), {
      input: input_formatted,
      output: output,
      error: ex && ex.stack || ex,
    });
  }

  // Verify that the stdout matches our expectations
  if (test.expect_stdout && (!test.node_version || semver.satisfies(process.version, test.node_version))) {
    var stdout = [run_code(input_code), run_code(input_code, true)];
    var toplevel = sandbox.has_toplevel({
      compress: test.options,
      mangle: test.mangle
    });
    var actual = stdout[toplevel ? 1 : 0];
    if (test.expect_stdout === true) {
      test.expect_stdout = actual;
    }
    actual = run_code(output, toplevel);

    // Ignore the known failures in CI, but not otherwise
    const isExpectingFailure = !process.env.CI ? false : [
      // Stdout difference
      'classes.js: issue_5015_2',
      'const.js: issue_4225',
      'const.js: issue_4229',
      'const.js: issue_4245',
      'const.js: use_before_init_3',
      'default-values.js: retain_empty_iife',
      'destructured.js: funarg_side_effects_2',
      'destructured.js: funarg_side_effects_3',
      'let.js: issue_4225',
      'let.js: issue_4229',
      'let.js: issue_4245',
      'let.js: use_before_init_3',

      // Error difference
      'dead-code.js: dead_code_2_should_warn',
    ].indexOf(`${basename}: ${test.name}`) >= 0

    if (!sandbox.same_stdout(test.expect_stdout, actual)) {
      if (isExpectingFailure) {
        console.error(`*** skipping test with known esbuild failure: ${basename}: ${test.name}`);
        return SKIP;
      }

      log([
        "!!! failed",
        "---INPUT---",
        "{input}",
        "---EXPECTED {expected_type}---",
        "{expected}",
        "---ACTUAL {actual_type}---",
        "{actual}",
        "",
        "",
      ].join("\n"), {
        input: input_formatted,
        expected_type: typeof test.expect_stdout == "string" ? "STDOUT" : "ERROR",
        expected: test.expect_stdout,
        actual_type: typeof actual == "string" ? "STDOUT" : "ERROR",
        actual: actual,
      });
    } else if (isExpectingFailure) {
      throw new Error(`UPDATE NEEDED: expected failure for ${basename}: ${test.name}, please remove this test from known failure list`);
    }
  }
}

////////////////////////////////////////////////////////////////////////////////
// The code below was copied verbatim from "uglify/demo/test/compress.js"
//
// UglifyJS is released under the BSD license:
//
// Copyright 2012-2019 (c) Mihai Bazon <mihai.bazon@gmail.com>
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions
// are met:
//
//     * Redistributions of source code must retain the above
//       copyright notice, this list of conditions and the following
//       disclaimer.
//
//     * Redistributions in binary form must reproduce the above
//       copyright notice, this list of conditions and the following
//       disclaimer in the documentation and/or other materials
//       provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDER “AS IS” AND ANY
// EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
// IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR
// PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER BE
// LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY,
// OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO,
// PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR
// PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR
// TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF
// THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF
// SUCH DAMAGE.

function evaluate(code) {
  if (code instanceof U.AST_Node) code = make_code(code, { beautify: true });
  return new Function("return(" + code + ")")();
}

function make_code(ast, options) {
  var stream = U.OutputStream(options);
  ast.print(stream);
  return stream.get();
}

function parse_test(file) {
  var script = fs.readFileSync(file, "utf8");
  // TODO try/catch can be removed after fixing https://github.com/mishoo/UglifyJS/issues/348
  try {
    var ast = U.parse(script, {
      filename: file
    });
  } catch (e) {
    console.error("Caught error while parsing tests in " + file);
    console.error(e);
    process.exit(1);
  }
  var tests = Object.create(null);
  var tw = new U.TreeWalker(function (node, descend) {
    if (node instanceof U.AST_LabeledStatement
      && tw.parent() instanceof U.AST_Toplevel) {
      var name = node.label.name;
      if (name in tests) {
        throw new Error('Duplicated test name "' + name + '" in ' + file);
      }
      tests[name] = get_one_test(name, node.body);
      return true;
    }
    if (!(node instanceof U.AST_Toplevel)) croak(node);
  });
  ast.walk(tw);
  return tests;

  function croak(node) {
    throw new Error(tmpl("Can't understand test file {file} [{line},{col}]\n{code}", {
      file: file,
      line: node.start.line,
      col: node.start.col,
      code: make_code(node, { beautify: false })
    }));
  }

  function read_string(stat) {
    if (stat.TYPE == "SimpleStatement") {
      var body = stat.body;
      switch (body.TYPE) {
        case "String":
          return body.value;
        case "Array":
          return body.elements.map(function (element) {
            if (element.TYPE !== "String")
              throw new Error("Should be array of strings");
            return element.value;
          }).join("\n");
      }
    }
    throw new Error("Should be string or array of strings");
  }

  function get_one_test(name, block) {
    var test = { name: name, options: {} };
    var tw = new U.TreeWalker(function (node, descend) {
      if (node instanceof U.AST_Assign) {
        if (!(node.left instanceof U.AST_SymbolRef)) {
          croak(node);
        }
        var name = node.left.name;
        test[name] = evaluate(node.right);
        return true;
      }
      if (node instanceof U.AST_LabeledStatement) {
        var label = node.label;
        assert.ok([
          "input",
          "expect",
          "expect_exact",
          "expect_warnings",
          "expect_stdout",
          "node_version",
        ].indexOf(label.name) >= 0, tmpl("Unsupported label {name} [{line},{col}]", {
          name: label.name,
          line: label.start.line,
          col: label.start.col
        }));
        var stat = node.body;
        if (label.name == "expect_exact" || label.name == "node_version") {
          test[label.name] = read_string(stat);
        } else if (label.name == "expect_stdout") {
          var body = stat.body;
          if (body instanceof U.AST_Boolean) {
            test[label.name] = body.value;
          } else if (body instanceof U.AST_Call) {
            var ctor = global[body.expression.name];
            assert.ok(ctor === Error || ctor.prototype instanceof Error, tmpl("Unsupported expect_stdout format [{line},{col}]", {
              line: label.start.line,
              col: label.start.col
            }));
            test[label.name] = ctor.apply(null, body.args.map(function (node) {
              assert.ok(node instanceof U.AST_Constant, tmpl("Unsupported expect_stdout format [{line},{col}]", {
                line: label.start.line,
                col: label.start.col
              }));
              return node.value;
            }));
          } else {
            test[label.name] = read_string(stat) + "\n";
          }
        } else {
          test[label.name] = stat;
        }
        return true;
      }
    });
    block.walk(tw);
    return test;
  }
}

function run_code(code, toplevel) {
  const sandbox = require(path.join(uglifyDir, 'test', 'sandbox'));

  var result = sandbox.run_code(code, toplevel);
  return typeof result == "string" ? result.replace(/\u001b\[\d+m/g, "") : result;
}

function tmpl() {
  return U.string_template.apply(null, arguments);
}

function to_toplevel(input, mangle_options) {
  if (!(input instanceof U.AST_BlockStatement)) throw new Error("Unsupported input syntax");
  var directive = true;
  var offset = input.start.line;
  var tokens = [];
  var toplevel = new U.AST_Toplevel(input.transform(new U.TreeTransformer(function (node) {
    if (U.push_uniq(tokens, node.start)) node.start.line -= offset;
    if (!directive || node === input) return;
    if (node instanceof U.AST_SimpleStatement && node.body instanceof U.AST_String) {
      return new U.AST_Directive(node.body);
    } else {
      directive = false;
    }
  })));
  toplevel.figure_out_scope(mangle_options);
  return toplevel;
}
