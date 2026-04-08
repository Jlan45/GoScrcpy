package client

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os/exec"
	"strconv"
	"time"

	"github.com/Jlan45/GoScrcpy/adb"
	"github.com/Jlan45/GoScrcpy/protocol"
)

const deviceNameFieldLength = 64

// Client is the high-level scrcpy client that manages the full lifecycle:
// push server, establish tunnel, connect sockets, demux streams, control device.
type Client struct {
	opts Options

	deviceName string
	serverCmd  *exec.Cmd

	// Tunnel state
	tunnelForward bool
	tunnelPort    uint16
	socketName    string
	scid          uint32

	// Sockets
	videoConn   net.Conn
	audioConn   net.Conn
	controlConn net.Conn

	// Listener for reverse tunnel
	listener net.Listener

	// Sub-components
	videoDemuxer *Demuxer
	audioDemuxer *Demuxer
	controller   *Controller
}

// NewClient creates a new scrcpy client with the given options.
func NewClient(opts Options) *Client {
	return &Client{
		opts: opts,
		scid: rand.Uint32() & 0x7FFFFFFF, // Java parseInt requires signed 32-bit range
	}
}

// Start pushes the server, establishes the tunnel, connects sockets,
// and starts demuxers/controller. The context controls cancellation.
func (c *Client) Start(ctx context.Context) error {
	// 1. Push server jar
	if c.opts.ServerPath != "" {
		log.Printf("[client] pushing server jar: %s", c.opts.ServerPath)
		if err := adb.Push(c.opts.Serial, c.opts.ServerPath, adb.DefaultServerDevicePath); err != nil {
			return fmt.Errorf("push server: %w", err)
		}
	}

	// 2. Setup tunnel
	if err := c.setupTunnel(); err != nil {
		return fmt.Errorf("setup tunnel: %w", err)
	}
	log.Printf("[client] tunnel ready: forward=%v port=%d socketName=%s", c.tunnelForward, c.tunnelPort, c.socketName)

	// 3. Launch server
	params := c.buildServerParams()
	cmd, err := adb.StartServer(c.opts.Serial, c.opts.ServerVersion, params)
	if err != nil {
		c.closeTunnel()
		return err
	}
	c.serverCmd = cmd

	// Give server a moment to start
	time.Sleep(500 * time.Millisecond)

	// 4. Connect sockets
	log.Printf("[client] connecting sockets (video=%v audio=%v control=%v)...", c.opts.Video, c.opts.Audio, c.opts.Control)
	if err := c.connectSockets(ctx); err != nil {
		c.Stop()
		return fmt.Errorf("connect sockets: %w", err)
	}
	log.Printf("[client] sockets connected")

	// 5. Close tunnel (no longer needed after sockets connected)
	c.closeTunnel()

	// 6. Read device name from first socket
	if err := c.readDeviceName(); err != nil {
		c.Stop()
		return fmt.Errorf("read device name: %w", err)
	}
	log.Printf("[client] device name: %s", c.deviceName)

	// 7. Set TCP_NODELAY on control socket
	if c.controlConn != nil {
		if tc, ok := c.controlConn.(*net.TCPConn); ok {
			_ = tc.SetNoDelay(true)
		}
	}

	// 8. Start demuxers and controller
	if c.opts.Video && c.videoConn != nil {
		c.videoDemuxer = NewDemuxer("video", c.videoConn, 256)
		if err := c.videoDemuxer.Start(true); err != nil {
			c.Stop()
			return err
		}
		w, h := c.videoDemuxer.VideoSize()
		log.Printf("[client] video demuxer started: codec=%s %dx%d", protocol.CodecName(c.videoDemuxer.CodecID()), w, h)
	}
	if c.opts.Audio && c.audioConn != nil {
		c.audioDemuxer = NewDemuxer("audio", c.audioConn, 256)
		if err := c.audioDemuxer.Start(false); err != nil {
			c.Stop()
			return err
		}
		log.Printf("[client] audio demuxer started: codec=%s", protocol.CodecName(c.audioDemuxer.CodecID()))
	}
	if c.opts.Control && c.controlConn != nil {
		c.controller = NewController(c.controlConn)
		c.controller.Start()
		log.Printf("[client] controller started")
	}

	return nil
}

// Stop terminates the server process and closes all connections.
func (c *Client) Stop() {
	if c.videoConn != nil {
		c.videoConn.Close()
	}
	if c.audioConn != nil {
		c.audioConn.Close()
	}
	if c.controlConn != nil {
		c.controlConn.Close()
	}
	if c.serverCmd != nil && c.serverCmd.Process != nil {
		_ = c.serverCmd.Process.Kill()
		_ = c.serverCmd.Wait()
	}
	c.closeTunnel()
}

// DeviceName returns the device name received during handshake.
func (c *Client) DeviceName() string { return c.deviceName }

// VideoStream returns the video packet channel (nil if video disabled).
func (c *Client) VideoStream() <-chan *protocol.Packet {
	if c.videoDemuxer == nil {
		return nil
	}
	return c.videoDemuxer.Packets()
}

// AudioStream returns the audio packet channel (nil if audio disabled).
func (c *Client) AudioStream() <-chan *protocol.Packet {
	if c.audioDemuxer == nil {
		return nil
	}
	return c.audioDemuxer.Packets()
}

// VideoCodecID returns the negotiated video codec ID.
func (c *Client) VideoCodecID() uint32 {
	if c.videoDemuxer == nil {
		return 0
	}
	return c.videoDemuxer.CodecID()
}

// AudioCodecID returns the negotiated audio codec ID.
func (c *Client) AudioCodecID() uint32 {
	if c.audioDemuxer == nil {
		return 0
	}
	return c.audioDemuxer.CodecID()
}

// VideoSize returns the initial video dimensions.
func (c *Client) VideoSize() (width, height uint32) {
	if c.videoDemuxer == nil {
		return 0, 0
	}
	return c.videoDemuxer.VideoSize()
}

// SendControl sends a control message to the device.
func (c *Client) SendControl(msg protocol.ControlMsg) error {
	if c.controller == nil {
		return fmt.Errorf("control not enabled")
	}
	return c.controller.SendMsg(msg)
}

// DeviceMessages returns the device message channel (nil if control disabled).
func (c *Client) DeviceMessages() <-chan protocol.DeviceMsg {
	if c.controller == nil {
		return nil
	}
	return c.controller.DeviceMessages()
}

// --- private methods ---

func (c *Client) setupTunnel() error {
	c.socketName = fmt.Sprintf("scrcpy_%08x", c.scid)

	if c.opts.ForceADBForward {
		return c.setupForwardTunnel()
	}

	// Try reverse tunnel first.
	for port := c.opts.PortRange[0]; port <= c.opts.PortRange[1]; port++ {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			continue
		}
		remote := "localabstract:" + c.socketName
		local := fmt.Sprintf("tcp:%d", port)
		if err := adb.Reverse(c.opts.Serial, remote, local); err != nil {
			listener.Close()
			// Reverse failed, fall back to forward.
			return c.setupForwardTunnel()
		}
		c.listener = listener
		c.tunnelPort = port
		c.tunnelForward = false
		return nil
	}

	return c.setupForwardTunnel()
}

func (c *Client) setupForwardTunnel() error {
	remote := "localabstract:" + c.socketName
	for port := c.opts.PortRange[0]; port <= c.opts.PortRange[1]; port++ {
		local := fmt.Sprintf("tcp:%d", port)
		if err := adb.Forward(c.opts.Serial, local, remote); err != nil {
			continue
		}
		c.tunnelPort = port
		c.tunnelForward = true
		return nil
	}
	return fmt.Errorf("no available port in range %d-%d", c.opts.PortRange[0], c.opts.PortRange[1])
}

func (c *Client) closeTunnel() {
	if c.listener != nil {
		c.listener.Close()
		c.listener = nil
		remote := "localabstract:" + c.socketName
		_ = adb.RemoveReverse(c.opts.Serial, remote)
	}
	if c.tunnelForward && c.tunnelPort != 0 {
		local := fmt.Sprintf("tcp:%d", c.tunnelPort)
		_ = adb.RemoveForward(c.opts.Serial, local)
	}
}

func (c *Client) connectSockets(ctx context.Context) error {
	if c.tunnelForward {
		return c.connectSocketsForward(ctx)
	}
	return c.connectSocketsReverse(ctx)
}

func (c *Client) connectSocketsReverse(ctx context.Context) error {
	accept := func() (net.Conn, error) {
		if dl, ok := ctx.Deadline(); ok {
			c.listener.(*net.TCPListener).SetDeadline(dl)
		}
		conn, err := c.listener.Accept()
		if err != nil {
			return nil, fmt.Errorf("accept: %w", err)
		}
		return conn, nil
	}

	if c.opts.Video {
		conn, err := accept()
		if err != nil {
			return err
		}
		c.videoConn = conn
	}
	if c.opts.Audio {
		conn, err := accept()
		if err != nil {
			return err
		}
		c.audioConn = conn
	}
	if c.opts.Control {
		conn, err := accept()
		if err != nil {
			return err
		}
		c.controlConn = conn
	}
	return nil
}

func (c *Client) connectSocketsForward(ctx context.Context) error {
	host := c.opts.TunnelHost
	if host == "" {
		host = "127.0.0.1"
	}
	port := c.opts.TunnelPort
	if port == 0 {
		port = c.tunnelPort
	}
	addr := net.JoinHostPort(host, strconv.Itoa(int(port)))

	// In forward mode, the first connection reads a dummy byte.
	connectAndVerify := func() (net.Conn, error) {
		var conn net.Conn
		var err error
		for attempts := 100; attempts > 0; attempts-- {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			conn, err = net.DialTimeout("tcp", addr, 2*time.Second)
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			// Read dummy byte to verify server is listening.
			dummy := make([]byte, 1)
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, err = io.ReadFull(conn, dummy)
			conn.SetReadDeadline(time.Time{})
			if err != nil {
				conn.Close()
				time.Sleep(100 * time.Millisecond)
				continue
			}
			return conn, nil
		}
		return nil, fmt.Errorf("failed to connect to server at %s", addr)
	}

	connectDirect := func() (net.Conn, error) {
		conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			return nil, fmt.Errorf("connect to %s: %w", addr, err)
		}
		return conn, nil
	}

	// First socket uses connectAndVerify (reads dummy byte).
	first, err := connectAndVerify()
	if err != nil {
		return err
	}

	if c.opts.Video {
		c.videoConn = first
	} else if c.opts.Audio {
		c.audioConn = first
	} else if c.opts.Control {
		c.controlConn = first
	}

	// Subsequent sockets connect directly (no dummy byte).
	if c.opts.Audio && c.audioConn == nil {
		conn, err := connectDirect()
		if err != nil {
			return err
		}
		c.audioConn = conn
	}
	if c.opts.Control && c.controlConn == nil {
		conn, err := connectDirect()
		if err != nil {
			return err
		}
		c.controlConn = conn
	}
	return nil
}

func (c *Client) readDeviceName() error {
	// Device name is read from the first available socket.
	var conn net.Conn
	switch {
	case c.videoConn != nil:
		conn = c.videoConn
	case c.audioConn != nil:
		conn = c.audioConn
	case c.controlConn != nil:
		conn = c.controlConn
	default:
		return fmt.Errorf("no socket available to read device name")
	}

	buf := make([]byte, deviceNameFieldLength)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return fmt.Errorf("read device name: %w", err)
	}
	// Null-terminated string within 64 bytes.
	for i, b := range buf {
		if b == 0 {
			c.deviceName = string(buf[:i])
			return nil
		}
	}
	c.deviceName = string(buf)
	return nil
}

func (c *Client) buildServerParams() map[string]string {
	p := map[string]string{
		"scid":      fmt.Sprintf("%08x", c.scid),
		"log_level": "info",
	}

	if !c.opts.Video {
		p["video"] = "false"
	}
	if !c.opts.Audio {
		p["audio"] = "false"
	}
	if !c.opts.Control {
		p["control"] = "false"
	}

	if c.opts.VideoCodec != "" && c.opts.VideoCodec != "h264" {
		p["video_codec"] = c.opts.VideoCodec
	}
	if c.opts.AudioCodec != "" && c.opts.AudioCodec != "opus" {
		p["audio_codec"] = c.opts.AudioCodec
	}
	if c.opts.VideoBitRate != 0 {
		p["video_bit_rate"] = strconv.FormatUint(uint64(c.opts.VideoBitRate), 10)
	}
	if c.opts.AudioBitRate != 0 {
		p["audio_bit_rate"] = strconv.FormatUint(uint64(c.opts.AudioBitRate), 10)
	}
	if c.opts.MaxSize != 0 {
		p["max_size"] = strconv.Itoa(int(c.opts.MaxSize))
	}
	if c.opts.MaxFPS != "" {
		p["max_fps"] = c.opts.MaxFPS
	}
	if c.tunnelForward {
		p["tunnel_forward"] = "true"
	}
	if c.opts.ShowTouches {
		p["show_touches"] = "true"
	}
	if c.opts.StayAwake {
		p["stay_awake"] = "true"
	}
	if c.opts.PowerOffOnClose {
		p["power_off_on_close"] = "true"
	}
	if !c.opts.ClipboardAutoSync {
		p["clipboard_autosync"] = "false"
	}
	if !c.opts.PowerOn {
		p["power_on"] = "false"
	}

	return p
}
