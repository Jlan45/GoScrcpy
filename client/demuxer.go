package client

import (
	"fmt"
	"io"
	"sync"

	"github.com/Jlan45/GoScrcpy/protocol"
)

// Demuxer reads a scrcpy media stream (video or audio) from a socket
// and delivers parsed packets.
type Demuxer struct {
	name   string
	reader io.Reader

	codecID uint32
	// Video only
	videoWidth  uint32
	videoHeight uint32

	packetCh chan *protocol.Packet
	done     chan struct{}
	once     sync.Once
}

// NewDemuxer creates a demuxer for the given stream.
// isVideo controls whether video dimensions are read after the codec ID.
func NewDemuxer(name string, reader io.Reader, bufSize int) *Demuxer {
	if bufSize <= 0 {
		bufSize = 256
	}
	return &Demuxer{
		name:     name,
		reader:   reader,
		packetCh: make(chan *protocol.Packet, bufSize),
		done:     make(chan struct{}),
	}
}

// Packets returns the channel delivering parsed packets.
func (d *Demuxer) Packets() <-chan *protocol.Packet {
	return d.packetCh
}

// CodecID returns the codec ID received from the server.
func (d *Demuxer) CodecID() uint32 { return d.codecID }

// VideoSize returns the initial video dimensions (only valid for video demuxer).
func (d *Demuxer) VideoSize() (width, height uint32) {
	return d.videoWidth, d.videoHeight
}

// Start begins reading packets in a goroutine. isVideo indicates whether
// to read video dimensions after the codec header.
func (d *Demuxer) Start(isVideo bool) error {
	// Read codec ID (4 bytes BE).
	codecBuf := make([]byte, 4)
	if _, err := io.ReadFull(d.reader, codecBuf); err != nil {
		return fmt.Errorf("demuxer %s: read codec id: %w", d.name, err)
	}
	d.codecID = protocol.Read32BE(codecBuf)

	if d.codecID == protocol.CodecDisabled {
		close(d.packetCh)
		close(d.done)
		return nil // stream disabled, not an error
	}
	if d.codecID == protocol.CodecConfigErr {
		close(d.packetCh)
		close(d.done)
		return fmt.Errorf("demuxer %s: stream configuration error on device", d.name)
	}

	// Video streams also send width + height (8 bytes).
	if isVideo {
		sizeBuf := make([]byte, 8)
		if _, err := io.ReadFull(d.reader, sizeBuf); err != nil {
			return fmt.Errorf("demuxer %s: read video size: %w", d.name, err)
		}
		d.videoWidth = protocol.Read32BE(sizeBuf[0:4])
		d.videoHeight = protocol.Read32BE(sizeBuf[4:8])
	}

	go func() {
		defer close(d.packetCh)
		defer close(d.done)

		for {
			pkt, err := protocol.ReadPacket(d.reader)
			if err != nil {
				return // stream ended
			}

			select {
			case d.packetCh <- pkt:
			case <-d.done:
				return
			}
		}
	}()

	return nil
}

// Stop signals the demuxer to stop.
func (d *Demuxer) Stop() {
	d.once.Do(func() {})
}

// Wait blocks until the demuxer goroutine exits.
func (d *Demuxer) Wait() {
	<-d.done
}
