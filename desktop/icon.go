package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"runtime"
)

// Tray icons are drawn in code: a cup, empty (off) or full (on),
// with a laptop base under it for the lid-closed mode.
var iconCache = map[Mode][]byte{}

func trayIcon(m Mode) []byte {
	if b, ok := iconCache[m]; ok {
		return b
	}
	b := drawIcon(m)
	if runtime.GOOS == "windows" {
		b = pngToICO(b, 32)
	}
	iconCache[m] = b
	return b
}

func drawIcon(m Mode) []byte {
	const s = 32
	img := image.NewNRGBA(image.Rect(0, 0, s, s))
	ink := color.NRGBA{235, 235, 235, 255} // light: visible on dark taskbars and panels
	if m == Off {
		ink = color.NRGBA{160, 160, 160, 255}
	}
	set := func(x, y int) { img.Set(x, y, ink) }

	// Cup body: x 6..21, y 9..23 (2px outline, filled when on)
	for y := 9; y <= 23; y++ {
		for x := 6; x <= 21; x++ {
			edge := x <= 7 || x >= 20 || y >= 22
			if m != Off || edge {
				set(x, y)
			}
		}
	}
	// Handle: ring on the right
	for y := 11; y <= 19; y++ {
		for x := 22; x <= 27; x++ {
			dx, dy := float64(x)-22, float64(y)-15
			d := dx*dx + dy*dy
			if d >= 9 && d <= 22 {
				set(x, y)
			}
		}
	}
	// Steam when on
	if m != Off {
		for _, x := range []int{10, 14, 18} {
			for y := 2; y <= 6; y++ {
				if (y+x)%3 != 0 {
					set(x+(y%2), y)
				}
			}
		}
	}
	// Laptop base for lid-closed mode
	if m == LidClosed {
		for y := 27; y <= 29; y++ {
			for x := 1; x <= 30; x++ {
				set(x, y)
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// pngToICO wraps a PNG in an .ico container (Windows Vista and later read PNG icons).
func pngToICO(p []byte, size int) []byte {
	var buf bytes.Buffer
	w := func(v any) { _ = binary.Write(&buf, binary.LittleEndian, v) }
	w(uint16(0))      // reserved
	w(uint16(1))      // type: icon
	w(uint16(1))      // image count
	w(uint8(size))    // width
	w(uint8(size))    // height
	w(uint8(0))       // palette
	w(uint8(0))       // reserved
	w(uint16(1))      // color planes
	w(uint16(32))     // bits per pixel
	w(uint32(len(p))) // data size
	w(uint32(6 + 16)) // data offset
	buf.Write(p)
	return buf.Bytes()
}
