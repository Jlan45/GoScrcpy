package protocol

// Control message types (client → device).
const (
	ControlMsgTypeInjectKeycode            uint8 = 0
	ControlMsgTypeInjectText               uint8 = 1
	ControlMsgTypeInjectTouchEvent          uint8 = 2
	ControlMsgTypeInjectScrollEvent         uint8 = 3
	ControlMsgTypeBackOrScreenOn            uint8 = 4
	ControlMsgTypeExpandNotificationPanel   uint8 = 5
	ControlMsgTypeExpandSettingsPanel       uint8 = 6
	ControlMsgTypeCollapsePanels            uint8 = 7
	ControlMsgTypeGetClipboard              uint8 = 8
	ControlMsgTypeSetClipboard              uint8 = 9
	ControlMsgTypeSetDisplayPower           uint8 = 10
	ControlMsgTypeRotateDevice              uint8 = 11
	ControlMsgTypeUhidCreate                uint8 = 12
	ControlMsgTypeUhidInput                 uint8 = 13
	ControlMsgTypeUhidDestroy               uint8 = 14
	ControlMsgTypeOpenHardKeyboardSettings  uint8 = 15
	ControlMsgTypeStartApp                  uint8 = 16
	ControlMsgTypeResetVideo                uint8 = 17
)

const (
	ControlMsgMaxSize              = 1 << 18 // 256KB
	ControlMsgInjectTextMaxLength  = 300
	ControlMsgClipboardTextMaxLen  = ControlMsgMaxSize - 14
)

// Android key event actions.
const (
	ActionDown     uint8 = 0
	ActionUp       uint8 = 1
	ActionMultiple uint8 = 2
)

// Copy key for GET_CLIPBOARD.
const (
	CopyKeyNone uint8 = 0
	CopyKeyCopy uint8 = 1
	CopyKeyCut  uint8 = 2
)

// Well-known pointer IDs.
const (
	PointerIDMouse         uint64 = ^uint64(0)     // 0xFFFFFFFFFFFFFFFF
	PointerIDGenericFinger uint64 = ^uint64(0) - 1  // 0xFFFFFFFFFFFFFFFE
	PointerIDVirtualFinger uint64 = ^uint64(0) - 2  // 0xFFFFFFFFFFFFFFFD
)

// ControlMsg is the interface for all control messages.
type ControlMsg interface {
	Serialize() []byte
}

// Position represents a point on the device screen.
type Position struct {
	X            int32
	Y            int32
	ScreenWidth  uint16
	ScreenHeight uint16
}

func writePosition(buf []byte, p Position) {
	Write32BE(buf[0:], uint32(p.X))
	Write32BE(buf[4:], uint32(p.Y))
	Write16BE(buf[8:], p.ScreenWidth)
	Write16BE(buf[10:], p.ScreenHeight)
}

// InjectKeycode — type 0, 14 bytes.
type InjectKeycode struct {
	Action    uint8
	Keycode   uint32
	Repeat    uint32
	Metastate uint32
}

func (m *InjectKeycode) Serialize() []byte {
	buf := make([]byte, 14)
	buf[0] = ControlMsgTypeInjectKeycode
	buf[1] = m.Action
	Write32BE(buf[2:], m.Keycode)
	Write32BE(buf[6:], m.Repeat)
	Write32BE(buf[10:], m.Metastate)
	return buf
}

// InjectText — type 1, variable length.
type InjectText struct {
	Text string
}

func (m *InjectText) Serialize() []byte {
	text := truncateUTF8(m.Text, ControlMsgInjectTextMaxLength)
	buf := make([]byte, 1+4+len(text))
	buf[0] = ControlMsgTypeInjectText
	Write32BE(buf[1:], uint32(len(text)))
	copy(buf[5:], text)
	return buf
}

// InjectTouchEvent — type 2, 32 bytes.
type InjectTouchEvent struct {
	Action       uint8
	PointerID    uint64
	Position     Position
	Pressure     float32
	ActionButton uint32
	Buttons      uint32
}

func (m *InjectTouchEvent) Serialize() []byte {
	buf := make([]byte, 32)
	buf[0] = ControlMsgTypeInjectTouchEvent
	buf[1] = m.Action
	Write64BE(buf[2:], m.PointerID)
	writePosition(buf[10:], m.Position)
	Write16BE(buf[22:], FloatToU16FP(m.Pressure))
	Write32BE(buf[24:], m.ActionButton)
	Write32BE(buf[28:], m.Buttons)
	return buf
}

// InjectScrollEvent — type 3, 21 bytes.
type InjectScrollEvent struct {
	Position Position
	HScroll  float32 // [-16, 16]
	VScroll  float32 // [-16, 16]
	Buttons  uint32
}

func (m *InjectScrollEvent) Serialize() []byte {
	buf := make([]byte, 21)
	buf[0] = ControlMsgTypeInjectScrollEvent
	writePosition(buf[1:], m.Position)
	hNorm := clampF(m.HScroll/16, -1, 1)
	vNorm := clampF(m.VScroll/16, -1, 1)
	Write16BE(buf[13:], uint16(FloatToI16FP(hNorm)))
	Write16BE(buf[15:], uint16(FloatToI16FP(vNorm)))
	Write32BE(buf[17:], m.Buttons)
	return buf
}

// BackOrScreenOn — type 4, 2 bytes.
type BackOrScreenOn struct {
	Action uint8
}

func (m *BackOrScreenOn) Serialize() []byte {
	return []byte{ControlMsgTypeBackOrScreenOn, m.Action}
}

// Simple 1-byte messages.
type (
	ExpandNotificationPanel  struct{}
	ExpandSettingsPanel      struct{}
	CollapsePanels           struct{}
	RotateDevice             struct{}
	OpenHardKeyboardSettings struct{}
	ResetVideo               struct{}
)

func (m *ExpandNotificationPanel) Serialize() []byte  { return []byte{ControlMsgTypeExpandNotificationPanel} }
func (m *ExpandSettingsPanel) Serialize() []byte       { return []byte{ControlMsgTypeExpandSettingsPanel} }
func (m *CollapsePanels) Serialize() []byte            { return []byte{ControlMsgTypeCollapsePanels} }
func (m *RotateDevice) Serialize() []byte              { return []byte{ControlMsgTypeRotateDevice} }
func (m *OpenHardKeyboardSettings) Serialize() []byte  { return []byte{ControlMsgTypeOpenHardKeyboardSettings} }
func (m *ResetVideo) Serialize() []byte                { return []byte{ControlMsgTypeResetVideo} }

// GetClipboard — type 8, 2 bytes.
type GetClipboard struct {
	CopyKey uint8
}

func (m *GetClipboard) Serialize() []byte {
	return []byte{ControlMsgTypeGetClipboard, m.CopyKey}
}

// SetClipboard — type 9, variable length.
type SetClipboard struct {
	Sequence uint64
	Text     string
	Paste    bool
}

func (m *SetClipboard) Serialize() []byte {
	text := truncateUTF8(m.Text, ControlMsgClipboardTextMaxLen)
	buf := make([]byte, 1+8+1+4+len(text))
	buf[0] = ControlMsgTypeSetClipboard
	Write64BE(buf[1:], m.Sequence)
	if m.Paste {
		buf[9] = 1
	}
	Write32BE(buf[10:], uint32(len(text)))
	copy(buf[14:], text)
	return buf
}

// SetDisplayPower — type 10, 2 bytes.
type SetDisplayPower struct {
	On bool
}

func (m *SetDisplayPower) Serialize() []byte {
	b := byte(0)
	if m.On {
		b = 1
	}
	return []byte{ControlMsgTypeSetDisplayPower, b}
}

// UhidCreate — type 12, variable length.
type UhidCreate struct {
	ID             uint16
	VendorID       uint16
	ProductID      uint16
	Name           string // max 127 bytes
	ReportDesc     []byte
}

func (m *UhidCreate) Serialize() []byte {
	name := truncateUTF8(m.Name, 127)
	buf := make([]byte, 1+2+2+2+1+len(name)+2+len(m.ReportDesc))
	buf[0] = ControlMsgTypeUhidCreate
	Write16BE(buf[1:], m.ID)
	Write16BE(buf[3:], m.VendorID)
	Write16BE(buf[5:], m.ProductID)
	idx := 7
	buf[idx] = byte(len(name))
	idx++
	copy(buf[idx:], name)
	idx += len(name)
	Write16BE(buf[idx:], uint16(len(m.ReportDesc)))
	idx += 2
	copy(buf[idx:], m.ReportDesc)
	return buf
}

// UhidInput — type 13, variable length.
type UhidInput struct {
	ID   uint16
	Data []byte // max 15 bytes
}

func (m *UhidInput) Serialize() []byte {
	buf := make([]byte, 1+2+2+len(m.Data))
	buf[0] = ControlMsgTypeUhidInput
	Write16BE(buf[1:], m.ID)
	Write16BE(buf[3:], uint16(len(m.Data)))
	copy(buf[5:], m.Data)
	return buf
}

// UhidDestroy — type 14, 3 bytes.
type UhidDestroy struct {
	ID uint16
}

func (m *UhidDestroy) Serialize() []byte {
	buf := make([]byte, 3)
	buf[0] = ControlMsgTypeUhidDestroy
	Write16BE(buf[1:], m.ID)
	return buf
}

// StartApp — type 16, variable length.
type StartApp struct {
	Name string // max 255 bytes
}

func (m *StartApp) Serialize() []byte {
	name := truncateUTF8(m.Name, 255)
	buf := make([]byte, 1+1+len(name))
	buf[0] = ControlMsgTypeStartApp
	buf[1] = byte(len(name))
	copy(buf[2:], name)
	return buf
}

// --- helpers ---

func truncateUTF8(s string, maxLen int) string {
	b := []byte(s)
	if len(b) <= maxLen {
		return s
	}
	// truncate at a valid UTF-8 boundary
	for maxLen > 0 && b[maxLen]>>6 == 0b10 {
		maxLen--
	}
	return string(b[:maxLen])
}

func clampF(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
