package protocol

import "fmt"

// Device message types (device → client).
const (
	DeviceMsgTypeClipboard    uint8 = 0
	DeviceMsgTypeAckClipboard uint8 = 1
	DeviceMsgTypeUhidOutput   uint8 = 2
)

const (
	DeviceMsgMaxSize      = 1 << 18 // 256KB
	DeviceMsgTextMaxLen   = DeviceMsgMaxSize - 5
)

// DeviceMsg represents a message from the device.
type DeviceMsg struct {
	Type uint8

	// Clipboard (type 0)
	ClipboardText string

	// AckClipboard (type 1)
	AckSequence uint64

	// UhidOutput (type 2)
	UhidID   uint16
	UhidData []byte
}

// DeserializeDeviceMsg reads one device message from buf.
// Returns the message, number of bytes consumed, and any error.
// consumed=0 means incomplete message (need more data).
func DeserializeDeviceMsg(buf []byte) (DeviceMsg, int, error) {
	if len(buf) == 0 {
		return DeviceMsg{}, 0, nil
	}

	var msg DeviceMsg
	msg.Type = buf[0]

	switch msg.Type {
	case DeviceMsgTypeClipboard:
		if len(buf) < 5 {
			return DeviceMsg{}, 0, nil
		}
		textLen := int(Read32BE(buf[1:5]))
		if len(buf) < 5+textLen {
			return DeviceMsg{}, 0, nil
		}
		msg.ClipboardText = string(buf[5 : 5+textLen])
		return msg, 5 + textLen, nil

	case DeviceMsgTypeAckClipboard:
		if len(buf) < 9 {
			return DeviceMsg{}, 0, nil
		}
		msg.AckSequence = Read64BE(buf[1:9])
		return msg, 9, nil

	case DeviceMsgTypeUhidOutput:
		if len(buf) < 5 {
			return DeviceMsg{}, 0, nil
		}
		msg.UhidID = Read16BE(buf[1:3])
		size := int(Read16BE(buf[3:5]))
		if len(buf) < 5+size {
			return DeviceMsg{}, 0, nil
		}
		msg.UhidData = make([]byte, size)
		copy(msg.UhidData, buf[5:5+size])
		return msg, 5 + size, nil

	default:
		return DeviceMsg{}, -1, fmt.Errorf("unknown device message type: %d", msg.Type)
	}
}
