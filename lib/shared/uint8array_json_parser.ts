const enum State {
  TopLevel,
  Array,
  Object,
}

const enum Char {
  // Punctuation
  Newline = 0x0A,
  Space = 0x20,
  Quote = 0x22,
  Plus = 0x2B,
  Comma = 0x2C,
  Minus = 0x2D,
  Dot = 0x2E,
  Slash = 0x2F,
  Colon = 0x3A,
  OpenBracket = 0x5B,
  Backslash = 0x5C,
  CloseBracket = 0x5D,
  OpenBrace = 0x7B,
  CloseBrace = 0x7D,

  // Numbers
  Digit0 = 0x30,
  Digit1 = 0x31,
  Digit2 = 0x32,
  Digit3 = 0x33,
  Digit4 = 0x34,
  Digit5 = 0x35,
  Digit6 = 0x36,
  Digit7 = 0x37,
  Digit8 = 0x38,
  Digit9 = 0x39,

  // Uppercase letters
  UpperA = 0x41,
  UpperE = 0x45,
  UpperF = 0x46,

  // Lowercase letters
  LowerA = 0x61,
  LowerB = 0x62,
  LowerE = 0x65,
  LowerF = 0x66,
  LowerL = 0x6C,
  LowerN = 0x6E,
  LowerR = 0x72,
  LowerS = 0x73,
  LowerT = 0x74,
  LowerU = 0x75,
}

const fromCharCode = String.fromCharCode

function throwSyntaxError(bytes: Uint8Array, index: number, message?: string): void {
  const c = bytes[index]
  let line = 1
  let column = 0

  for (let i = 0; i < index; i++) {
    if (bytes[i] === Char.Newline) {
      line++
      column = 0
    } else {
      column++
    }
  }

  throw new SyntaxError(
    message ? message :
      index === bytes.length ? 'Unexpected end of input while parsing JSON' :
        c >= 0x20 && c <= 0x7E ? `Unexpected character ${fromCharCode(c)} in JSON at position ${index} (line ${line}, column ${column})` :
          `Unexpected byte 0x${c.toString(16)} in JSON at position ${index} (line ${line}, column ${column})`)
}

export function JSON_parse(bytes: Uint8Array): any {
  if (!(bytes instanceof Uint8Array)) {
    throw new Error(`JSON input must be a Uint8Array`)
  }

  const propertyStack: (string | null)[] = []
  const objectStack: any[] = []
  const stateStack: State[] = []
  const length = bytes.length
  let property: string | null = null
  let state = State.TopLevel
  let object: any
  let i = 0

  while (i < length) {
    let c = bytes[i++]

    // Skip whitespace
    if (c <= Char.Space) {
      continue
    }

    let value: any

    // Validate state inside objects
    if (state === State.Object && property === null && c !== Char.Quote && c !== Char.CloseBrace) {
      throwSyntaxError(bytes, --i)
    }

    switch (c) {
      // True
      case Char.LowerT: {
        if (bytes[i++] !== Char.LowerR || bytes[i++] !== Char.LowerU || bytes[i++] !== Char.LowerE) {
          throwSyntaxError(bytes, --i)
        }

        value = true
        break
      }

      // False
      case Char.LowerF: {
        if (bytes[i++] !== Char.LowerA || bytes[i++] !== Char.LowerL || bytes[i++] !== Char.LowerS || bytes[i++] !== Char.LowerE) {
          throwSyntaxError(bytes, --i)
        }

        value = false
        break
      }

      // Null
      case Char.LowerN: {
        if (bytes[i++] !== Char.LowerU || bytes[i++] !== Char.LowerL || bytes[i++] !== Char.LowerL) {
          throwSyntaxError(bytes, --i)
        }

        value = null
        break
      }

      // Number begin
      case Char.Minus:
      case Char.Dot:
      case Char.Digit0:
      case Char.Digit1:
      case Char.Digit2:
      case Char.Digit3:
      case Char.Digit4:
      case Char.Digit5:
      case Char.Digit6:
      case Char.Digit7:
      case Char.Digit8:
      case Char.Digit9: {
        let index = i
        value = fromCharCode(c)
        c = bytes[i]

        // Scan over the rest of the number
        while (true) {
          switch (c) {
            case Char.Plus:
            case Char.Minus:
            case Char.Dot:
            case Char.Digit0:
            case Char.Digit1:
            case Char.Digit2:
            case Char.Digit3:
            case Char.Digit4:
            case Char.Digit5:
            case Char.Digit6:
            case Char.Digit7:
            case Char.Digit8:
            case Char.Digit9:
            case Char.LowerE:
            case Char.UpperE: {
              value += fromCharCode(c)
              c = bytes[++i]
              continue
            }
          }

          // Number end
          break
        }

        // Convert the string to a number
        value = +value

        // Validate the number
        if (isNaN(value)) {
          throwSyntaxError(bytes, --index, 'Invalid number')
        }

        break
      }

      // String begin
      case Char.Quote: {
        value = ''

        while (true) {
          if (i >= length) {
            throwSyntaxError(bytes, length)
          }

          c = bytes[i++]

          // String end
          if (c === Char.Quote) {
            break
          }

          // Escape sequence
          else if (c === Char.Backslash) {
            switch (bytes[i++]) {
              // Normal escape sequence
              case Char.Quote: value += '\"'; break
              case Char.Slash: value += '\/'; break
              case Char.Backslash: value += '\\'; break
              case Char.LowerB: value += '\b'; break
              case Char.LowerF: value += '\f'; break
              case Char.LowerN: value += '\n'; break
              case Char.LowerR: value += '\r'; break
              case Char.LowerT: value += '\t'; break

              // Unicode escape sequence
              case Char.LowerU: {
                let code = 0
                for (let j = 0; j < 4; j++) {
                  c = bytes[i++]
                  code <<= 4
                  if (c >= Char.Digit0 && c <= Char.Digit9) code |= c - Char.Digit0
                  else if (c >= Char.LowerA && c <= Char.LowerF) code |= c + (10 - Char.LowerA)
                  else if (c >= Char.UpperA && c <= Char.UpperF) code |= c + (10 - Char.UpperA)
                  else throwSyntaxError(bytes, --i)
                }
                value += fromCharCode(code)
                break
              }

              // Invalid escape sequence
              default: throwSyntaxError(bytes, --i); break
            }
          }

          // ASCII text
          else if (c <= 0x7F) {
            value += fromCharCode(c)
          }

          // 2-byte UTF-8 sequence
          else if ((c & 0xE0) === 0xC0) {
            value += fromCharCode(((c & 0x1F) << 6) | (bytes[i++] & 0x3F))
          }

          // 3-byte UTF-8 sequence
          else if ((c & 0xF0) === 0xE0) {
            value += fromCharCode(((c & 0x0F) << 12) | ((bytes[i++] & 0x3F) << 6) | (bytes[i++] & 0x3F))
          }

          // 4-byte UTF-8 sequence
          else if ((c & 0xF8) == 0xF0) {
            let codePoint = ((c & 0x07) << 18) | ((bytes[i++] & 0x3F) << 12) | ((bytes[i++] & 0x3F) << 6) | (bytes[i++] & 0x3F)
            if (codePoint > 0xFFFF) {
              codePoint -= 0x10000
              value += fromCharCode(((codePoint >> 10) & 0x3FF) | 0xD800)
              codePoint = 0xDC00 | (codePoint & 0x3FF)
            }
            value += fromCharCode(codePoint)
          }
        }

        // Force V8's rope representation to be flattened to compact the string and avoid running out of memory
        value[0]
        break
      }

      // Array begin
      case Char.OpenBracket: {
        value = []

        // Push the stack
        propertyStack.push(property)
        objectStack.push(object)
        stateStack.push(state)

        // Enter the array
        property = null
        object = value
        state = State.Array
        continue
      }

      // Object begin
      case Char.OpenBrace: {
        value = {}

        // Push the stack
        propertyStack.push(property)
        objectStack.push(object)
        stateStack.push(state)

        // Enter the object
        property = null
        object = value
        state = State.Object
        continue
      }

      // Array end
      case Char.CloseBracket: {
        if (state !== State.Array) {
          throwSyntaxError(bytes, --i)
        }

        // Leave the array
        value = object

        // Pop the stack
        property = propertyStack.pop() as string | null
        object = objectStack.pop()
        state = stateStack.pop() as State
        break
      }

      // Object end
      case Char.CloseBrace: {
        if (state !== State.Object) {
          throwSyntaxError(bytes, --i)
        }

        // Leave the object
        value = object

        // Pop the stack
        property = propertyStack.pop() as string | null
        object = objectStack.pop()
        state = stateStack.pop() as State
        break
      }

      default: {
        throwSyntaxError(bytes, --i)
      }
    }

    c = bytes[i]

    // Skip whitespace
    while (c <= Char.Space) {
      c = bytes[++i]
    }

    switch (state) {
      case State.TopLevel: {
        // Expect the end of the input
        if (i === length) {
          return value
        }

        break
      }

      case State.Array: {
        object.push(value)

        // Check for more values
        if (c === Char.Comma) {
          i++
          continue
        }

        // Expect the end of the array
        if (c === Char.CloseBracket) {
          continue
        }

        break
      }

      case State.Object: {
        // Property
        if (property === null) {
          property = value

          // Expect a colon
          if (c === Char.Colon) {
            i++
            continue
          }
        }

        // Value
        else {
          object[property] = value
          property = null

          // Check for more values
          if (c === Char.Comma) {
            i++
            continue
          }

          // Expect the end of the object
          if (c === Char.CloseBrace) {
            continue
          }
        }

        break
      }
    }

    // It's an error if we get here
    break
  }

  throwSyntaxError(bytes, i)
}
