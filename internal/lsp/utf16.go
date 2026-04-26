package lsp

// LSP positions and lengths are measured in UTF-16 code units, but lore's
// internal scanners work in UTF-8 byte offsets. The two agree on ASCII and
// disagree once any text contains a non-ASCII rune (e.g. the curly
// apostrophe `’` is 3 bytes UTF-8 but 1 UTF-16 unit). These helpers convert
// between the two for a single line.

// utf16UnitsForBytes returns the number of UTF-16 code units occupied by
// the first byteOff bytes of line. byteOff is clamped to [0, len(line)] and
// rounded down to the nearest rune boundary if it lands inside a multi-byte
// rune.
func utf16UnitsForBytes(line string, byteOff int) uint32 {
	if byteOff <= 0 {
		return 0
	}
	if byteOff > len(line) {
		byteOff = len(line)
	}
	var units uint32
	for i, r := range line {
		if i >= byteOff {
			break
		}
		if r > 0xFFFF {
			units += 2
		} else {
			units++
		}
	}
	return units
}

// bytesForUTF16Units returns the byte offset within line that corresponds to
// the given UTF-16 unit offset. If units lands inside a surrogate pair the
// result clamps to the start of that rune.
func bytesForUTF16Units(line string, units uint32) int {
	if units == 0 {
		return 0
	}
	var consumed uint32
	for i, r := range line {
		if consumed >= units {
			return i
		}
		if r > 0xFFFF {
			consumed += 2
		} else {
			consumed++
		}
	}
	return len(line)
}
