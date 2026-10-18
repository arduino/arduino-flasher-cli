// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPublicationLockAcrossProcesses(t *testing.T) {
	const helperEnv = "ARDUINO_FLASHER_TEST_LOCK_DIRECTORY"
	if dir := os.Getenv(helperEnv); dir != "" {
		lock, err := lockPublication(context.Background(), dir)
		require.NoError(t, err)
		fmt.Println("locked")
		_, _ = io.Copy(io.Discard, os.Stdin)
		runtime.KeepAlive(lock)
		// Exit without closing the file explicitly: the OS must release the
		// lock even when a download process exits without running its defers.
		os.Exit(0)
	}

	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	executable, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestPublicationLockAcrossProcesses$")
	cmd.Env = append(os.Environ(), helperEnv+"="+dir)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = stdin.Close()
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	reader := bufio.NewScanner(stdout)
	require.True(t, reader.Scan(), "helper failed to acquire the lock")
	require.Equal(t, "locked", reader.Text())

	waitCtx, stop := context.WithTimeout(ctx, 100*time.Millisecond)
	defer stop()
	blocked, err := lockPublication(waitCtx, dir)
	if blocked != nil {
		_ = blocked.Close()
	}
	require.ErrorIs(t, err, context.DeadlineExceeded, "another process must not publish while the helper owns the lock")
	require.NoError(t, stdin.Close())
	require.NoError(t, cmd.Wait())

	lock, err := lockPublication(ctx, dir)
	require.NoError(t, err, "process exit must release the lock without deleting its file")
	require.NoError(t, lock.Close())
	info, err := os.Stat(filepath.Join(dir, publicationLockFile))
	require.NoError(t, err)
	require.Zero(t, info.Size(), "publication locks must not consume image storage")
}

func TestPublicationLockCanceledBeforeOpen(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	lock, err := lockPublication(ctx, dir)
	require.Nil(t, lock)
	require.ErrorIs(t, err, context.Canceled)
	require.NoFileExists(t, filepath.Join(dir, publicationLockFile))
}
