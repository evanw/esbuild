// This is a simple script to get esbuild running in the browser. It's not at
// all fast. This script is intended to be used for a playground-style sandbox
// that lets you try out esbuild to understand how it transforms your code.

/**
 * @param {{
 *   wasmExecJsUrl: string,
 *   esbuildWasmUrl: string,
 *   worker?: boolean,
 * }} options
 *
 * @returns {Promise<
 *   (args: {files: Record<string, string>, flags: string[]}) => Promise<
 *     | {error: string}
 *     | {error: null, code: number, files: Record<string, string>}
 *   >
 * >}
 */
async function esbuildStartWorker(options) {
  const [wasmExecJs, esbuildWasm] = await Promise.all([
    fetch(options.wasmExecJsUrl).then(r => r.text()),
    fetch(options.esbuildWasmUrl).then(r => r.arrayBuffer()),
  ])
  const code = `${wasmExecJs};(${workerThread})();`

  if (options.worker !== false) {
    // Run esbuild off the main thread
    const blob = new Blob([code], { type: 'application/javascript' })
    worker = new Worker(URL.createObjectURL(blob))
  } else {
    // Run esbuild on the main thread
    const fn = new Function('postMessage', code + `var onmessage; return m => onmessage(m)`)
    const onmessage = fn(data => worker.onmessage({ data }))
    worker = {
      onmessage: null,
      postMessage: data => onmessage({ data }),
    }
  }

  const queue = []
  let isRunning = false
  worker.postMessage(esbuildWasm)

  return ({ files, flags }) => {
    return new Promise((resolve, reject) => {
      // Use a queue to make sure we only run one esbuild at a time
      queue.push({ resolve, reject, files, flags })
      runNext()
    })
  }

  function runNext() {
    if (isRunning || !queue.length) return
    isRunning = true
    const { resolve, reject, files, flags } = queue.shift()
    worker.onmessage = ({ data: { error, files, code } }) => {
      if (error !== null) reject(new Error(error))
      else resolve({ files, code })
      isRunning = false
      runNext()
    }
    worker.postMessage({ files, flags })
  }

  // This function is never called directly. Instead we call toString() on it
  // and use it to construct the source code for the worker.
  function workerThread() {
    onmessage = ({ data: wasm }) => {
      onmessage = ({ data: args }) => run(args)
        .catch(e => ({ error: e + '' }))
        .then(message => postMessage(message))

      async function run({ files, flags }) {
        let code = -1
        const fs = FS.fromFiles(files)
        self.fs = wrapForNode(fs)
        const go = new Go()
        go.env = { PWD: '/home/user' }
        go.argv = [''].concat(flags)
        go.exit = exit => code = exit
        const result = await WebAssembly.instantiate(wasm, go.importObject)
        await go.run(result.instance)
        return { error: null, code, files: fs.toFiles() }
      }
    }

    // A simple file system implementation that tracks a directory tree. Each
    // file system entry is either a directory or a file:
    //
    //   type Entry = { dirEntries: Record<string, Entry> } | { contents: string }
    //
    class FS {
      constructor() {
        this.root = { dirEntries: Object.create(null) }
      }

      static split(path) {
        const parts = path.split('/')
        while (parts.length > 1 && parts[parts.length - 1] === '') parts.pop()
        return [parts.slice(0, -1).join('/') || '/', parts[parts.length - 1]]
      }

      find(path) {
        if (path === '.') path = '/home/user'
        let node = this.root
        for (const part of path.split('/')) {
          if (part === '') continue
          if (!(part in node.dirEntries)) return null
          node = node.dirEntries[part]
        }
        return node
      }

      mkdir(path) {
        const [dir, base] = FS.split(path)
        const node = this.find(dir)
        if (!node || !node.dirEntries) throw new Error(`Invalid directory: ${dir}`)
        if (base in node.dirEntries) throw new Error(`Already exists: ${path}`)
        return node.dirEntries[base] = { dirEntries: Object.create(null) }
      }

      mkfile(path) {
        const [dir, base] = FS.split(path)
        const node = this.find(dir)
        if (!node || !node.dirEntries) throw new Error(`Invalid directory: ${dir}`)
        if (base in node.dirEntries) throw new Error(`Already exists: ${path}`)
        return node.dirEntries[base] = { contents: '' }
      }

      static fromFiles(files) {
        const fs = new FS()
        const ensureDir = path => {
          if (path === '/') return fs.root
          const [dir, base] = FS.split(path)
          const node = ensureDir(dir)
          if (!(base in node.dirEntries)) node.dirEntries[base] = { dirEntries: Object.create(null) }
          else if (!node.dirEntries[base].dirEntries) throw new Error(`Invalid directory: ${dir}`)
          return node.dirEntries[base]
        }
        for (const path in files) {
          const [dir] = FS.split(path)
          ensureDir(dir)
          fs.mkfile(path).contents = files[path]
        }
        return fs
      }

      toFiles() {
        const files = {}
        const findFiles = (path, node) => {
          for (const name in node.dirEntries) {
            const absName = path + '/' + name
            if (node.dirEntries[name].dirEntries) findFiles(absName, node.dirEntries[name])
            else files[absName] = node.dirEntries[name].contents
          }
        }
        findFiles('', this.root)
        return files
      }
    }

    // Return an object that's similar enough to node's "fs" module to trick
    // the Go WebAssembly runtime into running esbuild successfully.
    function wrapForNode(fs) {
      const fds = new Map()
      let nextFd = 3

      const enosys = text => {
        const err = new Error(text)
        err.code = 'ENOSYS'
        return err
      }

      const wrapStat = isDir => ({
        dev: 0,
        ino: 0,
        mode: isDir ? 0x4000 : 0,
        nlink: 0,
        uid: 0,
        gid: 0,
        rdev: 0,
        size: 0,
        blksize: 0,
        blocks: 0,
        atimeMs: 0,
        mtimeMs: 0,
        ctimeMs: 0,
        birthtimeMs: 0,
        isDirectory: () => isDir,
      })

      const decoder = new TextDecoder('utf-8')
      const openForWriting = node => {
        return {
          node,
          writeSync(buf) {
            node.contents += decoder.decode(buf)
            return buf.length
          },
          write(buf, offset, length, position, callback) {
            if (offset !== 0 || length !== buf.length || position !== null) {
              callback(enosys('Invalid write'))
            } else {
              callback(null, this.writeSync(buf))
            }
          },
        }
      }

      const encoder = new TextEncoder()
      const openForReading = node => {
        const bytes = encoder.encode(node.contents)
        let pos = 0
        return {
          node,
          read(buf, offset, length, position, callback) {
            if (offset !== 0 || length !== buf.length || position !== null) {
              callback(enosys('Invalid read'))
            } else {
              const count = Math.max(0, Math.min(length, bytes.length - pos))
              buf.set(bytes.subarray(pos, pos + count), offset)
              pos += count
              callback(null, count)
            }
          },
        }
      }

      fs.mkdir('/dev')
      fds.set(1, openForWriting(fs.mkfile('/dev/stdout')))
      fds.set(2, openForWriting(fs.mkfile('/dev/stderr')))

      return {
        constants: {
          O_WRONLY: 0x0001,
          O_RDWR: 0x0002,
          O_CREAT: 0x0200,
          O_TRUNC: 0x0400,
          O_APPEND: 0x0008,
          O_EXCL: 0x0800,
        },
        writeSync(fd, buf) {
          return fds.get(fd).writeSync(buf)
        },
        write(fd, buf, offset, length, position, callback) {
          fds.get(fd).write(buf, offset, length, position, callback)
        },
        close(fd, callback) {
          fds.delete(fd)
          callback(null)
        },
        open(path, flags, mode, callback) {
          if (flags === (this.constants.O_WRONLY | this.constants.O_CREAT | this.constants.O_TRUNC)) {
            let node
            try { node = fs.mkfile(path) }
            catch (e) { return callback(enosys(`Already exists: ${path}`)) }
            const fd = nextFd++
            fds.set(fd, openForWriting(node))
            return callback(null, fd)
          }
          if (flags === 0) {
            const node = fs.find(path)
            if (!node) return callback(enosys(`Does not exist: ${path}`))
            const fd = nextFd++
            fds.set(fd, node.dirEntries ? { node } : openForReading(node))
            return callback(null, fd)
          }
          return callback(enosys('Not implemented'))
        },
        read(fd, buffer, offset, length, position, callback) {
          fds.get(fd).read(buffer, offset, length, position, callback)
        },
        stat(path, callback) {
          const node = fs.find(path)
          if (!node) return callback(enosys(`Does not exist: ${path}`))
          callback(null, wrapStat(!!node.dirEntries))
        },
        lstat(path, callback) {
          const node = fs.find(path)
          if (!node) return callback(enosys(`Does not exist: ${path}`))
          callback(null, wrapStat(!!node.dirEntries))
        },
        fstat(fd, callback) {
          const { node } = fds.get(fd)
          callback(null, wrapStat(!!node.dirEntries))
        },
        readdir(path, callback) {
          const node = fs.find(path)
          if (!node || !node.dirEntries) return callback(enosys(`Not a directory: ${path}`))
          callback(null, Object.keys(node.dirEntries).sort())
        },
      }
    }
  }
}
