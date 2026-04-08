package client

import (
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/Jlan45/GoScrcpy/protocol"
)

// Controller manages sending control messages and receiving device messages
// over the control socket.
type Controller struct {
	conn net.Conn

	deviceMsgCh chan protocol.DeviceMsg
	done        chan struct{}
	mu          sync.Mutex
}

// NewController creates a controller for the given control socket.
func NewController(conn net.Conn) *Controller {
	return &Controller{
		conn:        conn,
		deviceMsgCh: make(chan protocol.DeviceMsg, 64),
		done:        make(chan struct{}),
	}
}

// SendMsg serializes and sends a control message to the device.
func (c *Controller) SendMsg(msg protocol.ControlMsg) error {
	data := msg.Serialize()
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.conn.Write(data)
	return err
}

// DeviceMessages returns the channel delivering device messages.
func (c *Controller) DeviceMessages() <-chan protocol.DeviceMsg {
	return c.deviceMsgCh
}

// Start begins the receiver goroutine that reads device messages.
func (c *Controller) Start() {
	go c.receiveLoop()
}

func (c *Controller) receiveLoop() {
	defer close(c.deviceMsgCh)
	defer close(c.done)

	buf := make([]byte, protocol.DeviceMsgMaxSize)
	head := 0

	for {
		n, err := c.conn.Read(buf[head:])
		if err != nil {
			if err != io.EOF {
				_ = fmt.Errorf("controller recv: %w", err)
			}
			return
		}
		head += n

		for {
			msg, consumed, err := protocol.DeserializeDeviceMsg(buf[:head])
			if err != nil {
				return // unrecoverable
			}
			if consumed == 0 {
				break // need more data
			}
			select {
			case c.deviceMsgCh <- msg:
			case <-c.done:
				return
			}
			head -= consumed
			if head > 0 {
				copy(buf, buf[consumed:consumed+head])
			}
		}
	}
}

// Wait blocks until the receiver goroutine exits.
func (c *Controller) Wait() {
	<-c.done
}
