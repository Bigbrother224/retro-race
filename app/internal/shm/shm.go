// Package shm implements a cross-process shared-memory channel between the
// player process (consumer) and a headless rival process (producer).
//
// The rival runs its own libretro core on the same ROM, publishes its RGBA
// framebuffer and race state to a fixed-size mmap'd file, and the player reads
// it for the live PiP window and the real progress/finish. This is the
// product's "one process per player" architecture: two processes each own a
// core instance (the libretro C shim is single-instance per process).
//
// A seqlock-style frame counter protects against torn reads: the producer
// writes the payload, then bumps `frame`; the reader snapshots `frame` before
// and after and accepts only matching, advanced frames.
package shm

import (
	"encoding/binary"
	"errors"
	"os"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Layout of the shared region (fixed so producer and consumer map the same
// size without a handshake).
const (
	// HeaderSize is the fixed header byte length.
	HeaderSize = 128
	// FrameMaxBytes is the largest RGBA framebuffer we publish (covers all
	// supported 8/16-bit systems comfortably; 512x448x4).
	FrameMaxBytes = 512 * 448 * 4
	// FrameStart is the byte offset of the framebuffer region.
	FrameStart = HeaderSize
	// RegionSize is the total mapped size.
	RegionSize = HeaderSize + FrameMaxBytes
)

// Header field offsets (little-endian).
const (
	offMagic    = 0
	offVersion  = 4
	offFrame    = 8
	offState    = 12
	offWidth    = 16
	offHeight   = 20
	offFPS      = 24
	offProgress = 28 // 0..1000 (per mille)
	offFinish   = 32 // frame at which the rival finished, or 0
	offReserved = 36
)

// State values (producer -> consumer).
const (
	StateIdle    uint32 = 0
	StateRacing  uint32 = 1
	StateDone    uint32 = 2
	StateAborted uint32 = 3
)

const (
	// Magic identifies a valid region.
	Magic = 0x52524652 // "RRFR"
	// Version of the layout.
	Version = 1
)

// Producer is the rival side: it writes a frame and bumps the seqlock.
type Producer struct {
	data []byte
	path string
}

// OpenProducer creates (or truncates) the shared region at path and maps it
// read-write. Call Close when done.
func OpenProducer(path string) (*Producer, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	if err := f.Truncate(RegionSize); err != nil {
		f.Close()
		return nil, err
	}
	b, err := unix.Mmap(int(f.Fd()), 0, RegionSize, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	f.Close()
	if err != nil {
		return nil, err
	}
	// Stamp the header once.
	binary.LittleEndian.PutUint32(b[offMagic:], Magic)
	binary.LittleEndian.PutUint32(b[offVersion:], Version)
	binary.LittleEndian.PutUint32(b[offFrame:], 0)
	binary.LittleEndian.PutUint32(b[offState:], StateIdle)
	return &Producer{data: b, path: path}, nil
}

// Close unmaps the region.
func (p *Producer) Close() {
	if p.data != nil {
		unix.Munmap(p.data)
		p.data = nil
	}
}

// Write publishes one frame: copies the RGBA framebuffer, sets the header
// metadata, then bumps the frame counter (release) so readers see a complete
// frame. framebuffer must be len(frameWidth*frameHeight*4).
func (p *Producer) Write(frame []byte, width, height int, state uint32, progress uint32, fps uint32, finishFrame uint32) {
	if p.data == nil {
		return
	}
	// Copy framebuffer (clamp to region).
	n := len(frame)
	if n > FrameMaxBytes {
		n = FrameMaxBytes
	}
	dst := p.data[FrameStart : FrameStart+n]
	copy(dst, frame[:n])

	binary.LittleEndian.PutUint32(p.data[offWidth:], uint32(width))
	binary.LittleEndian.PutUint32(p.data[offHeight:], uint32(height))
	binary.LittleEndian.PutUint32(p.data[offFPS:], fps)
	binary.LittleEndian.PutUint32(p.data[offProgress:], progress)
	binary.LittleEndian.PutUint32(p.data[offFinish:], finishFrame)
	binary.LittleEndian.PutUint32(p.data[offState:], state)

	// Publish: bump frame last (release order).
	frameNo := atomic.LoadUint32((*uint32)(unsafePointer(&p.data[offFrame]))) + 1
	atomic.StoreUint32((*uint32)(unsafePointer(&p.data[offFrame])), frameNo)
}

// SetState publishes a state change without a new frame (e.g. Done).
func (p *Producer) SetState(state uint32, progress uint32, finishFrame uint32) {
	if p.data == nil {
		return
	}
	binary.LittleEndian.PutUint32(p.data[offProgress:], progress)
	binary.LittleEndian.PutUint32(p.data[offFinish:], finishFrame)
	binary.LittleEndian.PutUint32(p.data[offState:], state)
	frameNo := atomic.LoadUint32((*uint32)(unsafePointer(&p.data[offFrame]))) + 1
	atomic.StoreUint32((*uint32)(unsafePointer(&p.data[offFrame])), frameNo)
}

// Consumer is the player side: it reads the rival's latest frame.
type Consumer struct {
	data []byte
	last uint32
	fb   []byte // stable copy of the latest framebuffer
}

// Snapshot is one consumed rival frame.
type Snapshot struct {
	Frame       []byte // RGBA, valid until the next Take
	Width       int
	Height      int
	State       uint32
	Progress    uint32
	FinishFrame uint32
	FPS         uint32
}

// OpenConsumer maps the shared region at path read-write (mmap requires at
// least one of the read flags; we only read). Returns an error if the region
// does not exist or is not yet created by the producer.
func OpenConsumer(path string) (*Consumer, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	b, err := unix.Mmap(int(f.Fd()), 0, RegionSize, unix.PROT_READ, unix.MAP_SHARED)
	f.Close()
	if err != nil {
		return nil, err
	}
	if binary.LittleEndian.Uint32(b[offMagic:]) != Magic {
		unix.Munmap(b)
		return nil, errors.New("shm: bad magic (region not from producer)")
	}
	c := &Consumer{data: b, fb: make([]byte, FrameMaxBytes)}
	return c, nil
}

// Close unmaps the region.
func (c *Consumer) Close() {
	if c.data != nil {
		unix.Munmap(c.data)
		c.data = nil
	}
}

// Take reads the latest complete frame. Returns a non-nil snapshot when a new
// frame has been published since the last Take. The returned Frame is a copy
// owned by the consumer (safe to hold across subsequent Tokes).
func (c *Consumer) Take() *Snapshot {
	if c.data == nil {
		return nil
	}
	framePtr := (*uint32)(unsafePointer(&c.data[offFrame]))
	f1 := atomic.LoadUint32(framePtr)
	if f1 == c.last {
		return nil
	}
	// Read the whole payload once; verify the seqlock is stable.
	// We copy framebuffer first, then re-check the counter.
	n := int(binary.LittleEndian.Uint32(c.data[offWidth:]) * binary.LittleEndian.Uint32(c.data[offHeight:]) * 4)
	if n <= 0 || n > FrameMaxBytes {
		n = 0
	}
	fb := c.fb[:0]
	if n > 0 {
		fb = c.fb[:n]
		copy(fb, c.data[FrameStart:FrameStart+n])
	}
	f2 := atomic.LoadUint32(framePtr)
	if f1 != f2 {
		// Torn read; try once more (rare). If still torn, skip.
		c.last = f2
		return nil
	}
	if f1 == c.last {
		return nil
	}
	c.last = f1
	return &Snapshot{
		Frame:       fb,
		Width:       int(binary.LittleEndian.Uint32(c.data[offWidth:])),
		Height:      int(binary.LittleEndian.Uint32(c.data[offHeight:])),
		State:       binary.LittleEndian.Uint32(c.data[offState:]),
		Progress:    binary.LittleEndian.Uint32(c.data[offProgress:]),
		FinishFrame: binary.LittleEndian.Uint32(c.data[offFinish:]),
		FPS:         binary.LittleEndian.Uint32(c.data[offFPS:]),
	}
}

// LastState returns the current state without consuming a frame.
func (c *Consumer) LastState() uint32 {
	if c.data == nil {
		return StateIdle
	}
	return binary.LittleEndian.Uint32(c.data[offState:])
}

// unsafePointer bridges a byte slice element to an atomic uint32.
func unsafePointer(b *byte) unsafe.Pointer { return unsafe.Pointer(b) }
