// Character encoding shared by all puzzle variants.
//
// 16-valued puzzles (hexadoku, hexamurai) use the hex digits 0-F.
// 9-valued ones (sudoku, samurai) use the digits 1-9, which leaves '0'
// free to mean "empty" as in most sudoku collections.
// '.', '*' and '_' always mark an empty cell.
package main

// charOf returns the printable character for value v of an nvals-valued puzzle.
func charOf(nvals int, v uint8) byte {
	if nvals == 9 {
		return '1' + v // 9x9: bits 0-8 represent values 1-9
	}
	if v < 10 {
		return '0' + v
	}
	return 'A' + v - 10
}

const (
	chEmpty = -1 // character marks an empty cell
	chNone  = -2 // character is not part of a grid (space, border, ...)
)

// valueOf returns the value of character ch in an nvals-valued puzzle,
// or chEmpty / chNone.
func valueOf(nvals int, ch byte) int {
	switch {
	case ch == '.' || ch == '*' || ch == '_':
		return chEmpty
	case ch >= '0' && ch <= '9':
		if nvals == 9 {
			if ch == '0' {
				return chEmpty
			}
			return int(ch - '1')
		}
		return int(ch - '0')
	case nvals == maxSize && ch >= 'a' && ch <= 'f':
		return int(ch-'a') + 10
	case nvals == maxSize && ch >= 'A' && ch <= 'F':
		return int(ch-'A') + 10
	}
	return chNone
}
