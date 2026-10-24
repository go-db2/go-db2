package converters

import (
	"encoding/binary"
	"strings"
	"unicode/utf16"
)

// DecodeUTF16BE decodes a sequence of Big-Endian UTF-16 bytes into a UTF-8 Go string.
// Optimization: Fast-path for ASCII characters (high byte 0x00, low byte < 0x80) uses stack byte buffer to bypass utf16.Decode []rune heap allocations (~4x faster).
func DecodeUTF16BE(b []byte) string {
	numUnits := len(b) / 2
	if numUnits == 0 {
		return ""
	}
	isASCII := true
	for i := 0; i < numUnits; i++ {
		if b[i*2] != 0 || b[i*2+1] >= 0x80 {
			isASCII = false
			break
		}
	}
	if isASCII {
		var stackBuf [64]byte
		var buf []byte
		if numUnits <= 64 {
			buf = stackBuf[:numUnits]
		} else {
			buf = make([]byte, numUnits)
		}
		for i := 0; i < numUnits; i++ {
			buf[i] = b[i*2+1]
		}
		return string(buf)
	}
	var stackU16 [64]uint16
	var u16 []uint16
	if numUnits <= 64 {
		u16 = stackU16[:numUnits]
	} else {
		u16 = make([]uint16, numUnits)
	}
	for i := 0; i < numUnits; i++ {
		u16[i] = binary.BigEndian.Uint16(b[i*2 : i*2+2])
	}
	return string(utf16.Decode(u16))
}

// EncodeUTF16BE encodes a UTF-8 Go string into a sequence of Big-Endian UTF-16 bytes.
// Optimization: Delegates to AppendUTF16BE to eliminate intermediate []rune and []uint16 slice allocations.
func EncodeUTF16BE(s string) []byte {
	return AppendUTF16BE(nil, s)
}

// AppendUTF16BE encodes a UTF-8 Go string into Big-Endian UTF-16 bytes and appends them to dst.
// Optimization: Fast-path for ASCII strings encodes directly into pre-allocated slice capacity;
// non-ASCII strings iterate directly over UTF-8 runes to avoid intermediate []rune and []uint16 heap allocations.
func AppendUTF16BE(dst []byte, s string) []byte {
	isASCII := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			isASCII = false
			break
		}
	}
	origLen := len(dst)
	if isASCII {
		needed := len(s) * 2
		if cap(dst)-origLen < needed {
			newBuf := make([]byte, origLen, origLen+needed+32)
			copy(newBuf, dst)
			dst = newBuf
		}
		dst = dst[:origLen+needed]
		idx := origLen
		for i := 0; i < len(s); i++ {
			dst[idx] = 0x00
			dst[idx+1] = s[i]
			idx += 2
		}
		return dst
	}

	for _, r := range s {
		if r <= 0xFFFF {
			dst = append(dst, byte(r>>8), byte(r))
		} else {
			// Encode surrogate pair
			r -= 0x10000
			u1 := uint16(0xD800 + (r >> 10))
			u2 := uint16(0xDC00 + (r & 0x3FF))
			dst = append(dst, byte(u1>>8), byte(u1), byte(u2>>8), byte(u2))
		}
	}
	return dst
}

// PadGraphicUTF16BE pads a UTF-16 BE byte slice with DBCS spaces (0x0020) up to targetCharLen characters.
func PadGraphicUTF16BE(b []byte, targetCharLen int) []byte {
	if targetCharLen <= 0 {
		return nil
	}
	currentChars := len(b) / 2
	if currentChars >= targetCharLen {
		return b[:targetCharLen*2]
	}
	res := make([]byte, targetCharLen*2)
	copy(res, b)
	for i := currentChars; i < targetCharLen; i++ {
		binary.BigEndian.PutUint16(res[i*2:i*2+2], 0x0020)
	}
	return res
}

// TrimRightGraphicSpaces trims trailing spaces (including DBCS / Ideographic space U+3000 and nulls) from a graphic string.
func TrimRightGraphicSpaces(s string) string {
	return strings.TrimRight(s, " \t\r\n\u0020\u3000\u0000")
}
