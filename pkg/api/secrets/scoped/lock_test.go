// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

//go:build unix

package scoped

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Concurrent read-modify-write cycles under LockStore lose nothing. Without the
// lock, the sleep between read and write makes them lose updates every run.
func TestLockStoreSerializesReadModifyWrite(t *testing.T) {
	scDir := filepath.Join(t.TempDir(), ".sc")
	counter := filepath.Join(t.TempDir(), "counter")
	if err := os.WriteFile(counter, []byte("0"), 0o600); err != nil {
		t.Fatal(err)
	}
	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := LockStore(scDir, nil)
			if err != nil {
				errs <- err
				return
			}
			defer release()
			data, _ := os.ReadFile(counter)
			v, _ := strconv.Atoi(string(data))
			time.Sleep(2 * time.Millisecond)
			errs <- writeFileAtomic(counter, []byte(strconv.Itoa(v+1)), 0o600)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if data, _ := os.ReadFile(counter); string(data) != strconv.Itoa(n) {
		t.Fatalf("counter = %s after %d locked increments", data, n)
	}
	// Nothing is left behind: the lock is on the directory itself.
	if entries, _ := os.ReadDir(scDir); len(entries) != 0 {
		t.Errorf("the lock left %d file(s) in %s", len(entries), scDir)
	}
}

// Two repositories do not share a lock.
func TestLockStoreIsPerRepository(t *testing.T) {
	release, err := LockStore(filepath.Join(t.TempDir(), ".sc"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	other, err := LockStore(filepath.Join(t.TempDir(), ".sc"), nil)
	if err != nil {
		t.Fatalf("another repository's store waited for this one's lock: %v", err)
	}
	other()
}

// A held lock makes the next command give up after the timeout with a message,
// says it is waiting first, and releasing it lets the next one in. The wait runs
// in a goroutine so a broken deadline fails the test instead of hanging it.
func TestLockStoreTimesOutWithAMessage(t *testing.T) {
	prevT, prevN := lockTimeout, lockWaitNotice
	lockTimeout, lockWaitNotice = 400*time.Millisecond, 100*time.Millisecond
	t.Cleanup(func() { lockTimeout, lockWaitNotice = prevT, prevN })
	scDir := filepath.Join(t.TempDir(), ".sc")
	release, err := LockStore(scDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	waited := 0
	done := make(chan error, 1)
	go func() {
		_, err := LockStore(scDir, func() { waited++ })
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "another sc process") {
			t.Fatalf("second lock: %v; want a timeout naming the holder", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the deadline did not stop the wait")
	}
	if waited != 1 {
		t.Errorf("waiting notice called %d times; want once", waited)
	}
	release()
	again, err := LockStore(scDir, nil)
	if err != nil {
		t.Fatalf("lock not released: %v", err)
	}
	again()
}

func TestLockStoreTimeoutFromEnvironment(t *testing.T) {
	scDir := filepath.Join(t.TempDir(), ".sc")
	t.Setenv("SC_SCOPE_LOCK_TIMEOUT", "nonsense")
	if _, err := LockStore(scDir, nil); err == nil || !strings.Contains(err.Error(), "SC_SCOPE_LOCK_TIMEOUT") {
		t.Fatalf("bad timeout accepted: %v", err)
	}
	t.Setenv("SC_SCOPE_LOCK_TIMEOUT", "200ms")
	release, err := LockStore(scDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	start := time.Now()
	if _, err := LockStore(scDir, nil); err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("env timeout not used: %v after %s", err, time.Since(start))
	}
}

// Where the filesystem cannot lock a directory (NFS), a lock file in .sc is
// locked instead; any other lock error fails the command.
func TestLockStoreFallsBackToAFileAndFailsOnOtherErrors(t *testing.T) {
	prev := tryLockFn
	t.Cleanup(func() { tryLockFn = prev })
	scDir := filepath.Join(t.TempDir(), ".sc")
	tryLockFn = func(f *os.File) (bool, error) {
		if st, err := f.Stat(); err == nil && st.IsDir() {
			return false, syscall.ENOLCK
		}
		return prev(f)
	}
	release, err := LockStore(scDir, nil)
	if err != nil {
		t.Fatalf("fallback: %v", err)
	}
	if _, err := os.Stat(filepath.Join(scDir, lockFileName)); err != nil {
		t.Errorf("fallback lock file missing: %v", err)
	}
	release()

	tryLockFn = func(*os.File) (bool, error) { return false, syscall.EIO }
	if rel, err := LockStore(scDir, nil); err == nil || rel != nil {
		t.Fatalf("an I/O error took the lock: %v", err)
	}
}

// The lock excludes other processes, which is what parallel CI commands are.
func TestLockStoreExcludesOtherProcesses(t *testing.T) {
	if dir := os.Getenv("SC_TEST_LOCK_HOLDER"); dir != "" {
		release, err := LockStore(dir, nil)
		if err != nil {
			os.Exit(2)
		}
		os.Stdout.WriteString("locked\n")
		time.Sleep(3 * time.Second)
		release()
		os.Exit(0)
	}
	prev := lockTimeout
	lockTimeout = 500 * time.Millisecond
	t.Cleanup(func() { lockTimeout = prev })
	scDir := filepath.Join(t.TempDir(), ".sc")
	child := exec.Command(os.Args[0], "-test.run=^TestLockStoreExcludesOtherProcesses$")
	child.Env = append(os.Environ(), "SC_TEST_LOCK_HOLDER="+scDir)
	out, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	line, _ := bufio.NewReader(out).ReadString('\n')
	if strings.TrimSpace(line) != "locked" {
		t.Fatalf("child did not take the lock: %q", line)
	}
	if _, err := LockStore(scDir, nil); err == nil {
		t.Fatal("took the lock while another process held it")
	}
}
