// The JavaScript API communicates with the Go child process over stdin/stdout
// using this protocol. It's a very simple binary protocol that uses length-
// prefixed strings. Every message has a 32-bit integer id and is either a
// request or a response. You must send a response after receiving a request
// because the other end is blocking on the response coming back. Requests are
// arrays of strings and responses are maps of strings to byte arrays.

import * as types from "./types";

export type Request = string[];
export type Response = { [key: string]: Uint8Array };

export function encodeRequest(id: number, request: Request): Uint8Array {
  // Figure out how long the request will be
  let argBuffers: Uint8Array[] = [];
  let length = 12;
  for (let arg of request) {
    let argBuffer = encodeUTF8(arg);
    argBuffers.push(argBuffer);
    length += 4 + argBuffer.length;
  }

  // Write out the request message
  let bytes = new Uint8Array(length);
  let offset = 0;
  let writeUint32 = (value: number) => {
    writeUInt32LE(bytes, value, offset);
    offset += 4;
  };
  writeUint32(length - 4);
  writeUint32(id << 1);
  writeUint32(argBuffers.length);
  for (let argBuffer of argBuffers) {
    writeUint32(argBuffer.length);
    bytes.set(argBuffer, offset);
    offset += argBuffer.length;
  }
  return bytes;
}

export function encodeResponse(id: number, response: Response): Uint8Array {
  // Figure out how long the response will be
  let keyBuffers: { [key: string]: Uint8Array } = {};
  let length = 12;
  let count = 0;
  for (let key in response) {
    let keyBuffer = encodeUTF8(key);
    let value = response[key];
    keyBuffers[key] = keyBuffer;
    length += 4 + keyBuffer.length + 4 + value.length;
    count++;
  }

  // Write out the request message
  let bytes = new Uint8Array(length);
  let offset = 0;
  let writeUint32 = (value: number) => {
    writeUInt32LE(bytes, value, offset);
    offset += 4;
  };
  writeUint32(length - 4);
  writeUint32((id << 1) | 1);
  writeUint32(count);
  for (let key in response) {
    let keyBuffer = keyBuffers[key];
    let value = response[key];
    writeUint32(keyBuffer.length);
    bytes.set(keyBuffer, offset);
    offset += keyBuffer.length;
    writeUint32(value.length);
    bytes.set(value, offset);
    offset += value.length;
  }
  return bytes;
}

export function decodeRequestOrResponse(bytes: Uint8Array): [number, Request | null, Response | null] {
  let offset = 0;
  let eat = (n: number) => {
    offset += n;
    if (offset > bytes.length) throw new Error('Invalid message');
    return offset - n;
  };

  // Read the id
  let id = readUInt32LE(bytes, eat(4));
  let isRequest = !(id & 1);
  id >>>= 1;

  // Read the argument count
  let argCount = readUInt32LE(bytes, eat(4));

  if (isRequest) {
    // Read the request
    let request: Request = [];
    for (let i = 0; i < argCount; i++) {
      let valueLength = readUInt32LE(bytes, eat(4));
      let value = bytes.slice(offset, eat(valueLength) + valueLength);
      request.push(decodeUTF8(value));
    }
    if (offset !== bytes.length) throw new Error('Invalid request');
    return [id, request, null];
  } else {
    // Read the response
    let response: Response = {};
    for (let i = 0; i < argCount; i++) {
      let keyLength = readUInt32LE(bytes, eat(4));
      let key = decodeUTF8(bytes.slice(offset, eat(keyLength) + keyLength));
      let valueLength = readUInt32LE(bytes, eat(4));
      let value = bytes.slice(offset, eat(valueLength) + valueLength);
      response[key] = value;
    }
    if (offset !== bytes.length) throw new Error('Invalid response');
    return [id, null, response];
  }
}

export function decodeOutputFiles(bytes: Uint8Array): types.OutputFile[] {
  let outputFiles: types.OutputFile[] = [];
  let offset = 0;
  let count = readUInt32LE(bytes, offset);
  offset += 4;
  for (let i = 0; i < count; i++) {
    let pathLength = readUInt32LE(bytes, offset);
    let path = decodeUTF8(bytes.slice(offset + 4, offset + 4 + pathLength));
    offset += 4 + pathLength;
    let contentsLength = readUInt32LE(bytes, offset);
    let contents = new Uint8Array(bytes.slice(offset + 4, offset + 4 + contentsLength));
    offset += 4 + contentsLength;
    outputFiles.push({ path, contents });
  }
  return outputFiles;
}

export function jsonToMessages(json: Uint8Array): types.Message[] {
  let parts = JSON.parse(decodeUTF8(json));
  let messages: types.Message[] = [];
  for (let i = 0; i < parts.length; i += 6) {
    messages.push({
      text: parts[i],
      location: parts[i + 1] < 0 ? null : {
        length: parts[i + 1],
        file: parts[i + 2],
        line: parts[i + 3],
        column: parts[i + 4],
        lineText: parts[i + 5],
      },
    });
  }
  return messages;
}

export let encodeUTF8: (text: string) => Uint8Array
export let decodeUTF8: (bytes: Uint8Array) => string

// For the browser and node 12.x
if (typeof TextEncoder !== 'undefined' && typeof TextDecoder !== 'undefined') {
  let encoder = new TextEncoder();
  let decoder = new TextDecoder();
  encodeUTF8 = text => encoder.encode(text);
  decodeUTF8 = bytes => decoder.decode(bytes);
}

// For node 10.x
else if (typeof Buffer !== 'undefined') {
  encodeUTF8 = text => Buffer.from(text);
  decodeUTF8 = bytes => Buffer.from(bytes).toString();
}

else {
  throw new Error('No UTF-8 codec found');
}

export function readUInt32LE(buffer: Uint8Array, offset: number): number {
  return buffer[offset++] |
    (buffer[offset++] << 8) |
    (buffer[offset++] << 16) |
    (buffer[offset++] << 24);
}

export function writeUInt32LE(buffer: Uint8Array, value: number, offset: number): void {
  buffer[offset++] = value;
  buffer[offset++] = value >> 8;
  buffer[offset++] = value >> 16;
  buffer[offset++] = value >> 24;
}
