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

// ---------------- wide alphabets ----------------
//
// Two Elektor summer puzzles run over more values than a uint16 candidate
// mask can hold: the Alphanumski of 7-8/2007 over 36 and the AlphaSudoku
// of 7-8/2008 over 25. They are handled by the separate core in alpha.go,
// which carries its masks in a uint64, and their alphabets are given as a
// plain string - value v is the character at index v.
const (
	alnum36 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	// no '0': the AlphaSudoku alphabet includes the letter 'O', and the
	// magazine drops the digit to keep the two apart.
	alsu25 = "123456789ABCDEFGHIJKLMNOP"
	// the Alfadoku of Elektuur 7-8/2006 uses no digits at all: "alle
	// letters van het alfabet, met uitzondering van de Z, dus A t/m Y".
	alfa25 = "ABCDEFGHIJKLMNOPQRSTUVWXY"
)

func charOfIn(alphabet string, v uint8) byte { return alphabet[v] }

// valueOfIn returns the value of character ch in the given alphabet, or
// chEmpty / chNone. Unlike valueOf it is a cold path - parsing only - so
// the linear scan costs nothing worth avoiding.
func valueOfIn(alphabet string, ch byte) int {
	if ch == '.' || ch == '*' || ch == '_' {
		return chEmpty
	}
	for i := 0; i < len(alphabet); i++ {
		if alphabet[i] == ch {
			return i
		}
	}
	return chNone
}
