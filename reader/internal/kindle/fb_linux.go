package kindle

import (
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

const fbioGetVScreenInfo = 0x4600

// fbInverted reports whether the hardware is inverting the display: the X
// driver applies epdcMode by setting fb_var_screeninfo.grayscale to 2
// (GRAYSCALE_8BIT_INVERTED) or 1 (GRAYSCALE_8BIT).
func fbInverted() (bool, error) {
	f, err := os.Open("/dev/fb0")
	if err != nil {
		return false, err
	}
	defer f.Close()
	var v [40]uint32 // struct fb_var_screeninfo; grayscale is the 8th field
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), fbioGetVScreenInfo, uintptr(unsafe.Pointer(&v[0]))); errno != 0 {
		return false, errno
	}
	return v[7] == 2, nil
}
