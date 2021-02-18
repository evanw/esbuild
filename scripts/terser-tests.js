const { installForTests } = require('./esbuild');
const childProcess = require('child_process');
const assert = require('assert');
const path = require('path');
const fs = require('fs');

const repoDir = path.dirname(__dirname);
const testDir = path.join(repoDir, 'scripts', '.terser-tests');
const terserDir = path.join(repoDir, 'demo', 'terser');
let U;

main().catch(e => setTimeout(() => { throw e }));

async function main() {
  // Terser's stdout comparisons fail if this is true since stdout contains
  // terminal color escape codes
  process.stdout.isTTY = false;

  // Make sure the tests are installed
  console.log('Downloading terser...');
  childProcess.execSync('make demo/terser', { cwd: repoDir, stdio: 'pipe' });
  U = require(terserDir);

  // Create a fresh test directory
  childProcess.execSync(`rm -fr "${testDir}"`);
  fs.mkdirSync(testDir)

  // Start the esbuild service
  const esbuild = installForTests();
  const service = await esbuild.startService();

  // Find test files
  const compressDir = path.join(terserDir, 'test', 'compress');
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

// Modified from "terser/demo/test/compress.js"
async function test_case(service, test) {
  const sandbox = require(path.join(terserDir, 'test', 'sandbox'));
  const log = (format, args) => { throw new Error(tmpl(format, args)); };

  var semver = require(path.join(terserDir, 'node_modules', 'semver'));
  var output_options = test.beautify || {};

  // Generate the input code
  if (test.input instanceof U.AST_SimpleStatement
    && test.input.body instanceof U.AST_TemplateString) {
    try {
      var input = U.parse(test.input.body.segments[0].value);
    } catch (ex) {
      return false;
    }
    var input_code = make_code(input, output_options);
    var input_formatted = test.input.body.segments[0].value;
  } else {
    var input = as_toplevel(test.input, test.mangle);
    var input_code = make_code(input, output_options);
    var input_formatted = make_code(test.input, {
      ecma: 2015,
      beautify: true,
      quote_style: 3,
      keep_quoted_props: true
    });
  }

  // Make sure it's valid
  try {
    U.parse(input_code);
  } catch (ex) {
    log("!!! Cannot parse input\n---INPUT---\n{input}\n--PARSE ERROR--\n{error}\n\n", {
      input: input_formatted,
      error: ex,
    });
    return false;
  }

  // Pretty-print it
  var ast = input.to_mozilla_ast();
  var mozilla_options = {
    ecma: output_options.ecma,
    ascii_only: output_options.ascii_only,
    comments: false,
  };
  var ast_as_string = U.AST_Node.from_mozilla_ast(ast).print_to_string(mozilla_options);

  // Run esbuild as a minifier
  try {
    var { code: output } = await service.transform(ast_as_string, {
      minify: true,
      keepNames: test.options.keep_fnames,
    });
  } catch (e) {
    const formatError = ({ text, location }) => {
      if (!location) return `\nerror: ${text}`;
      const { file, line, column } = location;
      return `\n${file}:${line}:${column}: error: ${text}`;
    }
    log("!!! esbuild failed\n---INPUT---\n{input}\n---ERROR---\n{error}\n", {
      input: ast_as_string,
      error: (e && e.message || e) + '' + (e.errors ? e.errors.map(formatError) : ''),
    });
    return false;
  }

  // Make sure esbuild generates valid JavaScript
  try {
    U.parse(output);
  } catch (ex) {
    log("!!! Test matched expected result but cannot parse output\n---INPUT---\n{input}\n---OUTPUT---\n{output}\n--REPARSE ERROR--\n{error}\n\n", {
      input: input_formatted,
      output: output,
      error: ex.stack,
    });
    return false;
  }

  // Verify that the stdout matches our expectations
  if (test.expect_stdout
    && (!test.node_version || semver.satisfies(process.version, test.node_version))
    && !process.env.TEST_NO_SANDBOX
  ) {
    if (test.expect_stdout === true) {
      test.expect_stdout = sandbox.run_code(input_code, test.prepend_code);
    }
    var stdout = sandbox.run_code(output, test.prepend_code);
    if (!sandbox.same_stdout(test.expect_stdout, stdout)) {
      log("!!! failed\n---INPUT---\n{input}\n---OUTPUT---\n{output}\n---EXPECTED {expected_type}---\n{expected}\n---ACTUAL {actual_type}---\n{actual}\n\n", {
        input: input_formatted,
        output: output,
        expected_type: typeof test.expect_stdout == "string" ? "STDOUT" : "ERROR",
        expected: test.expect_stdout,
        actual_type: typeof stdout == "string" ? "STDOUT" : "ERROR",
        actual: stdout,
      });
      return false;
    }
  }
  return true;
}

////////////////////////////////////////////////////////////////////////////////
// The code below was copied verbatim from "terser/demo/test/compress.js"
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

function tmpl() {
  return U.string_template.apply(this, arguments);
}

function as_toplevel(input, mangle_options) {
  if (!(input instanceof U.AST_BlockStatement))
    throw new Error("Unsupported input syntax");
  for (var i = 0; i < input.body.length; i++) {
    var stat = input.body[i];
    if (stat instanceof U.AST_SimpleStatement && stat.body instanceof U.AST_String)
      input.body[i] = new U.AST_Directive(stat.body);
    else break;
  }
  var toplevel = new U.AST_Toplevel(input);
  toplevel.figure_out_scope(mangle_options);
  return toplevel;
}

function parse_test(file) {
  var script = fs.readFileSync(file, "utf8");
  // TODO try/catch can be removed after fixing https://github.com/mishoo/UglifyJS2/issues/348
  try {
    var ast = U.parse(script, {
      filename: file
    });
  } catch (e) {
    console.log("Caught error while parsing tests in " + file + "\n");
    console.log(e);
    throw e;
  }
  var tests = {};
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

  function read_boolean(stat) {
    if (stat.TYPE == "SimpleStatement") {
      var body = stat.body;
      if (body instanceof U.AST_Boolean) {
        return body.value;
      }
    }
    throw new Error("Should be boolean");
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
    var test = {
      name: name,
      options: {},
      reminify: true,
    };
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
        assert.ok(
          [
            "input",
            "prepend_code",
            "expect",
            "expect_error",
            "expect_exact",
            "expect_warnings",
            "expect_stdout",
            "node_version",
            "reminify",
          ].includes(label.name),
          tmpl("Unsupported label {name} [{line},{col}]", {
            name: label.name,
            line: label.start.line,
            col: label.start.col
          })
        );
        var stat = node.body;
        if (label.name == "expect_exact" || label.name == "node_version") {
          test[label.name] = read_string(stat);
        } else if (label.name == "reminify") {
          var value = read_boolean(stat);
          test.reminify = value == null || value;
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
        } else if (label.name === "prepend_code") {
          test[label.name] = read_string(stat);
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

function make_code(ast, options) {
  var stream = U.OutputStream(options);
  ast.print(stream);
  return stream.get();
}

function evaluate(code) {
  if (code instanceof U.AST_Node)
    code = make_code(code, { beautify: true });
  return new Function("return(" + code + ")")();
}
