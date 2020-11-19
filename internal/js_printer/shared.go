package js_printer

// This has been copied from js_printer.go `addSourceMapping` body in order to use it from
// snap_printer without having to make LineOffsetTable properties public
// It was slightly modified:
//    accesses to `loc.Start` are `start`,
//    accesses to `lineOffsetTables` deref the pointer
//    originalLine, originalColumn are declared as return vars and thus ` := ` was changed to ` = `
func GetOriginalLoc(lineOffsetTables *[]LineOffsetTable, start int32) (originalLine int, originalColumn int) {
	// Binary search to find the line
	count := len(*lineOffsetTables)
	originalLine = 0
	for count > 0 {
		step := count / 2
		i := originalLine + step
		if (*lineOffsetTables)[i].byteOffsetToStartOfLine <= start {
			originalLine = i + 1
			count = count - step - 1
		} else {
			count = step
		}
	}
	originalLine--

	// Use the line to compute the column
	line := &(*lineOffsetTables)[originalLine]
	originalColumn = int(start - line.byteOffsetToStartOfLine)
	if line.columnsForNonASCII != nil && originalColumn >= int(line.byteOffsetToFirstNonASCII) {
		originalColumn = int(line.columnsForNonASCII[originalColumn-int(line.byteOffsetToFirstNonASCII)])
	}
	return originalLine, originalColumn
}
