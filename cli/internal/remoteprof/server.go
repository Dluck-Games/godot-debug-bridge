package remoteprof

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

// Serve handles one debugger session and publishes the completed report from a
// temporary file in the destination directory.
func Serve(ctx context.Context, port int, out string, ready func(int) error) error {
	l, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		return err
	}
	defer l.Close()
	dir := filepath.Dir(out)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".gdbg-profile-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	stop := context.AfterFunc(ctx, func() { l.Close() })
	defer stop()
	if err = ready(l.Addr().(*net.TCPAddr).Port); err != nil {
		return err
	}
	c := newCollector()
	conn, err := l.Accept()
	if err == nil {
		err = collect(ctx, conn, c)
	}
	if ctx.Err() != nil || errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNRESET) {
		err = nil
	}
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if writeErr := enc.Encode(map[string]any{"phases": c.finish()}); writeErr != nil {
		return errors.Join(err, writeErr)
	}
	if closeErr := file.Close(); closeErr != nil {
		return errors.Join(err, closeErr)
	}
	renameErr := os.Rename(file.Name(), out)
	if renameErr != nil {
		// Some filesystems require removing an existing report before replacement.
		if info, statErr := os.Lstat(out); statErr == nil && info.Mode().IsRegular() {
			if removeErr := os.Remove(out); removeErr != nil {
				return errors.Join(err, renameErr, removeErr)
			}
			renameErr = os.Rename(file.Name(), out)
		}
	}
	return errors.Join(err, renameErr)
}
func collect(ctx context.Context, conn net.Conn, c *collector) error {
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	for _, msg := range [][]any{{"profiler:servers", 1, []any{true, []any{128, false}}}, {"profiler:visual", 1, []any{true}}} {
		if err := writePacket(conn, msg); err != nil {
			return err
		}
	}
	r := bufio.NewReader(conn)
	for {
		b, err := readPacket(r)
		if err != nil {
			return err
		}
		if err := c.message(b); err != nil {
			return fmt.Errorf("decode profiler message: %w", err)
		}
	}
}
