package image

import (
	"bytes"
	"image"
	"strings"
	"testing"
)

func TestPlaceKittyCropped(t *testing.T) {
	var buf bytes.Buffer
	err := PlaceKittyCropped(&buf, 123, 10, 5, 0, 20, 100, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "\x1b_Gq=2,a=p,i=123,c=10,r=5,x=0,y=20,w=100,h=50\x1b\\"
	if buf.String() != expected {
		t.Errorf("expected %q, got %q", expected, buf.String())
	}
}

func TestWriteKittyChunks_Named_Uses_t(t *testing.T) {
	var buf bytes.Buffer
	// Test with id > 0. It should use a=t
	err := WriteKittyChunks(&buf, "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXo=", 10, 5, 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	// Must contain a=t (transmit only), NOT a=T
	if !strings.Contains(out, "a=t") {
		t.Errorf("named images must use a=t to avoid display flashes. Got: %q", out)
	}
	if strings.Contains(out, "a=T") {
		t.Errorf("named images must NOT use a=T. Got: %q", out)
	}
}

func TestWriteKittyChunks_Anonymous_Uses_T(t *testing.T) {
	var buf bytes.Buffer
	// Test with id = 0. It should use a=T
	err := WriteKittyChunks(&buf, "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXo=", 10, 5, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	// Must contain a=T (transmit and display)
	if !strings.Contains(out, "a=T") {
		t.Errorf("anonymous images must use a=T. Got: %q", out)
	}
}

func TestEncodeKittyPayload(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	payload, err := EncodeKittyPayload(img, 2, 2, 5, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(payload) == 0 {
		t.Errorf("expected non-empty payload")
	}
}
