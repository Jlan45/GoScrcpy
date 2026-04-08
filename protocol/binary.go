package protocol

import "encoding/binary"

var BE = binary.BigEndian

func Read16BE(b []byte) uint16 { return BE.Uint16(b) }
func Read32BE(b []byte) uint32 { return BE.Uint32(b) }
func Read64BE(b []byte) uint64 { return BE.Uint64(b) }

func Write16BE(b []byte, v uint16) { BE.PutUint16(b, v) }
func Write32BE(b []byte, v uint32) { BE.PutUint32(b, v) }
func Write64BE(b []byte, v uint64) { BE.PutUint64(b, v) }

// FloatToU16FP converts a float in [0, 1] to unsigned 16-bit fixed-point.
func FloatToU16FP(f float32) uint16 {
	if f >= 1.0 {
		return 0xFFFF
	}
	if f <= 0.0 {
		return 0
	}
	return uint16(f * 65536)
}

// FloatToI16FP converts a float in [-1, 1] to signed 16-bit fixed-point.
func FloatToI16FP(f float32) int16 {
	if f >= 1.0 {
		return 0x7FFF
	}
	if f <= -1.0 {
		return -0x8000
	}
	return int16(f * 32768)
}
