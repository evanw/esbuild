// The JavaScript API communicates with the Go child process over stdin/stdout
// using this protocol. It's a very simple binary protocol that uses primitives
// and nested arrays and maps. You must send a response after receiving a
// request because the other end is blocking on the response coming back.

package main

import (
	"encoding/binary"
	"sort"
)

func readUint32(bytes []byte) (value uint32, leftOver []byte, ok bool) {
	if len(bytes) >= 4 {
		return binary.LittleEndian.Uint32(bytes), bytes[4:], true
	}

	return 0, bytes, false
}

func writeUint32(bytes []byte, value uint32) []byte {
	bytes = append(bytes, 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(bytes[len(bytes)-4:], value)
	return bytes
}

func readLengthPrefixedSlice(bytes []byte) (slice []byte, leftOver []byte, ok bool) {
	if length, afterLength, ok := readUint32(bytes); ok && uint(len(afterLength)) >= uint(length) {
		return afterLength[:length], afterLength[length:], true
	}

	return []byte{}, bytes, false
}

type packet struct {
	id        uint32
	isRequest bool
	value     interface{}
}

func encodePacket(p packet) []byte {
	var visit func(interface{})
	var bytes []byte

	visit = func(value interface{}) {
		switch v := value.(type) {
		case nil:
			bytes = append(bytes, 0)

		case bool:
			n := uint8(0)
			if v {
				n = 1
			}
			bytes = append(bytes, 1, n)

		case int:
			bytes = append(bytes, 2)
			bytes = writeUint32(bytes, uint32(v))

		case string:
			bytes = append(bytes, 3)
			bytes = writeUint32(bytes, uint32(len(v)))
			bytes = append(bytes, v...)

		case []byte:
			bytes = append(bytes, 4)
			bytes = writeUint32(bytes, uint32(len(v)))
			bytes = append(bytes, v...)

		case []interface{}:
			bytes = append(bytes, 5)
			bytes = writeUint32(bytes, uint32(len(v)))
			for _, item := range v {
				visit(item)
			}

		case map[string]interface{}:
			// Sort keys for determinism
			keys := make([]string, 0, len(v))
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			bytes = append(bytes, 6)
			bytes = writeUint32(bytes, uint32(len(keys)))
			for _, k := range keys {
				bytes = writeUint32(bytes, uint32(len(k)))
				bytes = append(bytes, k...)
				visit(v[k])
			}

		default:
			panic("Invalid packet")
		}
	}

	bytes = writeUint32(bytes, 0) // Reserve space for the length
	if p.isRequest {
		bytes = writeUint32(bytes, p.id<<1)
	} else {
		bytes = writeUint32(bytes, (p.id<<1)|1)
	}
	visit(p.value)
	writeUint32(bytes[:0], uint32(len(bytes)-4)) // Patch the length in
	return bytes
}

func decodePacket(bytes []byte) (packet, bool) {
	var visit func() (interface{}, bool)

	visit = func() (interface{}, bool) {
		kind := bytes[0]
		bytes = bytes[1:]
		switch kind {
		case 0: // nil
			return nil, true

		case 1: // bool
			value := bytes[0]
			bytes = bytes[1:]
			return value != 0, true

		case 2: // int
			value, next, ok := readUint32(bytes)
			if !ok {
				return nil, false
			}
			bytes = next
			return int(value), true

		case 3: // string
			value, next, ok := readLengthPrefixedSlice(bytes)
			if !ok {
				return nil, false
			}
			bytes = next
			return string(value), true

		case 4: // []byte
			value, next, ok := readLengthPrefixedSlice(bytes)
			if !ok {
				return nil, false
			}
			bytes = next
			return value, true

		case 5: // []interface{}
			count, next, ok := readUint32(bytes)
			if !ok {
				return nil, false
			}
			bytes = next
			value := make([]interface{}, count)
			for i := 0; i < int(count); i++ {
				item, ok := visit()
				if !ok {
					return nil, false
				}
				value[i] = item
			}
			return value, true

		case 6: // map[string]interface{}
			count, next, ok := readUint32(bytes)
			if !ok {
				return nil, false
			}
			bytes = next
			value := make(map[string]interface{}, count)
			for i := 0; i < int(count); i++ {
				key, next, ok := readLengthPrefixedSlice(bytes)
				if !ok {
					return nil, false
				}
				bytes = next
				item, ok := visit()
				if !ok {
					return nil, false
				}
				value[string(key)] = item
			}
			return value, true

		default:
			panic("Invalid packet")
		}
	}

	id, bytes, ok := readUint32(bytes)
	if !ok {
		return packet{}, false
	}
	isRequest := (id & 1) == 0
	id >>= 1
	value, ok := visit()
	if !ok {
		return packet{}, false
	}
	if len(bytes) != 0 {
		return packet{}, false
	}
	return packet{id: id, isRequest: isRequest, value: value}, true
}
