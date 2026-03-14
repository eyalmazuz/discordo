//go:build linux || freebsd

package clipboard

import (
	"bytes"
	"github.com/ayn2op/clipboard"
	"os"
	"os/exec"
)

var wayland bool

var (
	osLookupEnv    = os.LookupEnv
	execLookPath   = exec.LookPath
	execCommand    = exec.Command
	clipboardInit  = clipboard.Init
	clipboardRead  = clipboard.Read
	clipboardWrite = clipboard.Write
)

func Init() error {
	if _, ok := osLookupEnv("WAYLAND_DISPLAY"); !ok {
		return clipboardInit()
	}
	if _, err := execLookPath("wl-copy"); err != nil {
		return err
	}
	if _, err := execLookPath("wl-paste"); err != nil {
		return err
	}
	wayland = true
	return nil
}

func Read(t Format) ([]byte, error) {
	if !wayland {
		return clipboardRead(clipboard.Format(t))
	}
	// -n: Don't print a newline at the end
	// -t type: MIME type specifier
	cmd := execCommand("wl-paste", "-nt", formatToType(t))
	outBuffer := bytes.Buffer{}
	cmd.Stdout = &outBuffer
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return outBuffer.Bytes(), nil
}

func Write(t Format, buf []byte) error {
	if !wayland {
		return clipboardWrite(clipboard.Format(t), buf)
	}
	// -t type: MIME type specifier
	cmd := execCommand("wl-copy", "-t", formatToType(t))
	cmd.Stdin = bytes.NewReader(buf)
	return cmd.Run()
}

func formatToType(t Format) string {
	switch t {
	case FmtImage:
		return "image"
	case FmtText:
		fallthrough
	default:
		return "text/plain;charset=utf-8"
	}
}
