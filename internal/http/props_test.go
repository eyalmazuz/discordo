package http

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestIdentifyProperties(t *testing.T) {
	props := IdentifyProperties()
	if props["os"] != OS {
		t.Errorf("expected OS %q, got %q", OS, props["os"])
	}
	if props["browser"] != Browser {
		t.Errorf("expected Browser %q, got %q", Browser, props["browser"])
	}
}

func TestGetSuperProps(t *testing.T) {
	encoded, err := getSuperProps()
	if err != nil {
		t.Fatalf("getSuperProps failed: %v", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}

	var props map[string]any
	if err := json.Unmarshal(decoded, &props); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}

	if props["os"] != OS {
		t.Errorf("expected OS %q in super props, got %v", OS, props["os"])
	}
	if _, ok := props["is_fast_connect"]; ok {
		t.Error("expected is_fast_connect to be deleted from super props")
	}
}

func TestGenerateLaunchSignature(t *testing.T) {
	sig := generateLaunchSignature()
	if len(sig) == 0 {
		t.Error("expected non-empty launch signature")
	}
}
