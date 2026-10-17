// Package input reads touch and key events.
//
// The evdev backend talks to Linux input devices directly. It is only
// exercised on the Kindle, but compiles everywhere so desktop builds and
// tests don't need build tags.
package input

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Event types and codes from linux/input-event-codes.h.
const (
	EvSyn = 0x00
	EvKey = 0x01
	EvAbs = 0x03

	SynReport = 0x00

	KeyPower   = 116
	BtnTouch   = 0x14a
	AbsX       = 0x00
	AbsY       = 0x01
	AbsMTSlot  = 0x2f
	AbsMTPosX  = 0x35
	AbsMTPosY  = 0x36
	AbsMTTrack = 0x39
)

// RawEvent is one struct input_event.
type RawEvent struct {
	Sec, Usec int64
	Type      uint16
	Code      uint16
	Value     int32
}

// AbsInfo mirrors struct input_absinfo.
type AbsInfo struct {
	Value, Minimum, Maximum, Fuzz, Flat, Resolution int32
}

// Device is an open evdev node.
type Device struct {
	Path string
	Name string
	f    *os.File
}

func ioc(dir, typ, nr, size uintptr) uintptr {
	return dir<<30 | size<<16 | typ<<8 | nr
}

const iocRead = 2

// Open opens an evdev node and reads its name.
func Open(path string) (*Device, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 256)
	req := ioc(iocRead, 'E', 0x06, uintptr(len(buf)))
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), req, uintptr(unsafe.Pointer(&buf[0]))); errno != 0 {
		f.Close()
		return nil, fmt.Errorf("EVIOCGNAME %s: %w", path, errno)
	}
	name := strings.TrimRight(string(buf), "\x00")
	return &Device{Path: path, Name: name, f: f}, nil
}

// FindByName returns the first /dev/input/event* device whose name matches.
func FindByName(names ...string) (*Device, error) {
	paths, _ := filepath.Glob("/dev/input/event*")
	for _, p := range paths {
		d, err := Open(p)
		if err != nil {
			continue
		}
		for _, n := range names {
			if d.Name == n {
				return d, nil
			}
		}
		d.Close()
	}
	return nil, fmt.Errorf("no input device named %v", names)
}

// Abs returns the range of an absolute axis.
func (d *Device) Abs(code uintptr) (AbsInfo, error) {
	var ai AbsInfo
	req := ioc(iocRead, 'E', 0x40+code, unsafe.Sizeof(ai))
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, d.f.Fd(), req, uintptr(unsafe.Pointer(&ai))); errno != 0 {
		return ai, errno
	}
	return ai, nil
}

// Read blocks for the next event. struct input_event uses a native-width
// timeval, so its size depends on the architecture (16 bytes on 32-bit ARM).
func (d *Device) Read() (RawEvent, error) {
	const long = int(unsafe.Sizeof(uintptr(0)))
	buf := make([]byte, 2*long+8)
	if _, err := d.f.Read(buf); err != nil {
		return RawEvent{}, err
	}
	var ev RawEvent
	if long == 4 {
		ev.Sec = int64(int32(binary.LittleEndian.Uint32(buf[0:])))
		ev.Usec = int64(int32(binary.LittleEndian.Uint32(buf[4:])))
	} else {
		ev.Sec = int64(binary.LittleEndian.Uint64(buf[0:]))
		ev.Usec = int64(binary.LittleEndian.Uint64(buf[8:]))
	}
	o := 2 * long
	ev.Type = binary.LittleEndian.Uint16(buf[o:])
	ev.Code = binary.LittleEndian.Uint16(buf[o+2:])
	ev.Value = int32(binary.LittleEndian.Uint32(buf[o+4:]))
	return ev, nil
}

// Close closes the device.
func (d *Device) Close() error { return d.f.Close() }
