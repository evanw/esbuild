(function(root, factory) {
  if (typeof define === "function" && define.amd) {
    define(factory);
  } else if (typeof exports === "object") {
    module.exports = factory();
  } else {
    root.Lazy = factory();
  }
})(this, function(context) {
  function Lazy(source) {
    if (source instanceof Array) {
      return new ArrayWrapper(source);
    } else if (typeof source === "string") {
      return new StringWrapper(source);
    } else if (source instanceof Sequence) {
      return source;
    }
    if (Lazy.extensions) {
      var extensions = Lazy.extensions, length = extensions.length, result;
      while (!result && length--) {
        result = extensions[length](source);
      }
      if (result) {
        return result;
      }
    }
    return new ObjectWrapper(source);
  }
  Lazy.VERSION = "0.4.3";
  Lazy.noop = function noop() {
  };
  Lazy.identity = function identity(x) {
    return x;
  };
  Lazy.strict = function strict() {
    function StrictLazy(source) {
      if (source == null) {
        throw new Error("You cannot wrap null or undefined using Lazy.");
      }
      if (typeof source === "number" || typeof source === "boolean") {
        throw new Error("You cannot wrap primitive values using Lazy.");
      }
      return Lazy(source);
    }
    ;
    Lazy(Lazy).each(function(property, name) {
      StrictLazy[name] = property;
    });
    return StrictLazy;
  };
  function Sequence() {
  }
  Sequence.define = function define(methodName, overrides) {
    if (!overrides || !overrides.getIterator && !overrides.each) {
      throw new Error("A custom sequence must implement *at least* getIterator or each!");
    }
    return defineSequenceType(Sequence, methodName, overrides);
  };
  Sequence.prototype.size = function size() {
    return this.getIndex().length();
  };
  Sequence.prototype.getIterator = function getIterator() {
    return new Iterator(this);
  };
  Sequence.prototype.root = function root() {
    return this.parent.root();
  };
  Sequence.prototype.isAsync = function isAsync() {
    return this.parent ? this.parent.isAsync() : false;
  };
  Sequence.prototype.value = function value() {
    return this.toArray();
  };
  Sequence.prototype.apply = function apply(source) {
    var root = this.root(), previousSource = root.source, result;
    try {
      root.source = source;
      result = this.value();
    } finally {
      root.source = previousSource;
    }
    return result;
  };
  function Iterator(sequence) {
    this.sequence = sequence;
    this.index = -1;
  }
  Iterator.prototype.current = function current() {
    return this.cachedIndex && this.cachedIndex.get(this.index);
  };
  Iterator.prototype.moveNext = function moveNext() {
    var cachedIndex = this.cachedIndex;
    if (!cachedIndex) {
      cachedIndex = this.cachedIndex = this.sequence.getIndex();
    }
    if (this.index >= cachedIndex.length() - 1) {
      return false;
    }
    ++this.index;
    return true;
  };
  Sequence.prototype.toArray = function toArray() {
    return this.reduce(function(arr, element) {
      arr.push(element);
      return arr;
    }, []);
  };
  Sequence.prototype.getIndex = function getIndex() {
    return new ArrayWrapper(this.toArray());
  };
  Sequence.prototype.get = function get(i) {
    var element;
    this.each(function(e, index) {
      if (index === i) {
        element = e;
        return false;
      }
    });
    return element;
  };
  Sequence.prototype.memoize = function memoize() {
    return new MemoizedSequence(this);
  };
  function MemoizedSequence(parent) {
    this.parent = parent;
  }
  Sequence.prototype.toObject = function toObject() {
    return this.reduce(function(object, pair) {
      object[pair[0]] = pair[1];
      return object;
    }, {});
  };
  Sequence.prototype.each = function each(fn) {
    var iterator = this.getIterator(), i = -1;
    while (iterator.moveNext()) {
      if (fn(iterator.current(), ++i) === false) {
        return false;
      }
    }
    return true;
  };
  Sequence.prototype.forEach = function forEach(fn) {
    return this.each(fn);
  };
  Sequence.prototype.map = function map(mapFn) {
    return new MappedSequence(this, createCallback(mapFn));
  };
  Sequence.prototype.collect = function collect(mapFn) {
    return this.map(mapFn);
  };
  function MappedSequence(parent, mapFn) {
    this.parent = parent;
    this.mapFn = mapFn;
  }
  MappedSequence.prototype = new Sequence();
  MappedSequence.prototype.getIterator = function getIterator() {
    return new MappingIterator(this.parent, this.mapFn);
  };
  MappedSequence.prototype.each = function each(fn) {
    var mapFn = this.mapFn;
    return this.parent.each(function(e, i) {
      return fn(mapFn(e, i), i);
    });
  };
  function MappingIterator(sequence, mapFn) {
    this.iterator = sequence.getIterator();
    this.mapFn = mapFn;
    this.index = -1;
  }
  MappingIterator.prototype.current = function current() {
    return this.mapFn(this.iterator.current(), this.index);
  };
  MappingIterator.prototype.moveNext = function moveNext() {
    if (this.iterator.moveNext()) {
      ++this.index;
      return true;
    }
    return false;
  };
  Sequence.prototype.pluck = function pluck(property) {
    return this.map(property);
  };
  Sequence.prototype.invoke = function invoke(methodName) {
    return this.map(function(e) {
      return e[methodName]();
    });
  };
  Sequence.prototype.filter = function filter(filterFn) {
    return new FilteredSequence(this, createCallback(filterFn));
  };
  Sequence.prototype.select = function select(filterFn) {
    return this.filter(filterFn);
  };
  function FilteredSequence(parent, filterFn) {
    this.parent = parent;
    this.filterFn = filterFn;
  }
  FilteredSequence.prototype = new Sequence();
  FilteredSequence.prototype.getIterator = function getIterator() {
    return new FilteringIterator(this.parent, this.filterFn);
  };
  FilteredSequence.prototype.each = function each(fn) {
    var filterFn = this.filterFn, j = 0;
    return this.parent.each(function(e, i) {
      if (filterFn(e, i)) {
        return fn(e, j++);
      }
    });
  };
  FilteredSequence.prototype.reverse = function reverse() {
    return this.parent.reverse().filter(this.filterFn);
  };
  function FilteringIterator(sequence, filterFn) {
    this.iterator = sequence.getIterator();
    this.filterFn = filterFn;
    this.index = 0;
  }
  FilteringIterator.prototype.current = function current() {
    return this.value;
  };
  FilteringIterator.prototype.moveNext = function moveNext() {
    var iterator = this.iterator, filterFn = this.filterFn, value;
    while (iterator.moveNext()) {
      value = iterator.current();
      if (filterFn(value, this.index++)) {
        this.value = value;
        return true;
      }
    }
    this.value = void 0;
    return false;
  };
  Sequence.prototype.reject = function reject(rejectFn) {
    rejectFn = createCallback(rejectFn);
    return this.filter(function(e) {
      return !rejectFn(e);
    });
  };
  Sequence.prototype.ofType = function ofType(type) {
    return this.filter(function(e) {
      return typeof e === type;
    });
  };
  Sequence.prototype.where = function where(properties) {
    return this.filter(properties);
  };
  Sequence.prototype.reverse = function reverse() {
    return new ReversedSequence(this);
  };
  function ReversedSequence(parent) {
    this.parent = parent;
  }
  ReversedSequence.prototype = new Sequence();
  ReversedSequence.prototype.getIterator = function getIterator() {
    return new ReversedIterator(this.parent);
  };
  function ReversedIterator(sequence) {
    this.sequence = sequence;
  }
  ReversedIterator.prototype.current = function current() {
    return this.getIndex().get(this.index);
  };
  ReversedIterator.prototype.moveNext = function moveNext() {
    var index = this.getIndex(), length = index.length();
    if (typeof this.index === "undefined") {
      this.index = length;
    }
    return --this.index >= 0;
  };
  ReversedIterator.prototype.getIndex = function getIndex() {
    if (!this.cachedIndex) {
      this.cachedIndex = this.sequence.getIndex();
    }
    return this.cachedIndex;
  };
  Sequence.prototype.concat = function concat(var_args) {
    return new ConcatenatedSequence(this, (__get_arraySlice__()).call(arguments, 0));
  };
  function ConcatenatedSequence(parent, arrays) {
    this.parent = parent;
    this.arrays = arrays;
  }
  ConcatenatedSequence.prototype = new Sequence();
  ConcatenatedSequence.prototype.each = function each(fn) {
    var done = false, i = 0;
    this.parent.each(function(e) {
      if (fn(e, i++) === false) {
        done = true;
        return false;
      }
    });
    if (!done) {
      Lazy(this.arrays).flatten().each(function(e) {
        if (fn(e, i++) === false) {
          return false;
        }
      });
    }
  };
  Sequence.prototype.first = function first(count) {
    if (typeof count === "undefined") {
      return getFirst(this);
    }
    return new TakeSequence(this, count);
  };
  Sequence.prototype.head = Sequence.prototype.take = function(count) {
    return this.first(count);
  };
  function TakeSequence(parent, count) {
    this.parent = parent;
    this.count = count;
  }
  TakeSequence.prototype = new Sequence();
  TakeSequence.prototype.getIterator = function getIterator() {
    return new TakeIterator(this.parent, this.count);
  };
  TakeSequence.prototype.each = function each(fn) {
    var count = this.count, i = 0;
    var result;
    var handle = this.parent.each(function(e) {
      if (i < count) {
        result = fn(e, i++);
      }
      if (i >= count) {
        return false;
      }
      return result;
    });
    if (handle instanceof AsyncHandle) {
      return handle;
    }
    return i === count && result !== false;
  };
  function TakeIterator(sequence, count) {
    this.iterator = sequence.getIterator();
    this.count = count;
  }
  TakeIterator.prototype.current = function current() {
    return this.iterator.current();
  };
  TakeIterator.prototype.moveNext = function moveNext() {
    return --this.count >= 0 && this.iterator.moveNext();
  };
  Sequence.prototype.takeWhile = function takeWhile(predicate) {
    return new TakeWhileSequence(this, predicate);
  };
  function TakeWhileSequence(parent, predicate) {
    this.parent = parent;
    this.predicate = predicate;
  }
  TakeWhileSequence.prototype = new Sequence();
  TakeWhileSequence.prototype.each = function each(fn) {
    var predicate = this.predicate, finished = false, j = 0;
    var result = this.parent.each(function(e, i) {
      if (!predicate(e, i)) {
        finished = true;
        return false;
      }
      return fn(e, j++);
    });
    if (result instanceof AsyncHandle) {
      return result;
    }
    return finished;
  };
  Sequence.prototype.initial = function initial(count) {
    return new InitialSequence(this, count);
  };
  function InitialSequence(parent, count) {
    this.parent = parent;
    this.count = typeof count === "number" ? count : 1;
  }
  InitialSequence.prototype = new Sequence();
  InitialSequence.prototype.each = function each(fn) {
    var index = this.parent.getIndex();
    return index.take(index.length() - this.count).each(fn);
  };
  Sequence.prototype.last = function last(count) {
    if (typeof count === "undefined") {
      return this.reverse().first();
    }
    return this.reverse().take(count).reverse();
  };
  Sequence.prototype.findWhere = function findWhere(properties) {
    return this.where(properties).first();
  };
  Sequence.prototype.rest = function rest(count) {
    return new DropSequence(this, count);
  };
  Sequence.prototype.skip = Sequence.prototype.tail = Sequence.prototype.drop = function drop(count) {
    return this.rest(count);
  };
  function DropSequence(parent, count) {
    this.parent = parent;
    this.count = typeof count === "number" ? count : 1;
  }
  DropSequence.prototype = new Sequence();
  DropSequence.prototype.each = function each(fn) {
    var count = this.count, dropped = 0, i = 0;
    return this.parent.each(function(e) {
      if (dropped++ < count) {
        return;
      }
      return fn(e, i++);
    });
  };
  Sequence.prototype.dropWhile = function dropWhile(predicate) {
    return new DropWhileSequence(this, predicate);
  };
  Sequence.prototype.skipWhile = function skipWhile(predicate) {
    return this.dropWhile(predicate);
  };
  function DropWhileSequence(parent, predicate) {
    this.parent = parent;
    this.predicate = predicate;
  }
  DropWhileSequence.prototype = new Sequence();
  DropWhileSequence.prototype.each = function each(fn) {
    var predicate = this.predicate, done = false;
    return this.parent.each(function(e) {
      if (!done) {
        if (predicate(e)) {
          return;
        }
        done = true;
      }
      return fn(e);
    });
  };
  Sequence.prototype.sort = function sort(sortFn, descending) {
    sortFn || (sortFn = compare);
    if (descending) {
      sortFn = reverseArguments(sortFn);
    }
    return new SortedSequence(this, sortFn);
  };
  Sequence.prototype.sortBy = function sortBy(sortFn, descending) {
    sortFn = createComparator(sortFn);
    if (descending) {
      sortFn = reverseArguments(sortFn);
    }
    return new SortedSequence(this, sortFn);
  };
  function SortedSequence(parent, sortFn) {
    this.parent = parent;
    this.sortFn = sortFn;
  }
  SortedSequence.prototype = new Sequence();
  SortedSequence.prototype.each = function each(fn) {
    var sortFn = this.sortFn, result = this.parent.toArray();
    result.sort(sortFn);
    return forEach(result, fn);
  };
  SortedSequence.prototype.reverse = function reverse() {
    return new SortedSequence(this.parent, reverseArguments(this.sortFn));
  };
  Sequence.prototype.groupBy = function groupBy(keyFn, valFn) {
    return new GroupedSequence(this, keyFn, valFn);
  };
  function GroupedSequence(parent, keyFn, valFn) {
    this.parent = parent;
    this.keyFn = keyFn;
    this.valFn = valFn;
  }
  Sequence.prototype.indexBy = function(keyFn, valFn) {
    return new IndexedSequence(this, keyFn, valFn);
  };
  function IndexedSequence(parent, keyFn, valFn) {
    this.parent = parent;
    this.keyFn = keyFn;
    this.valFn = valFn;
  }
  Sequence.prototype.countBy = function countBy(keyFn) {
    return new CountedSequence(this, keyFn);
  };
  function CountedSequence(parent, keyFn) {
    this.parent = parent;
    this.keyFn = keyFn;
  }
  Sequence.prototype.uniq = function uniq(keyFn) {
    return new UniqueSequence(this, keyFn);
  };
  Sequence.prototype.unique = function unique(keyFn) {
    return this.uniq(keyFn);
  };
  function UniqueSequence(parent, keyFn) {
    this.parent = parent;
    this.keyFn = keyFn;
  }
  UniqueSequence.prototype = new Sequence();
  UniqueSequence.prototype.each = function each(fn) {
    var cache = new Set(), keyFn = this.keyFn, i = 0;
    if (keyFn) {
      keyFn = createCallback(keyFn);
      return this.parent.each(function(e) {
        if (cache.add(keyFn(e))) {
          return fn(e, i++);
        }
      });
    } else {
      return this.parent.each(function(e) {
        if (cache.add(e)) {
          return fn(e, i++);
        }
      });
    }
  };
  Sequence.prototype.zip = function zip(var_args) {
    if (arguments.length === 1) {
      return new SimpleZippedSequence(this, var_args);
    } else {
      return new ZippedSequence(this, (__get_arraySlice__()).call(arguments, 0));
    }
  };
  function ZippedSequence(parent, arrays) {
    this.parent = parent;
    this.arrays = arrays;
  }
  ZippedSequence.prototype = new Sequence();
  ZippedSequence.prototype.each = function each(fn) {
    var arrays = this.arrays, i = 0;
    var iteratedLeft = this.parent.each(function(e) {
      var group = [e];
      for (var j = 0; j < arrays.length; ++j) {
        group.push(arrays[j][i]);
      }
      return fn(group, i++);
    });
    if (!iteratedLeft) {
      return false;
    }
    var group, keepGoing = true;
    while (keepGoing) {
      keepGoing = false;
      group = [void 0];
      for (var j = 0; j < arrays.length; ++j) {
        group.push(arrays[j][i]);
        if (arrays[j].length > i) {
          keepGoing = true;
        }
      }
      if (keepGoing && fn(group, i++) === false) {
        return false;
      }
    }
    return true;
  };
  Sequence.prototype.shuffle = function shuffle() {
    return new ShuffledSequence(this);
  };
  function ShuffledSequence(parent) {
    this.parent = parent;
  }
  ShuffledSequence.prototype = new Sequence();
  ShuffledSequence.prototype.each = function each(fn) {
    var shuffled = this.parent.toArray(), floor = Math.floor, random = Math.random, j = 0;
    for (var i = shuffled.length - 1; i > 0; --i) {
      swap(shuffled, i, floor(random() * (i + 1)));
      if (fn(shuffled[i], j++) === false) {
        return false;
      }
    }
    if (shuffled.length) {
      fn(shuffled[0], j);
    }
    return true;
  };
  Sequence.prototype.flatten = function flatten() {
    return new FlattenedSequence(this);
  };
  function FlattenedSequence(parent) {
    this.parent = parent;
  }
  FlattenedSequence.prototype = new Sequence();
  FlattenedSequence.prototype.each = function each(fn) {
    var index = 0;
    return this.parent.each(function recurseVisitor(e) {
      if (e instanceof Array) {
        return forEach(e, recurseVisitor);
      }
      if (e instanceof Sequence) {
        return e.each(recurseVisitor);
      }
      return fn(e, index++);
    });
  };
  Sequence.prototype.compact = function compact() {
    return this.filter(function(e) {
      return !!e;
    });
  };
  Sequence.prototype.without = function without(var_args) {
    return new WithoutSequence(this, (__get_arraySlice__()).call(arguments, 0));
  };
  Sequence.prototype.difference = function difference(var_args) {
    return this.without.apply(this, arguments);
  };
  function WithoutSequence(parent, values) {
    this.parent = parent;
    this.values = values;
  }
  WithoutSequence.prototype = new Sequence();
  WithoutSequence.prototype.each = function each(fn) {
    var set = createSet(this.values), i = 0;
    return this.parent.each(function(e) {
      if (!set.contains(e)) {
        return fn(e, i++);
      }
    });
  };
  Sequence.prototype.union = function union(var_args) {
    return this.concat(var_args).uniq();
  };
  Sequence.prototype.intersection = function intersection(var_args) {
    if (arguments.length === 1 && arguments[0] instanceof Array) {
      return new SimpleIntersectionSequence(this, var_args);
    } else {
      return new IntersectionSequence(this, (__get_arraySlice__()).call(arguments, 0));
    }
  };
  function IntersectionSequence(parent, arrays) {
    this.parent = parent;
    this.arrays = arrays;
  }
  IntersectionSequence.prototype = new Sequence();
  IntersectionSequence.prototype.each = function each(fn) {
    var sets = Lazy(this.arrays).map(function(values) {
      return new UniqueMemoizer(Lazy(values).getIterator());
    });
    var setIterator = new UniqueMemoizer(sets.getIterator()), i = 0;
    return this.parent.uniq().each(function(e) {
      var includedInAll = true;
      setIterator.each(function(set) {
        if (!set.contains(e)) {
          includedInAll = false;
          return false;
        }
      });
      if (includedInAll) {
        return fn(e, i++);
      }
    });
  };
  function UniqueMemoizer(iterator) {
    this.iterator = iterator;
    this.set = new Set();
    this.memo = [];
    this.currentValue = void 0;
  }
  UniqueMemoizer.prototype.current = function current() {
    return this.currentValue;
  };
  UniqueMemoizer.prototype.moveNext = function moveNext() {
    var iterator = this.iterator, set = this.set, memo = this.memo, current;
    while (iterator.moveNext()) {
      current = iterator.current();
      if (set.add(current)) {
        memo.push(current);
        this.currentValue = current;
        return true;
      }
    }
    return false;
  };
  UniqueMemoizer.prototype.each = function each(fn) {
    var memo = this.memo, length = memo.length, i = -1;
    while (++i < length) {
      if (fn(memo[i], i) === false) {
        return false;
      }
    }
    while (this.moveNext()) {
      if (fn(this.currentValue, i++) === false) {
        break;
      }
    }
  };
  UniqueMemoizer.prototype.contains = function contains(e) {
    if (this.set.contains(e)) {
      return true;
    }
    while (this.moveNext()) {
      if (this.currentValue === e) {
        return true;
      }
    }
    return false;
  };
  Sequence.prototype.every = function every(predicate) {
    predicate = createCallback(predicate);
    return this.each(function(e, i) {
      return !!predicate(e, i);
    });
  };
  Sequence.prototype.all = function all(predicate) {
    return this.every(predicate);
  };
  Sequence.prototype.some = function some(predicate) {
    predicate = createCallback(predicate, true);
    var success = false;
    this.each(function(e) {
      if (predicate(e)) {
        success = true;
        return false;
      }
    });
    return success;
  };
  Sequence.prototype.any = function any(predicate) {
    return this.some(predicate);
  };
  Sequence.prototype.none = function none(predicate) {
    return !this.any(predicate);
  };
  Sequence.prototype.isEmpty = function isEmpty() {
    return !this.any();
  };
  Sequence.prototype.indexOf = function indexOf(value) {
    var foundIndex = -1;
    this.each(function(e, i) {
      if (e === value) {
        foundIndex = i;
        return false;
      }
    });
    return foundIndex;
  };
  Sequence.prototype.lastIndexOf = function lastIndexOf(value) {
    var reversed = this.getIndex().reverse(), index = reversed.indexOf(value);
    if (index !== -1) {
      index = reversed.length() - index - 1;
    }
    return index;
  };
  Sequence.prototype.sortedIndex = function sortedIndex(value) {
    var indexed = this.getIndex(), lower = 0, upper = indexed.length(), i;
    while (lower < upper) {
      i = lower + upper >>> 1;
      if (compare(indexed.get(i), value) === -1) {
        lower = i + 1;
      } else {
        upper = i;
      }
    }
    return lower;
  };
  Sequence.prototype.contains = function contains(value) {
    return this.indexOf(value) !== -1;
  };
  Sequence.prototype.reduce = function reduce(aggregator, memo) {
    if (arguments.length < 2) {
      return this.tail().reduce(aggregator, this.head());
    }
    var eachResult = this.each(function(e, i) {
      memo = aggregator(memo, e, i);
    });
    if (eachResult instanceof AsyncHandle) {
      return eachResult.then(function() {
        return memo;
      });
    }
    return memo;
  };
  Sequence.prototype.inject = Sequence.prototype.foldl = function foldl(aggregator, memo) {
    return this.reduce(aggregator, memo);
  };
  Sequence.prototype.reduceRight = function reduceRight(aggregator, memo) {
    if (arguments.length < 2) {
      return this.initial(1).reduceRight(aggregator, this.last());
    }
    var indexed = this.getIndex(), i = indexed.length() - 1;
    return indexed.reverse().reduce(function(m, e) {
      return aggregator(m, e, i--);
    }, memo);
  };
  Sequence.prototype.foldr = function foldr(aggregator, memo) {
    return this.reduceRight(aggregator, memo);
  };
  Sequence.prototype.consecutive = function consecutive(count) {
    var queue = new Queue(count);
    var segments = this.map(function(element) {
      if (queue.add(element).count === count) {
        return queue.toArray();
      }
    });
    return segments.compact();
  };
  Sequence.prototype.chunk = function chunk(size) {
    if (size < 1) {
      throw new Error("You must specify a positive chunk size.");
    }
    return new ChunkedSequence(this, size);
  };
  function ChunkedSequence(parent, size) {
    this.parent = parent;
    this.chunkSize = size;
  }
  ChunkedSequence.prototype = new Sequence();
  ChunkedSequence.prototype.getIterator = function getIterator() {
    return new ChunkedIterator(this.parent, this.chunkSize);
  };
  function ChunkedIterator(sequence, size) {
    this.iterator = sequence.getIterator();
    this.size = size;
  }
  ChunkedIterator.prototype.current = function current() {
    return this.currentChunk;
  };
  ChunkedIterator.prototype.moveNext = function moveNext() {
    var iterator = this.iterator, chunkSize = this.size, chunk = [];
    while (chunk.length < chunkSize && iterator.moveNext()) {
      chunk.push(iterator.current());
    }
    if (chunk.length === 0) {
      return false;
    }
    this.currentChunk = chunk;
    return true;
  };
  Sequence.prototype.tap = function tap(callback) {
    return new TappedSequence(this, callback);
  };
  function TappedSequence(parent, callback) {
    this.parent = parent;
    this.callback = callback;
  }
  TappedSequence.prototype = new Sequence();
  TappedSequence.prototype.each = function each(fn) {
    var callback = this.callback;
    return this.parent.each(function(e, i) {
      callback(e, i);
      return fn(e, i);
    });
  };
  Sequence.prototype.find = function find(predicate) {
    return this.filter(predicate).first();
  };
  Sequence.prototype.detect = function detect(predicate) {
    return this.find(predicate);
  };
  Sequence.prototype.min = function min(valueFn) {
    if (typeof valueFn !== "undefined") {
      return this.minBy(valueFn);
    }
    return this.reduce(function(prev, current, i) {
      if (typeof prev === "undefined") {
        return current;
      }
      return current < prev ? current : prev;
    });
  };
  Sequence.prototype.minBy = function minBy(valueFn) {
    valueFn = createCallback(valueFn);
    return this.reduce(function(x, y) {
      return valueFn(y) < valueFn(x) ? y : x;
    });
  };
  Sequence.prototype.max = function max(valueFn) {
    if (typeof valueFn !== "undefined") {
      return this.maxBy(valueFn);
    }
    return this.reduce(function(prev, current, i) {
      if (typeof prev === "undefined") {
        return current;
      }
      return current > prev ? current : prev;
    });
  };
  Sequence.prototype.maxBy = function maxBy(valueFn) {
    valueFn = createCallback(valueFn);
    return this.reduce(function(x, y) {
      return valueFn(y) > valueFn(x) ? y : x;
    });
  };
  Sequence.prototype.sum = function sum(valueFn) {
    if (typeof valueFn !== "undefined") {
      return this.sumBy(valueFn);
    }
    return this.reduce(function(x, y) {
      return x + y;
    }, 0);
  };
  Sequence.prototype.sumBy = function sumBy(valueFn) {
    valueFn = createCallback(valueFn);
    return this.reduce(function(x, y) {
      return x + valueFn(y);
    }, 0);
  };
  Sequence.prototype.join = function join(delimiter) {
    delimiter = typeof delimiter === "undefined" ? "," : String(delimiter);
    return this.reduce(function(str, e, i) {
      if (i > 0) {
        str += delimiter;
      }
      return str + e;
    }, "");
  };
  Sequence.prototype.toString = function toString(delimiter) {
    return this.join(delimiter);
  };
  Sequence.prototype.async = function async(interval) {
    return new AsyncSequence(this, interval);
  };
  function SimpleIntersectionSequence(parent, array) {
    this.parent = parent;
    this.array = array;
    this.each = getEachForIntersection(array);
  }
  SimpleIntersectionSequence.prototype = new Sequence();
  SimpleIntersectionSequence.prototype.eachMemoizerCache = function eachMemoizerCache(fn) {
    var iterator = new UniqueMemoizer(Lazy(this.array).getIterator()), i = 0;
    return this.parent.uniq().each(function(e) {
      if (iterator.contains(e)) {
        return fn(e, i++);
      }
    });
  };
  SimpleIntersectionSequence.prototype.eachArrayCache = function eachArrayCache(fn) {
    var array = this.array, find = arrayContains, i = 0;
    return this.parent.uniq().each(function(e) {
      if (find(array, e)) {
        return fn(e, i++);
      }
    });
  };
  function getEachForIntersection(source) {
    if (source.length < 40) {
      return SimpleIntersectionSequence.prototype.eachArrayCache;
    } else {
      return SimpleIntersectionSequence.prototype.eachMemoizerCache;
    }
  }
  function SimpleZippedSequence(parent, array) {
    this.parent = parent;
    this.array = array;
  }
  SimpleZippedSequence.prototype = new Sequence();
  SimpleZippedSequence.prototype.each = function each(fn) {
    var array = this.array, i = -1;
    var iteratedLeft = this.parent.each(function(e) {
      ++i;
      return fn([e, array[i]], i);
    });
    if (!iteratedLeft) {
      return false;
    }
    while (++i < array.length) {
      if (fn([void 0, array[i]], i) === false) {
        return false;
      }
    }
    return true;
  };
  function ArrayLikeSequence() {
  }
  ArrayLikeSequence.prototype = new Sequence();
  ArrayLikeSequence.define = function define(methodName, overrides) {
    if (!overrides || typeof overrides.get !== "function") {
      throw new Error("A custom array-like sequence must implement *at least* get!");
    }
    return defineSequenceType(ArrayLikeSequence, methodName, overrides);
  };
  ArrayLikeSequence.prototype.get = function get(i) {
    return this.parent.get(i);
  };
  ArrayLikeSequence.prototype.length = function length() {
    return this.parent.length();
  };
  ArrayLikeSequence.prototype.getIndex = function getIndex() {
    return this;
  };
  ArrayLikeSequence.prototype.getIterator = function getIterator() {
    return new IndexedIterator(this);
  };
  function IndexedIterator(sequence) {
    this.sequence = sequence;
    this.index = -1;
  }
  IndexedIterator.prototype.current = function current() {
    return this.sequence.get(this.index);
  };
  IndexedIterator.prototype.moveNext = function moveNext() {
    if (this.index >= this.sequence.length() - 1) {
      return false;
    }
    ++this.index;
    return true;
  };
  ArrayLikeSequence.prototype.each = function each(fn) {
    var length = this.length(), i = -1;
    while (++i < length) {
      if (fn(this.get(i), i) === false) {
        return false;
      }
    }
    return true;
  };
  ArrayLikeSequence.prototype.push = function push(value) {
    return this.concat([value]);
  };
  ArrayLikeSequence.prototype.pop = function pop() {
    return this.initial();
  };
  ArrayLikeSequence.prototype.unshift = function unshift(value) {
    return Lazy([value]).concat(this);
  };
  ArrayLikeSequence.prototype.shift = function shift() {
    return this.drop();
  };
  ArrayLikeSequence.prototype.slice = function slice(begin, end) {
    var length = this.length();
    if (begin < 0) {
      begin = length + begin;
    }
    var result = this.drop(begin);
    if (typeof end === "number") {
      if (end < 0) {
        end = length + end;
      }
      result = result.take(end - begin);
    }
    return result;
  };
  ArrayLikeSequence.prototype.map = function map(mapFn) {
    return new IndexedMappedSequence(this, createCallback(mapFn));
  };
  function IndexedMappedSequence(parent, mapFn) {
    this.parent = parent;
    this.mapFn = mapFn;
  }
  IndexedMappedSequence.prototype = new ArrayLikeSequence();
  IndexedMappedSequence.prototype.get = function get(i) {
    if (i < 0 || i >= this.parent.length()) {
      return void 0;
    }
    return this.mapFn(this.parent.get(i), i);
  };
  ArrayLikeSequence.prototype.filter = function filter(filterFn) {
    return new IndexedFilteredSequence(this, createCallback(filterFn));
  };
  function IndexedFilteredSequence(parent, filterFn) {
    this.parent = parent;
    this.filterFn = filterFn;
  }
  IndexedFilteredSequence.prototype = new FilteredSequence();
  IndexedFilteredSequence.prototype.each = function each(fn) {
    var parent = this.parent, filterFn = this.filterFn, length = this.parent.length(), i = -1, j = 0, e;
    while (++i < length) {
      e = parent.get(i);
      if (filterFn(e, i) && fn(e, j++) === false) {
        return false;
      }
    }
    return true;
  };
  ArrayLikeSequence.prototype.reverse = function reverse() {
    return new IndexedReversedSequence(this);
  };
  function IndexedReversedSequence(parent) {
    this.parent = parent;
  }
  IndexedReversedSequence.prototype = new ArrayLikeSequence();
  IndexedReversedSequence.prototype.get = function get(i) {
    return this.parent.get(this.length() - i - 1);
  };
  ArrayLikeSequence.prototype.first = function first(count) {
    if (typeof count === "undefined") {
      return this.get(0);
    }
    return new IndexedTakeSequence(this, count);
  };
  function IndexedTakeSequence(parent, count) {
    this.parent = parent;
    this.count = count;
  }
  IndexedTakeSequence.prototype = new ArrayLikeSequence();
  IndexedTakeSequence.prototype.length = function length() {
    var parentLength = this.parent.length();
    return this.count <= parentLength ? this.count : parentLength;
  };
  ArrayLikeSequence.prototype.rest = function rest(count) {
    return new IndexedDropSequence(this, count);
  };
  function IndexedDropSequence(parent, count) {
    this.parent = parent;
    this.count = typeof count === "number" ? count : 1;
  }
  IndexedDropSequence.prototype = new ArrayLikeSequence();
  IndexedDropSequence.prototype.get = function get(i) {
    return this.parent.get(this.count + i);
  };
  IndexedDropSequence.prototype.length = function length() {
    var parentLength = this.parent.length();
    return this.count <= parentLength ? parentLength - this.count : 0;
  };
  ArrayLikeSequence.prototype.concat = function concat(var_args) {
    if (arguments.length === 1 && arguments[0] instanceof Array) {
      return new IndexedConcatenatedSequence(this, var_args);
    } else {
      return Sequence.prototype.concat.apply(this, arguments);
    }
  };
  function IndexedConcatenatedSequence(parent, other) {
    this.parent = parent;
    this.other = other;
  }
  IndexedConcatenatedSequence.prototype = new ArrayLikeSequence();
  IndexedConcatenatedSequence.prototype.get = function get(i) {
    var parentLength = this.parent.length();
    if (i < parentLength) {
      return this.parent.get(i);
    } else {
      return this.other[i - parentLength];
    }
  };
  IndexedConcatenatedSequence.prototype.length = function length() {
    return this.parent.length() + this.other.length;
  };
  ArrayLikeSequence.prototype.uniq = function uniq(keyFn) {
    return new IndexedUniqueSequence(this, createCallback(keyFn));
  };
  function IndexedUniqueSequence(parent, keyFn) {
    this.parent = parent;
    this.each = getEachForParent(parent);
    this.keyFn = keyFn;
  }
  IndexedUniqueSequence.prototype = new Sequence();
  IndexedUniqueSequence.prototype.eachArrayCache = function eachArrayCache(fn) {
    var parent = this.parent, keyFn = this.keyFn, length = parent.length(), cache = [], find = arrayContains, key, value, i = -1, j = 0;
    while (++i < length) {
      value = parent.get(i);
      key = keyFn(value);
      if (!find(cache, key)) {
        cache.push(key);
        if (fn(value, j++) === false) {
          return false;
        }
      }
    }
  };
  IndexedUniqueSequence.prototype.eachSetCache = UniqueSequence.prototype.each;
  function getEachForParent(parent) {
    if (parent.length() < 100) {
      return IndexedUniqueSequence.prototype.eachArrayCache;
    } else {
      return UniqueSequence.prototype.each;
    }
  }
  MemoizedSequence.prototype = new ArrayLikeSequence();
  MemoizedSequence.prototype.cache = function cache() {
    return this.cachedResult || (this.cachedResult = this.parent.toArray());
  };
  MemoizedSequence.prototype.get = function get(i) {
    return this.cache()[i];
  };
  MemoizedSequence.prototype.length = function length() {
    return this.cache().length;
  };
  MemoizedSequence.prototype.slice = function slice(begin, end) {
    return this.cache().slice(begin, end);
  };
  MemoizedSequence.prototype.toArray = function toArray() {
    return this.cache().slice(0);
  };
  function ArrayWrapper(source) {
    this.source = source;
  }
  ArrayWrapper.prototype = new ArrayLikeSequence();
  ArrayWrapper.prototype.root = function root() {
    return this;
  };
  ArrayWrapper.prototype.isAsync = function isAsync() {
    return false;
  };
  ArrayWrapper.prototype.get = function get(i) {
    return this.source[i];
  };
  ArrayWrapper.prototype.length = function length() {
    return this.source.length;
  };
  ArrayWrapper.prototype.each = function each(fn) {
    return forEach(this.source, fn);
  };
  ArrayWrapper.prototype.map = function map(mapFn) {
    return new MappedArrayWrapper(this, createCallback(mapFn));
  };
  ArrayWrapper.prototype.filter = function filter(filterFn) {
    return new FilteredArrayWrapper(this, createCallback(filterFn));
  };
  ArrayWrapper.prototype.uniq = function uniq(keyFn) {
    return new UniqueArrayWrapper(this, keyFn);
  };
  ArrayWrapper.prototype.concat = function concat(var_args) {
    if (arguments.length === 1 && arguments[0] instanceof Array) {
      return new ConcatArrayWrapper(this, var_args);
    } else {
      return ArrayLikeSequence.prototype.concat.apply(this, arguments);
    }
  };
  ArrayWrapper.prototype.toArray = function toArray() {
    return this.source.slice(0);
  };
  function MappedArrayWrapper(parent, mapFn) {
    this.parent = parent;
    this.mapFn = mapFn;
  }
  MappedArrayWrapper.prototype = new ArrayLikeSequence();
  MappedArrayWrapper.prototype.get = function get(i) {
    var source = this.parent.source;
    if (i < 0 || i >= source.length) {
      return void 0;
    }
    return this.mapFn(source[i]);
  };
  MappedArrayWrapper.prototype.length = function length() {
    return this.parent.source.length;
  };
  MappedArrayWrapper.prototype.each = function each(fn) {
    var source = this.parent.source, length = source.length, mapFn = this.mapFn, i = -1;
    while (++i < length) {
      if (fn(mapFn(source[i], i), i) === false) {
        return false;
      }
    }
    return true;
  };
  function FilteredArrayWrapper(parent, filterFn) {
    this.parent = parent;
    this.filterFn = filterFn;
  }
  FilteredArrayWrapper.prototype = new FilteredSequence();
  FilteredArrayWrapper.prototype.each = function each(fn) {
    var source = this.parent.source, filterFn = this.filterFn, length = source.length, i = -1, j = 0, e;
    while (++i < length) {
      e = source[i];
      if (filterFn(e, i) && fn(e, j++) === false) {
        return false;
      }
    }
    return true;
  };
  function UniqueArrayWrapper(parent, keyFn) {
    this.parent = parent;
    this.each = getEachForSource(parent.source);
    this.keyFn = keyFn;
  }
  UniqueArrayWrapper.prototype = new Sequence();
  UniqueArrayWrapper.prototype.eachNoCache = function eachNoCache(fn) {
    var source = this.parent.source, keyFn = this.keyFn, length = source.length, find = arrayContainsBefore, value, i = -1, k = 0;
    while (++i < length) {
      value = source[i];
      if (!find(source, value, i, keyFn) && fn(value, k++) === false) {
        return false;
      }
    }
    return true;
  };
  UniqueArrayWrapper.prototype.eachArrayCache = function eachArrayCache(fn) {
    var source = this.parent.source, keyFn = this.keyFn, length = source.length, cache = [], find = arrayContains, key, value, i = -1, j = 0;
    if (keyFn) {
      keyFn = createCallback(keyFn);
      while (++i < length) {
        value = source[i];
        key = keyFn(value);
        if (!find(cache, key)) {
          cache.push(key);
          if (fn(value, j++) === false) {
            return false;
          }
        }
      }
    } else {
      while (++i < length) {
        value = source[i];
        if (!find(cache, value)) {
          cache.push(value);
          if (fn(value, j++) === false) {
            return false;
          }
        }
      }
    }
    return true;
  };
  UniqueArrayWrapper.prototype.eachSetCache = UniqueSequence.prototype.each;
  function getEachForSource(source) {
    if (source.length < 40) {
      return UniqueArrayWrapper.prototype.eachNoCache;
    } else if (source.length < 100) {
      return UniqueArrayWrapper.prototype.eachArrayCache;
    } else {
      return UniqueArrayWrapper.prototype.eachSetCache;
    }
  }
  function ConcatArrayWrapper(parent, other) {
    this.parent = parent;
    this.other = other;
  }
  ConcatArrayWrapper.prototype = new ArrayLikeSequence();
  ConcatArrayWrapper.prototype.get = function get(i) {
    var source = this.parent.source, sourceLength = source.length;
    if (i < sourceLength) {
      return source[i];
    } else {
      return this.other[i - sourceLength];
    }
  };
  ConcatArrayWrapper.prototype.length = function length() {
    return this.parent.source.length + this.other.length;
  };
  ConcatArrayWrapper.prototype.each = function each(fn) {
    var source = this.parent.source, sourceLength = source.length, other = this.other, otherLength = other.length, i = 0, j = -1;
    while (++j < sourceLength) {
      if (fn(source[j], i++) === false) {
        return false;
      }
    }
    j = -1;
    while (++j < otherLength) {
      if (fn(other[j], i++) === false) {
        return false;
      }
    }
    return true;
  };
  function ObjectLikeSequence() {
  }
  ObjectLikeSequence.prototype = new Sequence();
  ObjectLikeSequence.define = function define(methodName, overrides) {
    if (!overrides || typeof overrides.each !== "function") {
      throw new Error("A custom object-like sequence must implement *at least* each!");
    }
    return defineSequenceType(ObjectLikeSequence, methodName, overrides);
  };
  ObjectLikeSequence.prototype.value = function value() {
    return this.toObject();
  };
  ObjectLikeSequence.prototype.get = function get(key) {
    var pair = this.pairs().find(function(pair) {
      return pair[0] === key;
    });
    return pair ? pair[1] : void 0;
  };
  ObjectLikeSequence.prototype.keys = function keys() {
    return new KeySequence(this);
  };
  function KeySequence(parent) {
    this.parent = parent;
  }
  KeySequence.prototype = new Sequence();
  KeySequence.prototype.each = function each(fn) {
    var i = -1;
    return this.parent.each(function(v, k) {
      return fn(k, ++i);
    });
  };
  ObjectLikeSequence.prototype.values = function values() {
    return new ValuesSequence(this);
  };
  function ValuesSequence(parent) {
    this.parent = parent;
  }
  ValuesSequence.prototype = new Sequence();
  ValuesSequence.prototype.each = function each(fn) {
    var i = -1;
    return this.parent.each(function(v, k) {
      return fn(v, ++i);
    });
  };
  ObjectLikeSequence.prototype.async = function async() {
    throw new Error("An ObjectLikeSequence does not support asynchronous iteration.");
  };
  ObjectLikeSequence.prototype.filter = function filter(filterFn) {
    return new FilteredObjectLikeSequence(this, createCallback(filterFn));
  };
  function FilteredObjectLikeSequence(parent, filterFn) {
    this.parent = parent;
    this.filterFn = filterFn;
  }
  FilteredObjectLikeSequence.prototype = new ObjectLikeSequence();
  FilteredObjectLikeSequence.prototype.each = function each(fn) {
    var filterFn = this.filterFn;
    return this.parent.each(function(v, k) {
      if (filterFn(v, k)) {
        return fn(v, k);
      }
    });
  };
  ObjectLikeSequence.prototype.reverse = function reverse() {
    return this;
  };
  ObjectLikeSequence.prototype.assign = function assign(other) {
    return new AssignSequence(this, other);
  };
  ObjectLikeSequence.prototype.extend = function extend(other) {
    return this.assign(other);
  };
  function AssignSequence(parent, other) {
    this.parent = parent;
    this.other = other;
  }
  AssignSequence.prototype = new ObjectLikeSequence();
  AssignSequence.prototype.get = function get(key) {
    return key in this.other ? this.other[key] : this.parent.get(key);
  };
  AssignSequence.prototype.each = function each(fn) {
    var merged = new Set(), done = false;
    Lazy(this.other).each(function(value, key) {
      if (fn(value, key) === false) {
        done = true;
        return false;
      }
      merged.add(key);
    });
    if (!done) {
      return this.parent.each(function(value, key) {
        if (!merged.contains(key) && fn(value, key) === false) {
          return false;
        }
      });
    }
  };
  ObjectLikeSequence.prototype.defaults = function defaults(defaults) {
    return new DefaultsSequence(this, defaults);
  };
  function DefaultsSequence(parent, defaults) {
    this.parent = parent;
    this.defaults = defaults;
  }
  DefaultsSequence.prototype = new ObjectLikeSequence();
  DefaultsSequence.prototype.get = function get(key) {
    var parentValue = this.parent.get(key);
    return parentValue !== void 0 ? parentValue : this.defaults[key];
  };
  DefaultsSequence.prototype.each = function each(fn) {
    var merged = new Set(), done = false;
    this.parent.each(function(value, key) {
      if (fn(value, key) === false) {
        done = true;
        return false;
      }
      if (typeof value !== "undefined") {
        merged.add(key);
      }
    });
    if (!done) {
      Lazy(this.defaults).each(function(value, key) {
        if (!merged.contains(key) && fn(value, key) === false) {
          return false;
        }
      });
    }
  };
  ObjectLikeSequence.prototype.invert = function invert() {
    return new InvertedSequence(this);
  };
  function InvertedSequence(parent) {
    this.parent = parent;
  }
  InvertedSequence.prototype = new ObjectLikeSequence();
  InvertedSequence.prototype.each = function each(fn) {
    this.parent.each(function(value, key) {
      return fn(key, value);
    });
  };
  ObjectLikeSequence.prototype.merge = function merge(var_args) {
    var mergeFn = arguments.length > 1 && typeof arguments[arguments.length - 1] === "function" ? (__get_arrayPop__()).call(arguments) : null;
    return new MergedSequence(this, (__get_arraySlice__()).call(arguments, 0), mergeFn);
  };
  function MergedSequence(parent, others, mergeFn) {
    this.parent = parent;
    this.others = others;
    this.mergeFn = mergeFn;
  }
  MergedSequence.prototype = new ObjectLikeSequence();
  MergedSequence.prototype.each = function each(fn) {
    var others = this.others, mergeFn = this.mergeFn || mergeObjects, keys = {};
    var iteratedFullSource = this.parent.each(function(value, key) {
      var merged = value;
      forEach(others, function(other) {
        if (key in other) {
          merged = mergeFn(merged, other[key]);
        }
      });
      keys[key] = true;
      return fn(merged, key);
    });
    if (iteratedFullSource === false) {
      return false;
    }
    var remaining = {};
    forEach(others, function(other) {
      for (var k in other) {
        if (!keys[k]) {
          remaining[k] = mergeFn(remaining[k], other[k]);
        }
      }
    });
    return Lazy(remaining).each(fn);
  };
  function mergeObjects(a, b) {
    var merged, prop;
    if (typeof b === "undefined") {
      return a;
    }
    if (isVanillaObject(a) && isVanillaObject(b)) {
      merged = {};
    } else if (a instanceof Array && b instanceof Array) {
      merged = [];
    } else {
      return b;
    }
    for (prop in a) {
      merged[prop] = mergeObjects(a[prop], b[prop]);
    }
    for (prop in b) {
      if (!merged[prop]) {
        merged[prop] = b[prop];
      }
    }
    return merged;
  }
  function isVanillaObject(object) {
    return object && object.constructor === Object;
  }
  ObjectLikeSequence.prototype.functions = function functions() {
    return this.filter(function(v, k) {
      return typeof v === "function";
    }).map(function(v, k) {
      return k;
    });
  };
  ObjectLikeSequence.prototype.methods = function methods() {
    return this.functions();
  };
  ObjectLikeSequence.prototype.pick = function pick(properties) {
    return new PickSequence(this, properties);
  };
  function PickSequence(parent, properties) {
    this.parent = parent;
    this.properties = properties;
  }
  PickSequence.prototype = new ObjectLikeSequence();
  PickSequence.prototype.get = function get(key) {
    return arrayContains(this.properties, key) ? this.parent.get(key) : void 0;
  };
  PickSequence.prototype.each = function each(fn) {
    var inArray = arrayContains, properties = this.properties;
    return this.parent.each(function(value, key) {
      if (inArray(properties, key)) {
        return fn(value, key);
      }
    });
  };
  ObjectLikeSequence.prototype.omit = function omit(properties) {
    return new OmitSequence(this, properties);
  };
  function OmitSequence(parent, properties) {
    this.parent = parent;
    this.properties = properties;
  }
  OmitSequence.prototype = new ObjectLikeSequence();
  OmitSequence.prototype.get = function get(key) {
    return arrayContains(this.properties, key) ? void 0 : this.parent.get(key);
  };
  OmitSequence.prototype.each = function each(fn) {
    var inArray = arrayContains, properties = this.properties;
    return this.parent.each(function(value, key) {
      if (!inArray(properties, key)) {
        return fn(value, key);
      }
    });
  };
  ObjectLikeSequence.prototype.pairs = function pairs() {
    return this.map(function(v, k) {
      return [k, v];
    });
  };
  ObjectLikeSequence.prototype.toArray = function toArray() {
    return this.pairs().toArray();
  };
  ObjectLikeSequence.prototype.toObject = function toObject() {
    return this.reduce(function(object, value, key) {
      object[key] = value;
      return object;
    }, {});
  };
  GroupedSequence.prototype = new ObjectLikeSequence();
  GroupedSequence.prototype.each = function each(fn) {
    var keyFn = createCallback(this.keyFn), valFn = createCallback(this.valFn), result;
    result = this.parent.reduce(function(grouped, e) {
      var key = keyFn(e), val = valFn(e);
      if (!(grouped[key] instanceof Array)) {
        grouped[key] = [val];
      } else {
        grouped[key].push(val);
      }
      return grouped;
    }, {});
    return transform(function(grouped) {
      for (var key in grouped) {
        if (fn(grouped[key], key) === false) {
          return false;
        }
      }
      return true;
    }, result);
  };
  IndexedSequence.prototype = new ObjectLikeSequence();
  IndexedSequence.prototype.each = function each(fn) {
    var keyFn = createCallback(this.keyFn), valFn = createCallback(this.valFn), indexed = {};
    return this.parent.each(function(e) {
      var key = keyFn(e), val = valFn(e);
      if (!indexed[key]) {
        indexed[key] = val;
        return fn(val, key);
      }
    });
  };
  CountedSequence.prototype = new ObjectLikeSequence();
  CountedSequence.prototype.each = function each(fn) {
    var keyFn = createCallback(this.keyFn), counted = {};
    this.parent.each(function(e) {
      var key = keyFn(e);
      if (!counted[key]) {
        counted[key] = 1;
      } else {
        counted[key] += 1;
      }
    });
    for (var key in counted) {
      if (fn(counted[key], key) === false) {
        return false;
      }
    }
    return true;
  };
  ObjectLikeSequence.prototype.watch = function watch(propertyNames) {
    throw new Error("You can only call #watch on a directly wrapped object.");
  };
  function ObjectWrapper(source) {
    this.source = source;
  }
  ObjectWrapper.prototype = new ObjectLikeSequence();
  ObjectWrapper.prototype.root = function root() {
    return this;
  };
  ObjectWrapper.prototype.isAsync = function isAsync() {
    return false;
  };
  ObjectWrapper.prototype.get = function get(key) {
    return this.source[key];
  };
  ObjectWrapper.prototype.each = function each(fn) {
    var source = this.source, keys = source ? Object.keys(source) : [], length = keys.length, key, index;
    for (index = 0; index < length; ++index) {
      key = keys[index];
      if (fn(source[key], key) === false) {
        return false;
      }
    }
    return true;
  };
  function StringLikeSequence() {
  }
  StringLikeSequence.prototype = new ArrayLikeSequence();
  StringLikeSequence.define = function define(methodName, overrides) {
    if (!overrides || typeof overrides.get !== "function") {
      throw new Error("A custom string-like sequence must implement *at least* get!");
    }
    return defineSequenceType(StringLikeSequence, methodName, overrides);
  };
  StringLikeSequence.prototype.value = function value() {
    return this.toString();
  };
  StringLikeSequence.prototype.getIterator = function getIterator() {
    return new CharIterator(this);
  };
  function CharIterator(source) {
    this.source = Lazy(source);
    this.index = -1;
  }
  CharIterator.prototype.current = function current() {
    return this.source.charAt(this.index);
  };
  CharIterator.prototype.moveNext = function moveNext() {
    return ++this.index < this.source.length();
  };
  StringLikeSequence.prototype.charAt = function charAt(i) {
    return this.get(i);
  };
  StringLikeSequence.prototype.charCodeAt = function charCodeAt(i) {
    var char = this.charAt(i);
    if (!char) {
      return NaN;
    }
    return char.charCodeAt(0);
  };
  StringLikeSequence.prototype.substring = function substring(start, stop) {
    return new StringSegment(this, start, stop);
  };
  function StringSegment(parent, start, stop) {
    this.parent = parent;
    this.start = Math.max(0, start);
    this.stop = stop;
  }
  StringSegment.prototype = new StringLikeSequence();
  StringSegment.prototype.get = function get(i) {
    return this.parent.get(i + this.start);
  };
  StringSegment.prototype.length = function length() {
    return (typeof this.stop === "number" ? this.stop : this.parent.length()) - this.start;
  };
  StringLikeSequence.prototype.first = function first(count) {
    if (typeof count === "undefined") {
      return this.charAt(0);
    }
    return this.substring(0, count);
  };
  StringLikeSequence.prototype.last = function last(count) {
    if (typeof count === "undefined") {
      return this.charAt(this.length() - 1);
    }
    return this.substring(this.length() - count);
  };
  StringLikeSequence.prototype.drop = function drop(count) {
    return this.substring(count);
  };
  StringLikeSequence.prototype.indexOf = function indexOf(substring, startIndex) {
    return this.toString().indexOf(substring, startIndex);
  };
  StringLikeSequence.prototype.lastIndexOf = function lastIndexOf(substring, startIndex) {
    return this.toString().lastIndexOf(substring, startIndex);
  };
  StringLikeSequence.prototype.contains = function contains(substring) {
    return this.indexOf(substring) !== -1;
  };
  StringLikeSequence.prototype.endsWith = function endsWith(suffix) {
    return this.substring(this.length() - suffix.length).toString() === suffix;
  };
  StringLikeSequence.prototype.startsWith = function startsWith(prefix) {
    return this.substring(0, prefix.length).toString() === prefix;
  };
  StringLikeSequence.prototype.toUpperCase = function toUpperCase() {
    return this.mapString(function(char) {
      return char.toUpperCase();
    });
  };
  StringLikeSequence.prototype.toLowerCase = function toLowerCase() {
    return this.mapString(function(char) {
      return char.toLowerCase();
    });
  };
  StringLikeSequence.prototype.mapString = function mapString(mapFn) {
    return new MappedStringLikeSequence(this, mapFn);
  };
  function MappedStringLikeSequence(parent, mapFn) {
    this.parent = parent;
    this.mapFn = mapFn;
  }
  MappedStringLikeSequence.prototype = new StringLikeSequence();
  MappedStringLikeSequence.prototype.get = IndexedMappedSequence.prototype.get;
  MappedStringLikeSequence.prototype.length = IndexedMappedSequence.prototype.length;
  StringLikeSequence.prototype.reverse = function reverse() {
    return new ReversedStringLikeSequence(this);
  };
  function ReversedStringLikeSequence(parent) {
    this.parent = parent;
  }
  ReversedStringLikeSequence.prototype = new StringLikeSequence();
  ReversedStringLikeSequence.prototype.get = IndexedReversedSequence.prototype.get;
  ReversedStringLikeSequence.prototype.length = IndexedReversedSequence.prototype.length;
  StringLikeSequence.prototype.toString = function toString() {
    return this.join("");
  };
  StringLikeSequence.prototype.match = function match(pattern) {
    return new StringMatchSequence(this, pattern);
  };
  function StringMatchSequence(parent, pattern) {
    this.parent = parent;
    this.pattern = pattern;
  }
  StringMatchSequence.prototype = new Sequence();
  StringMatchSequence.prototype.getIterator = function getIterator() {
    return new StringMatchIterator(this.parent.toString(), this.pattern);
  };
  function StringMatchIterator(source, pattern) {
    this.source = source;
    this.pattern = cloneRegex(pattern);
  }
  StringMatchIterator.prototype.current = function current() {
    return this.match[0];
  };
  StringMatchIterator.prototype.moveNext = function moveNext() {
    return !!(this.match = this.pattern.exec(this.source));
  };
  StringLikeSequence.prototype.split = function split(delimiter) {
    return new SplitStringSequence(this, delimiter);
  };
  function SplitStringSequence(parent, pattern) {
    this.parent = parent;
    this.pattern = pattern;
  }
  SplitStringSequence.prototype = new Sequence();
  SplitStringSequence.prototype.getIterator = function getIterator() {
    var source = this.parent.toString();
    if (this.pattern instanceof RegExp) {
      if (this.pattern.source === "" || this.pattern.source === "(?:)") {
        return new CharIterator(source);
      } else {
        return new SplitWithRegExpIterator(source, this.pattern);
      }
    } else if (this.pattern === "") {
      return new CharIterator(source);
    } else {
      return new SplitWithStringIterator(source, this.pattern);
    }
  };
  function SplitWithRegExpIterator(source, pattern) {
    this.source = source;
    this.pattern = cloneRegex(pattern);
  }
  SplitWithRegExpIterator.prototype.current = function current() {
    return this.source.substring(this.start, this.end);
  };
  SplitWithRegExpIterator.prototype.moveNext = function moveNext() {
    if (!this.pattern) {
      return false;
    }
    var match = this.pattern.exec(this.source);
    if (match) {
      this.start = this.nextStart ? this.nextStart : 0;
      this.end = match.index;
      this.nextStart = match.index + match[0].length;
      return true;
    } else if (this.pattern) {
      this.start = this.nextStart;
      this.end = void 0;
      this.nextStart = void 0;
      this.pattern = void 0;
      return true;
    }
    return false;
  };
  function SplitWithStringIterator(source, delimiter) {
    this.source = source;
    this.delimiter = delimiter;
  }
  SplitWithStringIterator.prototype.current = function current() {
    return this.source.substring(this.leftIndex, this.rightIndex);
  };
  SplitWithStringIterator.prototype.moveNext = function moveNext() {
    if (!this.finished) {
      this.leftIndex = typeof this.leftIndex !== "undefined" ? this.rightIndex + this.delimiter.length : 0;
      this.rightIndex = this.source.indexOf(this.delimiter, this.leftIndex);
    }
    if (this.rightIndex === -1) {
      this.finished = true;
      this.rightIndex = void 0;
      return true;
    }
    return !this.finished;
  };
  function StringWrapper(source) {
    this.source = source;
  }
  StringWrapper.prototype = new StringLikeSequence();
  StringWrapper.prototype.root = function root() {
    return this;
  };
  StringWrapper.prototype.isAsync = function isAsync() {
    return false;
  };
  StringWrapper.prototype.get = function get(i) {
    return this.source.charAt(i);
  };
  StringWrapper.prototype.length = function length() {
    return this.source.length;
  };
  StringWrapper.prototype.toString = function toString() {
    return this.source;
  };
  function GeneratedSequence(generatorFn, length) {
    this.get = generatorFn;
    this.fixedLength = length;
  }
  GeneratedSequence.prototype = new Sequence();
  GeneratedSequence.prototype.isAsync = function isAsync() {
    return false;
  };
  GeneratedSequence.prototype.length = function length() {
    return this.fixedLength;
  };
  GeneratedSequence.prototype.each = function each(fn) {
    var generatorFn = this.get, length = this.fixedLength, i = 0;
    while (typeof length === "undefined" || i < length) {
      if (fn(generatorFn(i), i++) === false) {
        return false;
      }
    }
    return true;
  };
  GeneratedSequence.prototype.getIterator = function getIterator() {
    return new GeneratedIterator(this);
  };
  function GeneratedIterator(sequence) {
    this.sequence = sequence;
    this.index = 0;
    this.currentValue = null;
  }
  GeneratedIterator.prototype.current = function current() {
    return this.currentValue;
  };
  GeneratedIterator.prototype.moveNext = function moveNext() {
    var sequence = this.sequence;
    if (typeof sequence.fixedLength === "number" && this.index >= sequence.fixedLength) {
      return false;
    }
    this.currentValue = sequence.get(this.index++);
    return true;
  };
  function AsyncSequence(parent, interval) {
    if (parent instanceof AsyncSequence) {
      throw new Error("Sequence is already asynchronous!");
    }
    this.parent = parent;
    this.interval = interval;
    this.onNextCallback = getOnNextCallback(interval);
    this.cancelCallback = getCancelCallback(interval);
  }
  AsyncSequence.prototype = new Sequence();
  AsyncSequence.prototype.isAsync = function isAsync() {
    return true;
  };
  AsyncSequence.prototype.getIterator = function getIterator() {
    throw new Error("An AsyncSequence does not support synchronous iteration.");
  };
  AsyncSequence.prototype.each = function each(fn) {
    var iterator = this.parent.getIterator(), onNextCallback = this.onNextCallback, cancelCallback = this.cancelCallback, i = 0;
    var handle = new AsyncHandle(function cancel() {
      if (cancellationId) {
        cancelCallback(cancellationId);
      }
    });
    var cancellationId = onNextCallback(function iterate() {
      cancellationId = null;
      try {
        if (iterator.moveNext() && fn(iterator.current(), i++) !== false) {
          cancellationId = onNextCallback(iterate);
        } else {
          handle._resolve();
        }
      } catch (e) {
        handle._reject(e);
      }
    });
    return handle;
  };
  function AsyncHandle(cancelFn) {
    this.resolveListeners = [];
    this.rejectListeners = [];
    this.state = PENDING;
    this.cancelFn = cancelFn;
  }
  var PENDING = 1, RESOLVED = 2, REJECTED = 3;
  AsyncHandle.prototype.then = function then(onFulfilled, onRejected) {
    var promise = new AsyncHandle(this.cancelFn);
    this.resolveListeners.push(function(value) {
      try {
        if (typeof onFulfilled !== "function") {
          resolve(promise, value);
          return;
        }
        resolve(promise, onFulfilled(value));
      } catch (e) {
        promise._reject(e);
      }
    });
    this.rejectListeners.push(function(reason) {
      try {
        if (typeof onRejected !== "function") {
          promise._reject(reason);
          return;
        }
        resolve(promise, onRejected(reason));
      } catch (e) {
        promise._reject(e);
      }
    });
    if (this.state === RESOLVED) {
      this._resolve(this.value);
    }
    if (this.state === REJECTED) {
      this._reject(this.reason);
    }
    return promise;
  };
  AsyncHandle.prototype._resolve = function _resolve(value) {
    if (this.state === REJECTED) {
      return;
    }
    if (this.state === PENDING) {
      this.state = RESOLVED;
      this.value = value;
    }
    consumeListeners(this.resolveListeners, this.value);
  };
  AsyncHandle.prototype._reject = function _reject(reason) {
    if (this.state === RESOLVED) {
      return;
    }
    if (this.state === PENDING) {
      this.state = REJECTED;
      this.reason = reason;
    }
    consumeListeners(this.rejectListeners, this.reason);
  };
  AsyncHandle.prototype.cancel = function cancel() {
    if (this.cancelFn) {
      this.cancelFn();
      this.cancelFn = null;
      this._resolve(false);
    }
  };
  AsyncHandle.prototype.onComplete = function onComplete(callback) {
    this.resolveListeners.push(callback);
    return this;
  };
  AsyncHandle.prototype.onError = function onError(callback) {
    this.rejectListeners.push(callback);
    return this;
  };
  function resolve(promise, x) {
    if (promise === x) {
      promise._reject(new TypeError("Cannot resolve a promise to itself"));
      return;
    }
    if (x instanceof AsyncHandle) {
      x.then(function(value) {
        resolve(promise, value);
      }, function(reason) {
        promise._reject(reason);
      });
      return;
    }
    var then;
    try {
      then = /function|object/.test(typeof x) && x != null && x.then;
    } catch (e) {
      promise._reject(e);
      return;
    }
    var thenableState = PENDING;
    if (typeof then === "function") {
      try {
        then.call(x, function resolvePromise(value) {
          if (thenableState !== PENDING) {
            return;
          }
          thenableState = RESOLVED;
          resolve(promise, value);
        }, function rejectPromise(reason) {
          if (thenableState !== PENDING) {
            return;
          }
          thenableState = REJECTED;
          promise._reject(reason);
        });
      } catch (e) {
        if (thenableState !== PENDING) {
          return;
        }
        promise._reject(e);
      }
      return;
    }
    promise._resolve(x);
  }
  function consumeListeners(listeners, value, callback) {
    callback || (callback = getOnNextCallback());
    callback(function() {
      if (listeners.length > 0) {
        listeners.shift()(value);
        consumeListeners(listeners, value, callback);
      }
    });
  }
  function getOnNextCallback(interval) {
    if (typeof interval === "undefined") {
      if (typeof setImmediate === "function") {
        return setImmediate;
      }
    }
    interval = interval || 0;
    return function(fn) {
      return setTimeout(fn, interval);
    };
  }
  function getCancelCallback(interval) {
    if (typeof interval === "undefined") {
      if (typeof clearImmediate === "function") {
        return clearImmediate;
      }
    }
    return clearTimeout;
  }
  function transform(fn, value) {
    if (value instanceof AsyncHandle) {
      return value.then(function() {
        fn(value);
      });
    }
    return fn(value);
  }
  AsyncSequence.prototype.reverse = function reverse() {
    return this.parent.reverse().async();
  };
  AsyncSequence.prototype.find = function find(predicate) {
    var found;
    var handle = this.each(function(e, i) {
      if (predicate(e, i)) {
        found = e;
        return false;
      }
    });
    return handle.then(function() {
      return found;
    });
  };
  AsyncSequence.prototype.indexOf = function indexOf(value) {
    var foundIndex = -1;
    var handle = this.each(function(e, i) {
      if (e === value) {
        foundIndex = i;
        return false;
      }
    });
    return handle.then(function() {
      return foundIndex;
    });
  };
  AsyncSequence.prototype.contains = function contains(value) {
    var found = false;
    var handle = this.each(function(e) {
      if (e === value) {
        found = true;
        return false;
      }
    });
    return handle.then(function() {
      return found;
    });
  };
  AsyncSequence.prototype.async = function async() {
    return this;
  };
  ObjectWrapper.prototype.watch = function watch(propertyNames) {
    return new WatchedPropertySequence(this.source, propertyNames);
  };
  function WatchedPropertySequence(object, propertyNames) {
    this.listeners = [];
    if (!propertyNames) {
      propertyNames = Lazy(object).keys().toArray();
    } else if (!(propertyNames instanceof Array)) {
      propertyNames = [propertyNames];
    }
    var listeners = this.listeners, index = 0;
    Lazy(propertyNames).each(function(propertyName) {
      var propertyValue = object[propertyName];
      Object.defineProperty(object, propertyName, {
        get: function() {
          return propertyValue;
        },
        set: function(value) {
          for (var i = listeners.length - 1; i >= 0; --i) {
            if (listeners[i]({property: propertyName, value}, index) === false) {
              listeners.splice(i, 1);
            }
          }
          propertyValue = value;
          ++index;
        }
      });
    });
  }
  WatchedPropertySequence.prototype = new AsyncSequence();
  WatchedPropertySequence.prototype.each = function each(fn) {
    this.listeners.push(fn);
  };
  function StreamLikeSequence() {
  }
  StreamLikeSequence.prototype = new AsyncSequence();
  StreamLikeSequence.prototype.isAsync = function isAsync() {
    return true;
  };
  StreamLikeSequence.prototype.split = function split(delimiter) {
    return new SplitStreamSequence(this, delimiter);
  };
  function SplitStreamSequence(parent, delimiter) {
    this.parent = parent;
    this.delimiter = delimiter;
    this.each = this.getEachForDelimiter(delimiter);
  }
  SplitStreamSequence.prototype = new Sequence();
  SplitStreamSequence.prototype.getEachForDelimiter = function getEachForDelimiter(delimiter) {
    if (delimiter instanceof RegExp) {
      return this.regexEach;
    }
    return this.stringEach;
  };
  SplitStreamSequence.prototype.regexEach = function each(fn) {
    var delimiter = cloneRegex(this.delimiter), buffer = "", start = 0, end, index = 0;
    var handle = this.parent.each(function(chunk) {
      buffer += chunk;
      var match;
      while (match = delimiter.exec(buffer)) {
        end = match.index;
        if (fn(buffer.substring(start, end), index++) === false) {
          return false;
        }
        start = end + match[0].length;
      }
      buffer = buffer.substring(start);
      start = 0;
    });
    handle.onComplete(function() {
      if (buffer.length > 0) {
        fn(buffer, index++);
      }
    });
    return handle;
  };
  SplitStreamSequence.prototype.stringEach = function each(fn) {
    var delimiter = this.delimiter, pieceIndex = 0, buffer = "", bufferIndex = 0;
    var handle = this.parent.each(function(chunk) {
      buffer += chunk;
      var delimiterIndex;
      while ((delimiterIndex = buffer.indexOf(delimiter)) >= 0) {
        var piece = buffer.substr(0, delimiterIndex);
        buffer = buffer.substr(delimiterIndex + delimiter.length);
        if (fn(piece, pieceIndex++) === false) {
          return false;
        }
      }
      return true;
    });
    handle.onComplete(function() {
      fn(buffer, pieceIndex++);
    });
    return handle;
  };
  StreamLikeSequence.prototype.lines = function lines() {
    return this.split("\n");
  };
  StreamLikeSequence.prototype.match = function match(pattern) {
    return new MatchedStreamSequence(this, pattern);
  };
  function MatchedStreamSequence(parent, pattern) {
    this.parent = parent;
    this.pattern = cloneRegex(pattern);
  }
  MatchedStreamSequence.prototype = new AsyncSequence();
  MatchedStreamSequence.prototype.each = function each(fn) {
    var pattern = this.pattern, done = false, i = 0;
    return this.parent.each(function(chunk) {
      Lazy(chunk).match(pattern).each(function(match) {
        if (fn(match, i++) === false) {
          done = true;
          return false;
        }
      });
      return !done;
    });
  };
  Lazy.createWrapper = function createWrapper(initializer) {
    var ctor = function() {
      this.listeners = [];
    };
    ctor.prototype = new StreamLikeSequence();
    ctor.prototype.each = function(listener) {
      this.listeners.push(listener);
    };
    ctor.prototype.emit = function(data) {
      var listeners = this.listeners;
      for (var len = listeners.length, i = len - 1; i >= 0; --i) {
        if (listeners[i](data) === false) {
          listeners.splice(i, 1);
        }
      }
    };
    return function() {
      var sequence = new ctor();
      initializer.apply(sequence, arguments);
      return sequence;
    };
  };
  Lazy.generate = function generate(generatorFn, length) {
    return new GeneratedSequence(generatorFn, length);
  };
  Lazy.range = function range() {
    var start = arguments.length > 1 ? arguments[0] : 0, stop = arguments.length > 1 ? arguments[1] : arguments[0], step = arguments.length > 2 && arguments[2];
    if (step === false) {
      step = stop > start ? 1 : -1;
    }
    if (step === 0) {
      return Lazy([]);
    }
    return Lazy.generate(function(i) {
      return start + step * i;
    }).take(Math.ceil((stop - start) / step));
  };
  Lazy.repeat = function repeat(value, count) {
    return Lazy.generate(function() {
      return value;
    }, count);
  };
  Lazy.Sequence = Sequence;
  Lazy.ArrayLikeSequence = ArrayLikeSequence;
  Lazy.ObjectLikeSequence = ObjectLikeSequence;
  Lazy.StringLikeSequence = StringLikeSequence;
  Lazy.StreamLikeSequence = StreamLikeSequence;
  Lazy.GeneratedSequence = GeneratedSequence;
  Lazy.AsyncSequence = AsyncSequence;
  Lazy.AsyncHandle = AsyncHandle;
  Lazy.clone = function clone(target) {
    return Lazy(target).value();
  };
  Lazy.deprecate = function deprecate(message, fn) {
    return function() {
      get_console().warn(message);
      return fn.apply(this, arguments);
    };
  };

let arrayPop;
function __get_arrayPop__() {
  return arrayPop = arrayPop || (Array.prototype.pop)
}

let arraySlice;
function __get_arraySlice__() {
  return arraySlice = arraySlice || (Array.prototype.slice)
}
  function createCallback(callback, defaultValue) {
    switch (typeof callback) {
      case "function":
        return callback;
      case "string":
        return function(e) {
          return e[callback];
        };
      case "object":
        return function(e) {
          return Lazy(callback).all(function(value, key) {
            return e[key] === value;
          });
        };
      case "undefined":
        return defaultValue ? function() {
          return defaultValue;
        } : Lazy.identity;
      default:
        throw new Error("Don't know how to make a callback from a " + typeof callback + "!");
    }
  }
  function createComparator(callback, descending) {
    if (!callback) {
      return compare;
    }
    callback = createCallback(callback);
    return function(x, y) {
      return compare(callback(x), callback(y));
    };
  }
  function reverseArguments(fn) {
    return function(x, y) {
      return fn(y, x);
    };
  }
  function createSet(values) {
    var set = new Set();
    Lazy(values || []).flatten().each(function(e) {
      set.add(e);
    });
    return set;
  }
  function compare(x, y) {
    if (x === y) {
      return 0;
    }
    return x > y ? 1 : -1;
  }
  function forEach(array, fn) {
    var i = -1, len = array.length;
    while (++i < len) {
      if (fn(array[i], i) === false) {
        return false;
      }
    }
    return true;
  }
  function getFirst(sequence) {
    var result;
    sequence.each(function(e) {
      result = e;
      return false;
    });
    return result;
  }
  function arrayContains(array, element) {
    var i = -1, length = array.length;
    if (element !== element) {
      while (++i < length) {
        if (array[i] !== array[i]) {
          return true;
        }
      }
      return false;
    }
    while (++i < length) {
      if (array[i] === element) {
        return true;
      }
    }
    return false;
  }
  function arrayContainsBefore(array, element, index, keyFn) {
    var i = -1;
    if (keyFn) {
      keyFn = createCallback(keyFn);
      while (++i < index) {
        if (keyFn(array[i]) === keyFn(element)) {
          return true;
        }
      }
    } else {
      while (++i < index) {
        if (array[i] === element) {
          return true;
        }
      }
    }
    return false;
  }
  function swap(array, i, j) {
    var temp = array[i];
    array[i] = array[j];
    array[j] = temp;
  }
  function cloneRegex(pattern) {
    return eval("" + pattern + (!pattern.global ? "g" : ""));
  }
  ;
  function Set() {
    this.table = {};
    this.objects = [];
  }
  Set.prototype.add = function add(value) {
    var table = this.table, type = typeof value, firstChar, objects;
    switch (type) {
      case "number":
      case "boolean":
      case "undefined":
        if (!table[value]) {
          table[value] = true;
          return true;
        }
        return false;
      case "string":
        switch (value.charAt(0)) {
          case "_":
          case "f":
          case "t":
          case "c":
          case "u":
          case "@":
          case "0":
          case "1":
          case "2":
          case "3":
          case "4":
          case "5":
          case "6":
          case "7":
          case "8":
          case "9":
          case "N":
            value = "@" + value;
        }
        if (!table[value]) {
          table[value] = true;
          return true;
        }
        return false;
      default:
        objects = this.objects;
        if (!arrayContains(objects, value)) {
          objects.push(value);
          return true;
        }
        return false;
    }
  };
  Set.prototype.contains = function contains(value) {
    var type = typeof value, firstChar;
    switch (type) {
      case "number":
      case "boolean":
      case "undefined":
        return !!this.table[value];
      case "string":
        switch (value.charAt(0)) {
          case "_":
          case "f":
          case "t":
          case "c":
          case "u":
          case "@":
          case "0":
          case "1":
          case "2":
          case "3":
          case "4":
          case "5":
          case "6":
          case "7":
          case "8":
          case "9":
          case "N":
            value = "@" + value;
        }
        return !!this.table[value];
      default:
        return arrayContains(this.objects, value);
    }
  };
  function Queue(capacity) {
    this.contents = new Array(capacity);
    this.start = 0;
    this.count = 0;
  }
  Queue.prototype.add = function add(element) {
    var contents = this.contents, capacity = contents.length, start = this.start;
    if (this.count === capacity) {
      contents[start] = element;
      this.start = (start + 1) % capacity;
    } else {
      contents[this.count++] = element;
    }
    return this;
  };
  Queue.prototype.toArray = function toArray() {
    var contents = this.contents, start = this.start, count = this.count;
    var snapshot = contents.slice(start, start + count);
    if (snapshot.length < count) {
      snapshot = snapshot.concat(contents.slice(0, count - snapshot.length));
    }
    return snapshot;
  };
  function defineSequenceType(base, name, overrides) {
    var ctor = function ctor() {
    };
    ctor.prototype = new base();
    for (var override in overrides) {
      ctor.prototype[override] = overrides[override];
    }
    var factory = function factory() {
      var sequence = new ctor();
      sequence.parent = this;
      if (sequence.init) {
        sequence.init.apply(sequence, arguments);
      }
      return sequence;
    };
    var methodNames = typeof name === "string" ? [name] : name;
    for (var i = 0; i < methodNames.length; ++i) {
      base.prototype[methodNames[i]] = factory;
    }
    return ctor;
  }
  return Lazy;
});