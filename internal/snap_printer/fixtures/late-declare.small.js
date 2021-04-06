function foo() {
  arraySlice.call(arguments, 0)
}
var arraySlice = Array.prototype.slice;
module.exports = foo
