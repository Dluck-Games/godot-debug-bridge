package record

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireWindowLock_AndReclaimsStale(t *testing.T) {
	lockDir := filepath.Join(t.TempDir(), "win.lock")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Dead PID should be reclaimed immediately.
	if err := os.WriteFile(filepath.Join(lockDir, "pid"), []byte("9999999"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	lock, err := AcquireWindowLock(ctx, lockDir)
	if err != nil {
		t.Fatalf("AcquireWindowLock: %v", err)
	}
	defer lock.Unlock()

	// While held by our live PID, a second acquire with short timeout must time out.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel2()
	if _, err := AcquireWindowLock(ctx2, lockDir); err == nil {
		t.Fatal("expected second AcquireWindowLock to time out while held")
	}

	lock.Unlock()
	if _, err := os.Stat(lockDir); !os.IsNotExist(err) {
		t.Fatalf("lock directory still exists after Unlock: %v", err)
	}
}
