package client

// Options configures the scrcpy client connection.
type Options struct {
	// Serial is the ADB device serial (empty for single device).
	Serial string

	// ServerPath is the local path to the scrcpy-server jar file.
	ServerPath string

	// ServerVersion must match the server's BuildConfig.VERSION_NAME.
	ServerVersion string

	Video   bool // enable video stream (default true)
	Audio   bool // enable audio stream (default true)
	Control bool // enable device control (default true)

	VideoCodec string // "h264" (default), "h265", "av1"
	AudioCodec string // "opus" (default), "aac", "flac", "raw"

	VideoBitRate uint32 // default 8000000
	AudioBitRate uint32 // default 128000
	MaxSize      uint16 // max video dimension, 0 = no limit
	MaxFPS       string // float string, e.g. "60"

	// Tunnel settings.
	ForceADBForward bool
	PortRange       [2]uint16 // default [27183, 27199]
	TunnelHost      string    // default "127.0.0.1"
	TunnelPort      uint16    // 0 = auto from tunnel

	// Server behavior.
	ShowTouches    bool
	StayAwake      bool
	PowerOffOnClose bool
	ClipboardAutoSync bool // default true
	PowerOn           bool // default true
}

// DefaultOptions returns options with sensible defaults.
func DefaultOptions() Options {
	return Options{
		ServerVersion:     "3.1",
		Video:             true,
		Audio:             true,
		Control:           true,
		VideoCodec:        "h264",
		AudioCodec:        "opus",
		VideoBitRate:      8_000_000,
		AudioBitRate:      128_000,
		PortRange:         [2]uint16{27183, 27199},
		TunnelHost:        "127.0.0.1",
		ClipboardAutoSync: true,
		PowerOn:           true,
	}
}
