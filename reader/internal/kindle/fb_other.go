//go:build !linux

package kindle

import "errors"

func fbInverted() (bool, error) { return false, errors.New("no framebuffer") }
