package protocol

import (
	"fmt"
	"io"
)

const PacketHeaderSize = 12

const (
	PacketFlagConfig   uint64 = 1 << 63
	PacketFlagKeyFrame uint64 = 1 << 62
	PacketPTSMask      uint64 = PacketFlagKeyFrame - 1
)

// Packet represents a single demuxed media packet from the scrcpy stream.
type Packet struct {
	PTS        int64  // presentation timestamp in microseconds (62-bit)
	IsConfig   bool   // codec config data (SPS/PPS for H.26x)
	IsKeyFrame bool
	Data       []byte
}

// ReadPacket reads one packet from the stream.
func ReadPacket(r io.Reader) (*Packet, error) {
	header := make([]byte, PacketHeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("read packet header: %w", err)
	}

	ptsFlags := Read64BE(header[0:8])
	size := Read32BE(header[8:12])
	if size == 0 {
		return nil, fmt.Errorf("invalid zero-length packet")
	}

	data := make([]byte, size)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("read packet data (%d bytes): %w", size, err)
	}

	pkt := &Packet{
		IsConfig:   ptsFlags&PacketFlagConfig != 0,
		IsKeyFrame: ptsFlags&PacketFlagKeyFrame != 0,
		Data:       data,
	}
	if pkt.IsConfig {
		pkt.PTS = -1 // AV_NOPTS_VALUE equivalent
	} else {
		pkt.PTS = int64(ptsFlags & PacketPTSMask)
	}
	return pkt, nil
}
