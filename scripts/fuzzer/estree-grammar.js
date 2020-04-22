// These helper functions are shortcuts to make the rules below cleaner
const oneOf = (...of) => ({ type: 'choice', of })
const nullable = of => oneOf(literal(null), of)
const arrayOf = of => ({ type: 'array', of })
const arrayOfWithTrailing = (of, trailing) => ({ type: 'array', of, trailing })
const nonEmptyArrayOf = of => ({ type: 'array', of, nonEmpty: true })
const literal = literal => ({ type: 'literal', literal })

module.exports = {
  Program: {
    body: arrayOf(oneOf('Statement', 'Declaration', 'ModuleDeclaration')),
  },

  $Pattern: {
    Identifier: {
      name: 'string',
    },
    ArrayPattern: {
      elements: arrayOfWithTrailing(oneOf('Pattern', literal(null)), 'RestElement'),
    },
    ObjectPattern: {
      properties: arrayOf('AssignmentProperty'),
    },
    AssignmentPattern: {
      left: 'NoAssignmentPattern',
      right: 'Expression',
    },
  },

  $NoAssignmentPattern: {
    Identifier: {
      name: 'string',
    },
    ArrayPattern: {
      elements: arrayOfWithTrailing(oneOf('Pattern', literal(null)), 'RestElement'),
    },
    ObjectPattern: {
      properties: arrayOf('AssignmentProperty'),
    },
  },

  AssignmentProperty: {
    type: literal('Property'),
    key: 'Identifier',
    value: 'AssignmentPropertyPattern',
    kind: literal('init'),
    method: literal(false),
  },

  $AssignmentPropertyPattern: {
    Identifier: {
      name: 'string',
    },
    AssignmentPattern: {
      left: 'NoAssignmentPattern',
      right: 'Expression',
    },
  },

  RestElement: {
    argument: 'NoAssignmentPattern',
  },

  $Expression: {
    ThisExpression: {},
    Identifier: {
      name: 'string',
    },
    Literal: {
      value: oneOf('boolean', 'string', 'number', 'regexp', literal(null)),
    },
    ArrayExpression: {
      elements: arrayOfWithTrailing(oneOf('Expression', literal(null)), 'SpreadElement'),
    },
    ObjectExpression: {
      properties: arrayOf('Property'),
    },
    FunctionExpression: {
      id: nullable('Identifier'),
      async: 'boolean',
      params: arrayOf('Pattern'),
      body: 'BlockStatement',
    },
    UnaryExpression: {
      operator: oneOf(literal('-'), literal('+'), literal('!'), literal('~'), literal('typeof'), literal('void'), literal('delete')),
      prefix: 'boolean',
      argument: 'Expression',
    },
    UpdateExpression: {
      operator: oneOf(literal('--'), literal('++')),
      argument: 'Expression',
      prefix: 'boolean',
    },
    BinaryExpression: {
      operator: oneOf(
        literal('=='), literal('!='), literal('==='), literal('!=='),
        literal('<'), literal('<='), literal('>'), literal('>='),
        literal('<<'), literal('>>'), literal('>>>'),
        literal('+'), literal('-'), literal('*'), literal('/'), literal('%'),
        literal('|'), literal('^'), literal('&'), literal('in'),
        literal('instanceof'), literal('**'),
      ),
      left: 'Expression',
      right: 'Expression',
    },
    AssignmentExpression: {
      operator: oneOf(
        literal('='), literal('+='), literal('-='), literal('*='), literal('/='), literal('%='),
        literal('<<='), literal('>>='), literal('>>>='),
        literal('|='), literal('^='), literal('&='), literal('**='),
      ),
      left: oneOf('NoAssignmentPattern', 'Expression'),
      right: 'Expression',
    },
    LogicalExpression: {
      operator: oneOf(literal('||'), literal('&&')),
      left: 'Expression',
      right: 'Expression',
    },
    $MemberExpression: [
      {
        object: 'Expression',
        property: 'Identifier',
        computed: literal(false),
      },
      {
        object: 'Expression',
        property: 'Expression',
        computed: literal(true),
      },
    ],
    ConditionalExpression: {
      test: 'Expression',
      alternate: 'Expression',
      consequent: 'Expression',
    },
    CallExpression: {
      callee: 'Expression',
      arguments: arrayOf(oneOf('Expression', 'SpreadElement')),
    },
    NewExpression: {
      callee: 'Expression',
      arguments: arrayOf(oneOf('Expression', 'SpreadElement')),
    },
    SequenceExpression: {
      expressions: nonEmptyArrayOf('Expression'),
    },
    $ArrowFunctionExpression: [
      {
        async: 'boolean',
        params: arrayOf('Pattern'),
        body: 'BlockStatement',
        expression: literal(false),
      },
      {
        async: 'boolean',
        params: arrayOf('Pattern'),
        body: 'Expression',
        expression: literal(true),
      },
    ],
    ClassExpression: {
      id: nullable('Identifier'),
      superClass: nullable('SuperClassExpression'),
      body: 'ClassBody',
    },
  },

  // Try to work around escodegen bugs that don't correctly parenthesize super class expressions
  $SuperClassExpression: {
    ThisExpression: {},
    Identifier: {
      name: 'string',
    },
    Literal: {
      value: oneOf('boolean', 'string', 'number', 'regexp', literal(null)),
    },
    ArrayExpression: {
      elements: arrayOf(oneOf('Expression', 'SpreadElement', literal(null))),
    },
    ObjectExpression: {
      properties: arrayOf('Property'),
    },
    FunctionExpression: {
      id: nullable('Identifier'),
      async: 'boolean',
      params: arrayOf('Pattern'),
      body: 'BlockStatement',
    },
    $MemberExpression: [
      {
        object: 'Expression',
        property: 'Identifier',
        computed: literal(false),
      },
      {
        object: 'Expression',
        property: 'Expression',
        computed: literal(true),
      },
    ],
    ClassExpression: {
      id: nullable('Identifier'),
      superClass: nullable('SuperClassExpression'),
      body: 'ClassBody',
    },
  },

  ForStatement: {
    init: nullable(oneOf('VariableDeclaration', 'Expression')),
    test: nullable('Expression'),
    update: nullable('Expression'),
    body: 'Statement',
  },

  $Statement: {
    EmptyStatement: {},
    DebuggerStatement: {},
    WithStatement: {
      object: 'Expression',
      body: 'Statement',
    },
    ReturnStatement: {
      argument: nullable('Expression'),
    },
    LabeledStatement: {
      label: 'Identifier',
      body: 'Statement',
    },
    BreakStatement: {
      label: nullable('Identifier'),
    },
    ContinueStatement: {
      label: nullable('Identifier'),
    },
    ExpressionStatement: {
      expression: 'Expression',
    },
    BlockStatement: {
      body: arrayOf(oneOf('Statement', 'Declaration')),
    },
    IfStatement: {
      test: 'Expression',
      consequent: 'Statement',
      alternate: nullable('Statement'),
    },
    SwitchStatement: {
      discriminant: 'Expression',
      cases: arrayOf('SwitchCase'),
    },
    ThrowStatement: {
      argument: 'Expression',
    },
    $TryStatement: [
      {
        block: 'BlockStatement',
        handler: 'CatchClause',
        finalizer: nullable('BlockStatement'),
      },
      {
        block: 'BlockStatement',
        handler: nullable('CatchClause'),
        finalizer: 'BlockStatement',
      },
    ],
    WhileStatement: {
      test: 'Expression',
      body: 'Statement',
    },
    DoWhileStatement: {
      body: 'Statement',
      test: 'Expression',
    },
    ForStatement: {
      init: nullable(oneOf('VariableDeclaration', 'Expression')),
      test: nullable('Expression'),
      update: nullable('Expression'),
      body: 'Statement',
    },
    ForInStatement: {
      left: oneOf('VariableDeclaration', 'Pattern'),
      right: 'Expression',
      body: 'Statement',
    },
    ForOfStatement: {
      left: oneOf('VariableDeclaration', 'Pattern'),
      right: 'Expression',
      body: 'Statement',
    },
  },

  $Declaration: {
    VariableDeclaration: {
      declarations: nonEmptyArrayOf('VariableDeclarator'),
      kind: oneOf(literal('var'), literal('let'), literal('const')),
    },
    FunctionDeclaration: {
      id: 'Identifier',
      async: 'boolean',
      params: arrayOf('Pattern'),
      body: 'BlockStatement',
    },
    ClassDeclaration: {
      id: 'Identifier',
      superClass: nullable('SuperClassExpression'),
      body: 'ClassBody',
    },
  },

  VariableDeclaration: {
    declarations: nonEmptyArrayOf('VariableDeclarator'),
    kind: oneOf(literal('var'), literal('let'), literal('const')),
  },

  VariableDeclarator: {
    id: 'NoAssignmentPattern',
    init: nullable('Expression'),
  },

  FunctionDeclaration: {
    id: 'Identifier',
    async: 'boolean',
    params: arrayOf('Pattern'),
    body: 'BlockStatement',
  },

  ClassDeclaration: {
    id: 'Identifier',
    superClass: nullable('SuperClassExpression'),
    body: 'ClassBody',
  },

  AnonymousDefaultExportedFunctionDeclaration: {
    type: literal('FunctionDeclaration'),
    id: literal(null),
    async: 'boolean',
    params: arrayOf('Pattern'),
    body: 'BlockStatement',
  },

  AnonymousDefaultExportedClassDeclaration: {
    type: literal('ClassDeclaration'),
    id: literal(null),
    superClass: nullable('SuperClassExpression'),
    body: 'ClassBody',
  },

  $ModuleDeclaration: {
    ImportDeclaration: {
      specifiers: arrayOf('ImportSpecifier', 'ImportDefaultSpecifier', 'ImportNamespaceSpecifier'),
      source: 'ImportPath',
    },
    $ExportNamedDeclaration: [
      {
        declaration: 'Declaration',
        source: 'ImportPath',
      },
      {
        specifiers: arrayOf('ExportSpecifier'),
        source: 'ImportPath',
      },
    ],
    ExportDefaultDeclaration: {
      declaration: oneOf(
        'AnonymousDefaultExportedFunctionDeclaration',
        'AnonymousDefaultExportedClassDeclaration',
        'FunctionDeclaration',
        'ClassDeclaration',
        'Expression',
      ),
    },
    ExportAllDeclaration: {
      source: 'ImportPath',
    },
  },

  ImportSpecifier: {
    local: 'Identifier',
    imported: 'Identifier',
  },

  ExportSpecifier: {
    local: 'Identifier',
    imported: 'Identifier',
  },

  ImportDefaultSpecifier: {
    local: 'Identifier',
  },

  ImportNamespaceSpecifier: {
    local: 'Identifier',
  },

  ImportPath: {
    type: literal('Literal'),
    value: 'string',
  },

  Identifier: {
    name: 'string',
  },

  SpreadElement: {
    argument: 'Expression',
  },

  CatchClause: {
    param: 'NoAssignmentPattern',
    body: 'BlockStatement',
  },

  BlockStatement: {
    body: arrayOf(oneOf('Statement', 'Declaration')),
  },

  SwitchCase: {
    test: nullable('Expression'),
    consequent: arrayOf(oneOf('Statement', 'Declaration')),
  },

  FunctionExpression: {
    id: nullable('Identifier'),
    async: 'boolean',
    params: arrayOf('Pattern'),
    body: 'BlockStatement',
  },

  NoAsyncFunctionExpression: {
    type: literal('FunctionExpression'),
    id: nullable('Identifier'),
    params: arrayOf('Pattern'),
    body: 'BlockStatement',
  },

  $Property: [
    {
      key: 'Identifier',
      value: 'Expression',
      kind: literal('init'),
    },
    {
      key: 'Identifier',
      value: 'NoAsyncFunctionExpression',
      kind: oneOf(literal('get'), literal('set')),
    },
    {
      key: 'Identifier',
      value: 'FunctionExpression',
      kind: literal('init'),
      method: literal(true),
    },
    {
      key: 'Expression',
      computed: literal(true),
      value: 'Expression',
      kind: literal('init'),
    },
    {
      key: 'Expression',
      computed: literal(true),
      value: 'NoAsyncFunctionExpression',
      kind: oneOf(literal('get'), literal('set')),
    },
    {
      key: 'Expression',
      computed: literal(true),
      value: 'FunctionExpression',
      kind: literal('init'),
      method: literal(true),
    },
  ],

  ClassBody: {
    body: arrayOf('MethodDefinition'),
  },

  $MethodDefinition: [
    {
      key: 'Identifier',
      value: 'FunctionExpression',
      kind: literal('method'),
      computed: literal(false),
      static: 'boolean',
    },
    {
      key: 'Identifier',
      value: 'NoAsyncFunctionExpression',
      kind: oneOf(literal('get'), literal('set')),
      computed: literal(false),
      static: 'boolean',
    },
    {
      key: 'Expression',
      value: 'FunctionExpression',
      kind: literal('method'),
      computed: literal(true),
      static: 'boolean',
    },
    {
      key: 'Expression',
      value: 'NoAsyncFunctionExpression',
      kind: oneOf(literal('get'), literal('set')),
      computed: literal(true),
      static: 'boolean',
    },
  ],
}
