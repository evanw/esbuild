// This is a simple fuzzer to detect crashes. It could certainly be improved
// to expand coverage.

const fs = require('fs');
const os = require('os');
const path = require('path');
const escodegen = require('escodegen');
const estreeWalker = require('estree-walker');
const child_process = require('child_process');

function fuzz() {
  const tempDir = os.tmpdir();
  const inPath = path.join(tempDir, 'esbuild.fuzz.in.js');
  const outPath = path.join(tempDir, 'esbuild.fuzz.out.js');
  const esbuildPath = path.join(path.dirname(__dirname), 'esbuild');
  child_process.execSync('make', { cwd: path.join(__dirname, '..') });

  while (true) {
    const ast = randomJavaScriptAST();
    tryRun(ast, esbuildPath, inPath, ['--outfile=' + outPath]);
    tryRun(ast, esbuildPath, inPath, ['--outfile=' + outPath, '--minify']);
  }
}

function tryRun(ast, esbuildPath, inPath, args) {
  args = [inPath].concat(args);

  // See if the build crashes
  fs.writeFileSync(inPath, escodegen.generate(ast));
  try {
    child_process.execFileSync(esbuildPath, args);
    return;
  } catch (e) {
  }

  console.log('Found crash:');
  console.log('-'.repeat(80));
  console.log(escodegen.generate(ast));
  let ignoreCount = 0;

  while (true) {
    const clonedAST = JSON.parse(JSON.stringify(ast));
    if (!tryToSimplifySomething(clonedAST, ignoreCount)) break;

    // See if the build crashes
    let didCrash = false;
    const clonedCode = escodegen.generate(clonedAST);
    fs.writeFileSync(inPath, clonedCode);
    try {
      child_process.execFileSync(esbuildPath, args);
    } catch (e) {
      didCrash = true;
    }

    // Either keep or discard this change
    if (didCrash) {
      ast = clonedAST;
      console.log('Minimized to ' + clonedCode.length);
      console.log('-'.repeat(80));
      console.log(escodegen.generate(ast));
    } else {
      ignoreCount++;
      console.log('Ignore count is ' + ignoreCount);
    }
  }

  console.log('Minimized test case:');
  console.log('-'.repeat(80));
  console.log(escodegen.generate(ast));
  process.exit();
}

function tryToSimplifySomething(ast, ignoreCount) {
  function tryToRemoveNode(walker) {
    if (ignoreCount > 0) {
      ignoreCount--;
    } else {
      walker.remove();
      didSimplifySomething = true;
    }
  }

  function tryToReplaceNode(walker, replacement) {
    if (ignoreCount > 0) {
      ignoreCount--;
    } else {
      walker.replace(replacement);
      didSimplifySomething = true;
    }
  }

  let didSimplifySomething = false;
  estreeWalker.walk(ast, {
    enter(node, parent) {
      if (didSimplifySomething) {
        this.skip();
        return;
      }

      // Try to remove statements
      if (parent && (parent.type === 'Program' || parent.type === 'BlockStatement')) {
        tryToRemoveNode(this);
        if (didSimplifySomething) {
          return;
        }
      }

      // Statements
      switch (node.type) {
        case 'ThrowStatement':
          tryToReplaceNode(this, { type: 'ExpressionStatement', expression: node.argument });
          return;

        case 'ReturnStatement':
          if (node.argument) {
            tryToReplaceNode(this, { type: 'ExpressionStatement', expression: node.argument });
          }
          return;

        case 'FunctionDeclaration':
          tryToReplaceNode(this, node.body);
          return;
      }

      // Expressions
      switch (node.type) {
        case 'Identifier':
          if (parent && parent.type === 'FunctionDeclaration' && parent.params.includes(node)) {
            tryToRemoveNode(this);
          } else if (parent && parent.type === 'FunctionExpression' && (parent.params.includes(node) || parent.id === node)) {
            tryToRemoveNode(this);
          }
          return;

        case 'UnaryExpression':
          tryToReplaceNode(this, node.argument);
          return;

        case 'LogicalExpression':
        case 'BinaryExpression':
          tryToReplaceNode(this, node.left);
          tryToReplaceNode(this, node.right);
          return;

        case 'ConditionalExpression':
          tryToReplaceNode(this, node.test);
          tryToReplaceNode(this, node.consequent);
          tryToReplaceNode(this, node.alternate);
          return;

        case 'AssignmentPattern':
          tryToReplaceNode(this, node.left);
          return;

        case 'ArrowFunctionExpression':
          if (node.expression) {
            tryToReplaceNode(this, node.body);
          }
          return;
      }
    },
  });
  return didSimplifySomething;
}

function randomJavaScriptAST() {
  const identifiers = [
    'foo',
    'bar',
    'a',
    'b',
    'c',
  ];

  function randomInt(count) {
    return Math.random() * count | 0;
  }

  function randomOrNull(fn) {
    return Math.random() < 0.5 ? fn() : null;
  }

  function randomMap(probability, fn) {
    const array = [];
    while (Math.random() < probability) {
      array.push(fn());
    }
    return array;
  }

  function randomIdent() {
    return { type: 'Identifier', name: identifiers[randomInt(identifiers.length)] };
  }

  function randomParam() {
    if (Math.random() < 0.5) {
      return randomIdent();
    } else {
      return {
        type: "AssignmentPattern",
        left: randomIdent(),
        right: randomExpr(),
      }
    }
  }

  function randomExpr() {
    if (nestingDepth > 20) {
      return { type: 'Literal', value: randomInt(10) };
    }
    try {
      nestingDepth++;
      switch (randomInt(17)) {
        case 0: return { type: 'Literal', value: randomInt(10) };
        case 1: return { type: 'Literal', value: true };
        case 2: return { type: 'Literal', value: false };
        case 3: return { type: 'Literal', value: null };
        case 4: return { type: 'UnaryExpression', operator: 'void', argument: { type: 'Literal', value: 0 } };
        case 5: return { type: 'Literal', value: identifiers[randomInt(identifiers.length)] };
        case 6: return randomIdent();
        case 7: return { type: 'ArrayExpression', elements: randomMap(0.5, randomExpr) };
        case 8: return { type: 'ConditionalExpression', test: randomExpr(), consequent: randomExpr(), alternate: randomExpr() };
        case 9: return { type: 'CallExpression', callee: randomExpr(), arguments: randomMap(0.5, randomExpr) };
        case 10: return {
          type: 'FunctionExpression',
          id: randomOrNull(randomIdent),
          params: randomMap(0.5, randomParam),
          body: { type: 'BlockStatement', body: wrapInsideFunc(() => randomMap(0.75, randomStmt)) },
        };
        case 11: return { type: 'LogicalExpression', operator: '&&', left: randomExpr(), right: randomExpr() };
        case 12: return { type: 'LogicalExpression', operator: '||', left: randomExpr(), right: randomExpr() };
        case 13: return { type: 'BinaryExpression', operator: '===', left: randomExpr(), right: randomExpr() };
        case 14: return { type: 'BinaryExpression', operator: '!==', left: randomExpr(), right: randomExpr() };
        case 15: return { type: 'UnaryExpression', operator: '!', argument: randomExpr() };
        case 16: {
          const expression = Math.random() < 0.5;
          return {
            type: 'ArrowFunctionExpression',
            params: randomMap(0.5, randomParam),
            expression,
            body: expression ? randomExpr() : { type: 'BlockStatement', body: wrapInsideFunc(() => randomMap(0.75, randomStmt)) },
          };
        }
      };
    } finally {
      nestingDepth--;
    }
  }

  let nestedFuncCount = 0;
  let nestingDepth = 0;

  function wrapInsideFunc(fn) {
    nestedFuncCount++;
    const value = fn();
    nestedFuncCount--;
    return value;
  }

  function randomStmt() {
    if (nestingDepth > 10) {
      return { type: 'ExpressionStatement', expression: randomExpr() };
    }
    try {
      nestingDepth++;
      switch (randomInt(4)) {
        case 0: return { type: 'ExpressionStatement', expression: randomExpr() };
        case 1: return { type: 'ThrowStatement', argument: randomExpr() };
        case 2: return nestedFuncCount > 0
          ? randomStmt()
          : { type: 'ReturnStatement', argument: randomOrNull(randomExpr) };
        case 3: return {
          type: 'FunctionDeclaration',
          id: randomIdent(),
          params: randomMap(0.5, randomParam),
          body: { type: 'BlockStatement', body: wrapInsideFunc(() => randomMap(0.75, randomStmt)) },
        };
      }
    } finally {
      nestingDepth--;
    }
  }

  return {
    type: 'Program',
    body: randomMap(0.75, randomStmt),
  };
}

fuzz();
