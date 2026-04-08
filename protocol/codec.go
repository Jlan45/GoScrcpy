package protocol

// Codec IDs as transmitted on the wire (4-byte big-endian ASCII).
const (
	CodecH264 uint32 = 0x68323634 // "h264"
	CodecH265 uint32 = 0x68323635 // "h265"
	CodecAV1  uint32 = 0x00617631 // "av1"
	CodecOPUS uint32 = 0x6f707573 // "opus"
	CodecAAC  uint32 = 0x00616163 // "aac"
	CodecFLAC uint32 = 0x666c6163 // "flac"
	CodecRAW  uint32 = 0x00726177 // "raw"

	// Special codec ID values from the server.
	CodecDisabled   uint32 = 0x00000000 // stream explicitly disabled
	CodecConfigErr  uint32 = 0x00000001 // configuration error on device
)

// CodecName returns a human-readable name for a codec ID.
func CodecName(id uint32) string {
	switch id {
	case CodecH264:
		return "h264"
	case CodecH265:
		return "h265"
	case CodecAV1:
		return "av1"
	case CodecOPUS:
		return "opus"
	case CodecAAC:
		return "aac"
	case CodecFLAC:
		return "flac"
	case CodecRAW:
		return "raw"
	case CodecDisabled:
		return "disabled"
	case CodecConfigErr:
		return "config-error"
	default:
		return "unknown"
	}
}

// Audio constants hardcoded in scrcpy.
const (
	AudioSampleRate = 48000
	AudioChannels   = 2
)
