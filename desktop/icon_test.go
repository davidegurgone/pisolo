package main

import (
	"bytes"
	"image/png"
	"testing"
)

func TestIcons(t *testing.T) {
	for _, m := range []Mode{Off, Awake, LidClosed} {
		img, err := png.Decode(bytes.NewReader(drawIcon(m)))
		if err != nil || img.Bounds().Dx() != 32 {
			t.Fatalf("mode %d: %v", m, err)
		}
	}
	ico := pngToICO(drawIcon(Awake), 32)
	if !bytes.Equal(ico[:6], []byte{0, 0, 1, 0, 1, 0}) {
		t.Fatal("bad ico header")
	}
}
