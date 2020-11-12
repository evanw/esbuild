import crypto = require('crypto');
import path = require('path');
import fs = require('fs');
import os = require('os');

// This is a way to serialize concurrent file write and delete operations such
// that no two file operations for the same path are run concurrently. This is
// necessary to ensure the integrity of the file system cache. Set the file
// contents to null to delete the file.
let asyncFileWriter = (): (target: string, contents: string | null, callback?: () => void) => void => {
  type Update = { contents: string | null | undefined, callbacks: (() => void)[] };
  let updates = new Map<string, Update>();
  let tempDir = os.tmpdir();

  let apply = (target: string, contents: string | null, callbacks: (() => void)[]): void => {
    let after = () => {
      updates.delete(target);

      // Re-apply the latest change if another one happened in the meantime
      if (update.contents !== void 0) {
        apply(target, update.contents, update.callbacks);
      }

      // Notify everything that was waiting on this operation when it started
      for (const callback of callbacks) callback();
    };

    // Allow future concurrent updates to communicate with this one
    let update: Update = { contents: void 0, callbacks: [] };
    updates.set(target, update);

    if (contents === null) {
      // Remove the file
      fs.unlink(target, after);
    } else {
      // Update the file
      fs.mkdir(path.dirname(target), { recursive: true }, err => {
        if (err) return;

        // Use a temporary path to ensure the file write is atomic
        let tempPath = path.join(tempDir, 'esbuild.cache' + Math.random().toString(36).slice(1));
        fs.writeFile(tempPath, contents, err => {
          if (err) return;
          fs.rename(tempPath, target, after);
        });
      });
    }
  };

  return (target, contents, callback) => {
    // If a change is already in progress, tell that change to re-apply later
    let update = updates.get(target);
    if (update) {
      update.contents = contents;
      if (callback) update.callbacks.push(callback);
    } else {
      apply(target, contents, callback ? [callback] : []);
    }
  };
};

// Use a single shared file writer so all mutations for a file are serialized.
let fileWriter = asyncFileWriter();

type LRUCache = (key: string, value: number) => void;

// This is a LRU cache with a string key and a numeric value. When too many
// keys are added, "evict" is called for the key with the lowest value.
let lruCache = (limit: number, evict: (key: string) => void): LRUCache => {
  let keys: string[] = [];
  let values: number[] = [];

  return (key, value) => {
    // Remove this key if it already exists
    let existingIndex = keys.indexOf(key);
    if (existingIndex >= 0) {
      keys.splice(existingIndex, 1);
      values.splice(existingIndex, 1);
    }

    // Scan to find the insertion point
    let insertionIndex = 0;
    while (insertionIndex < values.length && values[insertionIndex] > value) {
      insertionIndex++;
    }
    keys.splice(insertionIndex, 0, key);
    values.splice(insertionIndex, 0, value);

    // Evict the oldest entry to stay under the limit
    if (keys.length > limit) {
      evict(keys[limit]);
      keys.pop();
      values.pop();
    }
  };
};

// Try to read the JSON contents of the provided path. Return "undefined" if
// the operation didn't succeed.
let loadFromCache = (cachePath: string, callback: (json?: any) => void): void => {
  fs.readFile(cachePath, 'utf8', (err, json) => {
    if (err) return callback();
    try {
      json = JSON.parse(json);
    } catch {
      return callback();
    }
    callback(json);
  });
};

// This can be used to memoize the fallback function, which is expected to
// return a JSON-serializable object. The key is an array of buffers which
// will all be hashed.
export let fsCache = (cacheDir: string) => {
  // Reset the cache tracking between builds. It replicates some file system
  // state in memory for speed, but we don't want to get too far out of sync
  // with the underlying file system in case other things are mutating the
  // cache directory at the same time.
  let cachePromiseForDir = new Map<string, Promise<LRUCache>>();

  // Get the LRU cache for the provided directory. A cache will be automatically
  // generated if it doesn't already exist, and will be pre-filled with existing
  // directory entries.
  let lruCacheForDir = (dir: string, callback: (cache: LRUCache) => void) => {
    let promise = cachePromiseForDir.get(dir);
    if (!promise) {
      promise = new Promise(resolve => {
        fs.readdir(dir, (err, entries) => {
          // The key space is sharded into 256 directories. Of those, limit each
          // directory to at most 64 files. That means the cache can hold at most
          // 16,384 files.
          let cache = lruCache(64, cachePath => fileWriter(cachePath, null));
          if (err) return resolve(cache);
          let count = entries.length + 1;
          let done = () => {
            if (--count === 0) resolve(cache);
          };
          for (let name of entries) {
            let cachePath = path.join(dir, name);
            fs.stat(cachePath, (err, stats) => {
              if (!err) cache(cachePath, stats.mtime.getTime());
              done();
            });
          }
          done();
        });
      });
      cachePromiseForDir.set(dir, promise);
    }
    return promise.then(callback);
  };

  // Mark the provided path as recently used. This will remove the oldest
  // directory entry if the directory size is at the limit.
  let updateLRU = (cachePath: string, mtime: number) => {
    lruCacheForDir(path.dirname(cachePath), cache => {
      cache(cachePath, mtime);
    });
  };

  return (key: (string | Uint8Array)[], fallback: () => any): Promise<any> =>
    new Promise((resolve, reject) => {
      // Compute the hash of the key
      let hash = crypto.createHash('sha1');
      hash.on('readable', () => {
        let data = hash.read();
        if (data) {
          // Translate the hash into a directory path
          let hex = data.toString('hex');
          let cachePath = path.join(cacheDir, hex.slice(0, 2), hex.slice(2))
          loadFromCache(cachePath, json => {
            if (json !== void 0) {
              // Cache hit: return the value from the cache. Mark this entry as
              // recently used so it's kept around.
              let mtime = new Date;
              fs.utimes(cachePath, mtime, mtime, () => {
                updateLRU(cachePath, mtime.getTime());
              });
              resolve(json);
            } else {
              // Cache miss: call the fallback function. Use "async" to handle
              // "fallback" returning a promise.
              (async () => fallback())().then(json => {
                // Write to the cache for next time
                fileWriter(cachePath, JSON.stringify(json), () => {
                  updateLRU(cachePath, new Date().getTime());
                });
                resolve(json);
              }, reject);
            }
          });
        }
      });
      let isFirst = true;
      for (let part of key) {
        if (!isFirst) hash.write('\0');
        hash.write(part);
        isFirst = false;
      }
      hash.end();
    });
};
