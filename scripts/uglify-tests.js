const { installForTests } = require('./esbuild');
const childProcess = require('child_process');
const assert = require('assert');
const path = require('path');
const fs = require('fs');

const repoDir = path.dirname(__dirname);
const testDir = path.join(repoDir, 'scripts', '.uglify-tests');
const uglifyDir = path.join(repoDir, 'demo', 'uglify');
let U;

main().catch(e => setTimeout(() => { throw e }));

async function main() {
  // // Terser's stdout comparisons fail if this is true since stdout contains
  // // terminal color escape codes
  // process.stdout.isTTY = false;

  // Make sure the tests are installed
  console.log('Downloading uglify...');
  childProcess.execSync('make demo/uglify', { cwd: repoDir, stdio: 'pipe' });
  U = require(path.join(uglifyDir, 'test', 'node'));

  // Start the esbuild service
  const esbuild = installForTests(testDir);
  const service = await esbuild.startService();

  // Find test files
  const compressDir = path.join(uglifyDir, 'test', 'compress');
  const files = fs.readdirSync(compressDir).filter(name => name.endsWith('.js'));

  // Run all tests concurrently
  let passedTotal = 0;
  let failedTotal = 0;
  const runTest = file => test_file(service, path.join(compressDir, file))
    .then(({ passed, failed }) => {
      passedTotal += passed;
      failedTotal += failed;
    });
  await Promise.all(files.map(runTest));

  // Clean up test output
  service.stop();
  childProcess.execSync(`rm -fr "${testDir}"`);

  console.log(`${failedTotal} failed out of ${passedTotal + failedTotal}`);
  if (failedTotal) {
    process.exit(1);
  }
}

async function test_file(service, file) {
  let passed = 0;
  let failed = 0;
  const tests = parse_test(file);
  const runTest = name => test_case(service, tests[name])
    .then(() => passed++)
    .catch(e => {
      failed++;
      console.error(`❌ ${file}: ${name}: ${(e && e.message || e).trim()}\n`);
      pass = false;
    });
  await Promise.all(Object.keys(tests).map(runTest));
  return { passed, failed };
}

// Modified from "uglify/demo/test/compress.js"
async function test_case(service, test) {
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

  // Run esbuild as a minifier
  try {
    var { js: output } = await service.transform(input_code, {
      minify: true,
      target: 'es5',
    });
  } catch (e) {
    const formatError = ({ text, location }) => {
      if (!location) return `\nerror: ${text}`;
      const { file, line, column } = location;
      return `\n${file}:${line}:${column}: error: ${text}`;
    }
    log("!!! esbuild failed\n---INPUT---\n{input}\n---ERROR---\n{error}\n", {
      input: input_code,
      error: (e && e.message || e) + '' + (e.errors ? e.errors.map(formatError) : ''),
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
      error: ex,
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
    if (!sandbox.same_stdout(test.expect_stdout, actual)) {
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
