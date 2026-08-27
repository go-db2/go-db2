package converters

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

// DRDA Wire Data Types
const (
	DRDATypeInteger      uint8 = 0x02
	DRDATypeNInteger     uint8 = 0x03
	DRDATypeSmall        uint8 = 0x04
	DRDATypeNSmall       uint8 = 0x05
	DRDAType1ByteInt     uint8 = 0x06
	DRDATypeN1ByteInt    uint8 = 0x07
	DRDATypeFloat8       uint8 = 0x0A
	DRDATypeNFloat8      uint8 = 0x0B
	DRDATypeFloat4       uint8 = 0x0C
	DRDATypeNFloat4      uint8 = 0x0D
	DRDATypeDecimal      uint8 = 0x0E
	DRDATypeNDecimal     uint8 = 0x0F
	DRDATypeInteger8     uint8 = 0x16
	DRDATypeNInteger8    uint8 = 0x17
	DRDATypeLOBLOC       uint8 = 0x18
	DRDATypeNLOBLOC      uint8 = 0x19
	DRDATypeCLOBLOC      uint8 = 0x1A
	DRDATypeNCLOBLOC     uint8 = 0x1B
	DRDATypeDBCSCLOBLOC  uint8 = 0x1C
	DRDATypeNDBCSCLOBLOC uint8 = 0x1D
	DRDATypeRowID        uint8 = 0x1E
	DRDATypeNRowID       uint8 = 0x1F
	DRDATypeDate         uint8 = 0x20
	DRDATypeNDate        uint8 = 0x21
	DRDATypeTime         uint8 = 0x22
	DRDATypeNTime        uint8 = 0x23
	DRDATypeTimestamp    uint8 = 0x24
	DRDATypeNTimestamp   uint8 = 0x25
	DRDATypeFixByte      uint8 = 0x26
	DRDATypeNFixByte     uint8 = 0x27
	DRDATypeVarByte      uint8 = 0x28
	DRDATypeNVarByte     uint8 = 0x29
	DRDATypeLongVarByte  uint8 = 0x2A
	DRDATypeNLongVarByte uint8 = 0x2B
	DRDATypeChar         uint8 = 0x30
	DRDATypeNChar        uint8 = 0x31
	DRDATypeVarChar      uint8 = 0x32
	DRDATypeNVarChar     uint8 = 0x33
	DRDATypeLong         uint8 = 0x34
	DRDATypeNLong        uint8 = 0x35
	DRDATypeGraphic      uint8 = 0x36
	DRDATypeNGraphic     uint8 = 0x37
	DRDATypeVarGraph     uint8 = 0x38
	DRDATypeNVarGraph    uint8 = 0x39
	DRDATypeLonGraph     uint8 = 0x3A
	DRDATypeNLonGraph    uint8 = 0x3B
	DRDATypeMix          uint8 = 0x3C
	DRDATypeNMix         uint8 = 0x3D
	DRDATypeVarMix       uint8 = 0x3E
	DRDATypeNVarMix      uint8 = 0x3F
	DRDATypeLongMix      uint8 = 0x40
	DRDATypeNLongMix     uint8 = 0x41
	DRDATypeCStrMix      uint8 = 0x42
	DRDATypeNCStrMix     uint8 = 0x43
	DRDATypeBoolean      uint8 = 0xBE
	DRDATypeNBoolean     uint8 = 0xBF
	DRDATypeFixBytes     uint8 = 0xC0
	DRDATypeNFixBytes    uint8 = 0xC1
	DRDATypeVarBinary    uint8 = 0xC2
	DRDATypeNVarBinary   uint8 = 0xC3
	DRDATypeLOBBytes     uint8 = 0xC8
	DRDATypeNLOBBytes    uint8 = 0xC9
	DRDATypeLOBCSBCS     uint8 = 0xCE
	DRDATypeNLOBCSBCS    uint8 = 0xCF
)

// IsNullableDRDAType returns true if the DRDA wire type supports NULL indicators.
func IsNullableDRDAType(t uint8) bool {
	switch t {
	case DRDATypeNInteger, DRDATypeNSmall, DRDATypeN1ByteInt,
		DRDATypeNFloat8, DRDATypeNFloat4, DRDATypeNDecimal,
		DRDATypeNInteger8, DRDATypeNRowID, DRDATypeNDate,
		DRDATypeNTime, DRDATypeNTimestamp, DRDATypeNFixByte,
		DRDATypeNVarByte, DRDATypeNLongVarByte, DRDATypeNChar,
		DRDATypeNVarChar, DRDATypeNLong, DRDATypeNGraphic,
		DRDATypeNVarGraph, DRDATypeNLonGraph, DRDATypeNMix,
		DRDATypeNVarMix, DRDATypeNLongMix, DRDATypeNCStrMix,
		DRDATypeNBoolean, DRDATypeNFixBytes, DRDATypeNVarBinary,
		DRDATypeNLOBLOC, DRDATypeNCLOBLOC, DRDATypeNDBCSCLOBLOC,
		DRDATypeNLOBBytes, DRDATypeNLOBCSBCS,
		0xF5, 0xF7, 0xF9:
		return true
	default:
		return false
	}
}

// DecodeField reads and parses a single column value from a binary QRYDTA reader.
func DecodeField(drdaType uint8, ps []byte, r io.Reader, endian binary.ByteOrder) (any, error) {
	if IsNullableDRDAType(drdaType) {
		nullIndicator := make([]byte, 1)
		if _, err := io.ReadFull(r, nullIndicator); err != nil {
			return nil, err
		}
		if nullIndicator[0] == 0xFF {
			return nil, nil // Null value
		}
	}

	switch drdaType {
	case DRDATypeChar, DRDATypeNChar, DRDATypeMix, DRDATypeNMix:
		ln := int(binary.BigEndian.Uint16(ps))
		buf := make([]byte, ln)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return strings.TrimRight(string(buf), " "), nil

	case DRDATypeVarChar, DRDATypeNVarChar,
		DRDATypeVarMix, DRDATypeNVarMix,
		DRDATypeLong, DRDATypeNLong,
		DRDATypeLongMix, DRDATypeNLongMix:
		var lenBuf [2]byte
		if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
			return nil, err
		}
		ln := int(binary.BigEndian.Uint16(lenBuf[:]))
		buf := make([]byte, ln)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return string(buf), nil

	case DRDATypeSmall, DRDATypeNSmall:
		var buf [2]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return nil, err
		}
		return int64(int16(endian.Uint16(buf[:]))), nil

	case DRDATypeInteger, DRDATypeNInteger:
		var buf [4]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return nil, err
		}
		return int64(int32(endian.Uint32(buf[:]))), nil

	case DRDATypeInteger8, DRDATypeNInteger8:
		var buf [8]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return nil, err
		}
		return int64(endian.Uint64(buf[:])), nil

	case DRDAType1ByteInt, DRDATypeN1ByteInt:
		var buf [1]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return nil, err
		}
		return int64(int8(buf[0])), nil

	case DRDATypeFloat4, DRDATypeNFloat4:
		var buf [4]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return nil, err
		}
		bits := endian.Uint32(buf[:])
		return float64(math.Float32frombits(bits)), nil

	case DRDATypeFloat8, DRDATypeNFloat8:
		var buf [8]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return nil, err
		}
		bits := endian.Uint64(buf[:])
		return math.Float64frombits(bits), nil

	case DRDATypeBoolean, DRDATypeNBoolean:
		ln := int(binary.BigEndian.Uint16(ps))
		buf := make([]byte, ln)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return buf[len(buf)-1] != 0, nil

	case DRDATypeDate, DRDATypeNDate:
		ln := int(binary.BigEndian.Uint16(ps))
		buf := make([]byte, ln)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		t, err := time.Parse("2006-01-02", string(buf))
		if err != nil {
			return string(buf), nil
		}
		return t, nil

	case DRDATypeTime, DRDATypeNTime:
		ln := int(binary.BigEndian.Uint16(ps))
		buf := make([]byte, ln)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return string(buf), nil

	case DRDATypeTimestamp, DRDATypeNTimestamp:
		ln := int(binary.BigEndian.Uint16(ps))
		buf := make([]byte, ln)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		s := strings.TrimRight(string(buf), " ")
		// Formats: "2006-01-02-15.04.05" or "2006-01-02-15.04.05.000000"
		if len(s) >= 19 {
			layout := "2006-01-02-15.04.05"
			if len(s) > 19 && s[19] == '.' {
				layout = "2006-01-02-15.04.05.000000"
			}
			if t, err := time.Parse(layout, s); err == nil {
				return t, nil
			}
		}
		return s, nil

	case DRDATypeVarBinary, DRDATypeNVarBinary, DRDATypeVarByte, DRDATypeNVarByte:
		var lenBuf [2]byte
		if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
			return nil, err
		}
		ln := int(binary.BigEndian.Uint16(lenBuf[:]))
		buf := make([]byte, ln)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return buf, nil

	case DRDATypeFixByte, DRDATypeNFixByte, DRDATypeFixBytes, DRDATypeNFixBytes, DRDATypeRowID, DRDATypeNRowID:
		ln := int(binary.BigEndian.Uint16(ps)) & 0x7FFF
		buf := make([]byte, ln)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return buf, nil

	case DRDATypeDecimal, DRDATypeNDecimal:
		if len(ps) < 2 {
			return nil, fmt.Errorf("invalid Decimal ps descriptor length: %d", len(ps))
		}
		precision := int(ps[0])
		scale := int(ps[1])
		byteLen := (precision + 2) / 2
		buf := make([]byte, byteLen)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return DecodePackedDecimal(buf, scale), nil

	case DRDATypeLOBLOC, DRDATypeNLOBLOC, DRDATypeCLOBLOC, DRDATypeNCLOBLOC,
		DRDATypeDBCSCLOBLOC, DRDATypeNDBCSCLOBLOC,
		DRDATypeLOBBytes, DRDATypeNLOBBytes, DRDATypeLOBCSBCS, DRDATypeNLOBCSBCS,
		0x10, 0x11, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9:
		ln := int(binary.BigEndian.Uint16(ps)) & 0x7FFF
		if ln > 0 {
			buf := make([]byte, ln)
			if _, err := io.ReadFull(r, buf); err != nil {
				return nil, err
			}
		}
		if drdaType == DRDATypeLOBCSBCS || drdaType == DRDATypeNLOBCSBCS || drdaType == 0xF6 || drdaType == 0xF7 {
			return "", nil
		}
		return []byte{}, nil

	default:
		// Fallback: try reading ps length
		if len(ps) >= 2 {
			ln := int(binary.BigEndian.Uint16(ps)) & 0x7FFF
			if ln > 0 {
				buf := make([]byte, ln)
				_, _ = io.ReadFull(r, buf)
				return buf, nil
			}
		}
		return nil, fmt.Errorf("unsupported DRDA field type: 0x%02X", drdaType)
	}
}

// DecodePackedDecimal converts IBM Packed Decimal bytes into a decimal string.
func DecodePackedDecimal(b []byte, scale int) string {
	var digits bytes.Buffer
	for i, by := range b {
		high := (by >> 4) & 0x0F
		low := by & 0x0F

		if i == len(b)-1 {
			// Last byte: high nibble is digit, low nibble is sign (0xC, 0xF = +, 0xD = -)
			digits.WriteByte('0' + high)
			sign := ""
			if low == 0x0D || low == 0x0B {
				sign = "-"
			}
			numStr := strings.TrimLeft(digits.String(), "0")
			if numStr == "" {
				numStr = "0"
			}
			if scale > 0 {
				if len(numStr) <= scale {
					numStr = "0." + strings.Repeat("0", scale-len(numStr)) + numStr
				} else {
					dotPos := len(numStr) - scale
					numStr = numStr[:dotPos] + "." + numStr[dotPos:]
				}
			}
			return sign + numStr
		}

		digits.WriteByte('0' + high)
		digits.WriteByte('0' + low)
	}
	return digits.String()
}
