package image

import (
	"bytes"
	"image"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) {
	return 0, errors.New("write")
}

type failAfterFirstWriteWriter struct {
	writes int
}

func (w *failAfterFirstWriteWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes >= 2 {
		return 0, errors.New("write later")
	}
	return len(p), nil
}

func TestIsKittySupported_Branches(t *testing.T) {
	oldTerm := os.Getenv("TERM")
	oldTermProgram := os.Getenv("TERM_PROGRAM")
	defer func() {
		os.Setenv("TERM", oldTerm)
		os.Setenv("TERM_PROGRAM", oldTermProgram)
	}()

	tests := []struct {
		name    string
		term    string
		program string
		want    bool
	}{
		{"xterm", "xterm-256color", "", false},
		{"kitty", "xterm-kitty", "", true},
		{"wezterm", "", "WezTerm", true},
		{"ghostty", "", "ghostty", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("TERM", tt.term)
			os.Setenv("TERM_PROGRAM", tt.program)
			if got := IsKittySupported(); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKitty_DeleteCommands(t *testing.T) {
	var buf bytes.Buffer
	
	DeleteKittyByID(&buf, 1)
	if !strings.Contains(buf.String(), "a=d,d=I,i=1") {
		t.Errorf("DeleteKittyByID failed: %q", buf.String())
	}
	buf.Reset()

	DeleteKittyPlacement(&buf, 2)
	if !strings.Contains(buf.String(), "a=d,d=i,i=2") {
		t.Errorf("DeleteKittyPlacement failed: %q", buf.String())
	}
	buf.Reset()

	DeleteAllKitty(&buf)
	if !strings.Contains(buf.String(), "a=d,d=A") {
		t.Errorf("DeleteAllKitty failed: %q", buf.String())
	}
}

func TestKitty_PlaceKitty(t *testing.T) {
	var buf bytes.Buffer
	PlaceKitty(&buf, 10, 2, 3)
	if !strings.Contains(buf.String(), "a=p,i=10,c=2,r=3") {
		t.Errorf("PlaceKitty failed: %q", buf.String())
	}
}

func TestEncodeKitty_Error(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	err := EncodeKitty(&bytes.Buffer{}, img, 0, 1, 1, 1) // targetW=0
	if err == nil {
		t.Error("expected error for invalid target size")
	}

	if err := EncodeKitty(failWriter{}, img, 1, 1, 1, 1); err == nil {
		t.Fatal("expected writer error from EncodeKitty")
	}
}

func TestEncodeKittyPayload_EncodeFailure(t *testing.T) {
	oldPNGEncode := pngEncode
	t.Cleanup(func() { pngEncode = oldPNGEncode })
	pngEncode = func(io.Writer, image.Image) error {
		return errors.New("encode fail")
	}

	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if _, err := EncodeKittyPayload(img, 1, 1, 1, 1); err == nil || !strings.Contains(err.Error(), "png encode") {
		t.Fatalf("expected wrapped png encode failure, got %v", err)
	}
}

func TestWriteKittyChunks_MultiChunk(t *testing.T) {
	var buf bytes.Buffer
	// Create data > kittyChunkSize (4096)
	data := strings.Repeat("a", 5000)
	err := WriteKittyChunks(&buf, data, 1, 1, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	// Should see m=1 then m=0
	if !strings.Contains(buf.String(), "m=1") || !strings.Contains(buf.String(), "m=0") {
		t.Errorf("expected multi-chunk output, got %q", buf.String())
	}
}

func TestResizeImage_Empty(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 0, 0))
	got := resizeImage(img, 10, 10)
	if got != img {
		t.Error("expected original image for empty bounds")
	}
}

func TestWriteKittyChunks_Error(t *testing.T) {
	if err := WriteKittyChunks(failWriter{}, "abcd", 1, 1, 1); err == nil {
		t.Fatal("expected writer error from WriteKittyChunks")
	}
}

func TestWriteKittyChunks_ErrorOnContinuationChunk(t *testing.T) {
	writer := &failAfterFirstWriteWriter{}
	data := strings.Repeat("b", kittyChunkSize+16)
	if err := WriteKittyChunks(writer, data, 2, 2, 99); err == nil {
		t.Fatal("expected continuation chunk write to fail")
	}
}
