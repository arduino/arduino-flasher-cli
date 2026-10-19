// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package updater

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"syscall"

	"github.com/arduino/go-paths-helper"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/klauspost/compress/zstd"

	"github.com/arduino/arduino-flasher-cli/cmd/feedback"
	"github.com/arduino/arduino-flasher-cli/cmd/i18n"
)

const (
	// Archive entries bigger than this are not extracted but served from the archive.
	streamThreshold = 64 * 1024 * 1024
	// How much already-decompressed data is kept to serve out of order reads.
	streamWindow = 32 * 1024 * 1024
	streamChunk  = 1024 * 1024
)

// StreamImage extracts only the small files of a .tar.zst image into dest. The
// big disk images are exposed through a read-only FUSE mount that decompresses
// them on the fly, and linked from where the extraction would have put them.
// The returned function unmounts the filesystem.
func StreamImage(ctx context.Context, archive, dest *paths.Path) (func(), error) {
	feedback.Print(i18n.Tr("Indexing image archive (big files will be streamed, not extracted)"))

	mnt, err := paths.MkTempDir("", "arduino-flasher-stream-")
	if err != nil {
		return nil, err
	}

	var big []*entryStream
	err = walkArchive(archive, func(index int, hdr *tar.Header, tr *tar.Reader) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		name, ok := safeEntryName(hdr.Name)
		if !ok {
			return fmt.Errorf("unsafe path in archive: %q", hdr.Name)
		}
		target := dest.Join(name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			return target.MkdirAll()
		case tar.TypeReg:
			if err := target.Parent().MkdirAll(); err != nil {
				return err
			}
			if hdr.Size <= streamThreshold {
				return writeEntry(target, tr)
			}
			s := &entryStream{archive: archive, index: index, size: hdr.Size, mountName: fmt.Sprintf("%d-%s", index, path.Base(name))}
			big = append(big, s)
			feedback.Printf("  %s (%d bytes, streamed)", name, hdr.Size)
			return os.Symlink(mnt.Join(s.mountName).String(), target.String())
		}
		return nil
	})
	if err != nil {
		_ = mnt.Remove()
		return nil, fmt.Errorf("could not index archive: %w", err)
	}

	root := &streamRoot{files: big}
	server, err := fs.Mount(mnt.String(), root, &fs.Options{
		MountOptions: fuse.MountOptions{FsName: "arduino-flasher", Name: "imagestream"},
	})
	if err != nil {
		_ = mnt.Remove()
		return nil, fmt.Errorf("could not mount the streaming filesystem (is FUSE available?): %w", err)
	}

	return func() {
		_ = server.Unmount()
		for _, s := range big {
			s.close()
		}
		_ = mnt.Remove()
	}, nil
}

// safeEntryName rejects names escaping the extraction directory.
func safeEntryName(name string) (string, bool) {
	clean := path.Clean("/" + name)[1:]
	if clean == "" {
		return ".", true
	}
	if strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || clean != path.Clean(name) {
		return "", false
	}
	return clean, true
}

func writeEntry(target *paths.Path, r io.Reader) error {
	f, err := target.Create()
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func walkArchive(archive *paths.Path, cb func(index int, hdr *tar.Header, tr *tar.Reader) error) error {
	f, err := archive.Open()
	if err != nil {
		return err
	}
	defer f.Close()
	dec, err := zstd.NewReader(f)
	if err != nil {
		return err
	}
	defer dec.Close()
	tr := tar.NewReader(dec)
	for i := 0; ; i++ {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := cb(i, hdr, tr); err != nil {
			return err
		}
	}
}

// entryStream gives random access to an archive entry, decompressing forward
// and restarting from the beginning of the archive when asked to go back.
type entryStream struct {
	archive   *paths.Path
	index     int
	size      int64
	mountName string

	mu       sync.Mutex
	file     *os.File
	dec      *zstd.Decoder
	tr       *tar.Reader
	pos      int64  // entry offset of the next byte the decoder yields
	buf      []byte // data in [bufStart, pos)
	bufStart int64
}

func (s *entryStream) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLocked()
}

func (s *entryStream) closeLocked() {
	if s.dec != nil {
		s.dec.Close()
	}
	if s.file != nil {
		_ = s.file.Close()
	}
	s.file, s.dec, s.tr, s.buf = nil, nil, nil, nil
	s.pos, s.bufStart = 0, 0
}

func (s *entryStream) restartLocked() error {
	s.closeLocked()
	f, err := s.archive.Open()
	if err != nil {
		return err
	}
	dec, err := zstd.NewReader(f)
	if err != nil {
		_ = f.Close()
		return err
	}
	s.file, s.dec = f, dec
	tr := tar.NewReader(dec)
	for i := 0; i <= s.index; i++ {
		if _, err := tr.Next(); err != nil {
			s.closeLocked()
			return fmt.Errorf("could not seek to entry %d: %w", s.index, err)
		}
	}
	s.tr = tr
	return nil
}

// rewind makes sure the next read from the start is served without delay, so
// the time to skip earlier entries is spent before flashing begins.
func (s *entryStream) rewind() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tr != nil && s.bufStart == 0 {
		return nil
	}
	return s.restartLocked()
}

func (s *entryStream) ReadAt(p []byte, off int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if off >= s.size {
		return 0, io.EOF
	}
	end := min(off+int64(len(p)), s.size)

	if s.tr == nil || off < s.bufStart {
		if err := s.restartLocked(); err != nil {
			return 0, err
		}
	}
	if off > s.pos+streamWindow {
		if _, err := io.CopyN(io.Discard, s.tr, off-s.pos); err != nil {
			return 0, err
		}
		s.pos, s.bufStart, s.buf = off, off, s.buf[:0]
	}
	for s.pos < end {
		if len(s.buf) > 2*streamWindow {
			drop := min(s.pos-streamWindow, off) - s.bufStart
			if drop > 0 {
				s.buf = s.buf[:copy(s.buf, s.buf[drop:])]
				s.bufStart += drop
			}
		}
		n := min(int64(streamChunk), s.size-s.pos)
		l := len(s.buf)
		s.buf = slices.Grow(s.buf, int(n))[:l+int(n)]
		if _, err := io.ReadFull(s.tr, s.buf[l:]); err != nil {
			s.closeLocked()
			return 0, err
		}
		s.pos += n
	}
	return copy(p, s.buf[off-s.bufStart:end-s.bufStart]), nil
}

type streamRoot struct {
	fs.Inode
	files []*entryStream
}

var _ = (fs.NodeOnAdder)((*streamRoot)(nil))

func (r *streamRoot) OnAdd(ctx context.Context) {
	for _, s := range r.files {
		ch := r.NewPersistentInode(ctx, &streamFile{s: s}, fs.StableAttr{Mode: syscall.S_IFREG})
		r.AddChild(s.mountName, ch, false)
	}
}

type streamFile struct {
	fs.Inode
	s *entryStream
}

var (
	_ = (fs.NodeGetattrer)((*streamFile)(nil))
	_ = (fs.NodeOpener)((*streamFile)(nil))
	_ = (fs.NodeReader)((*streamFile)(nil))
)

func (f *streamFile) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = syscall.S_IFREG | 0444
	out.Size = uint64(f.s.size)
	return 0
}

func (f *streamFile) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	if flags&(syscall.O_WRONLY|syscall.O_RDWR) != 0 {
		return nil, 0, syscall.EROFS
	}
	if err := f.s.rewind(); err != nil {
		feedback.Warning(err.Error())
		return nil, 0, syscall.EIO
	}
	return nil, fuse.FOPEN_KEEP_CACHE, 0
}

func (f *streamFile) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	n, err := f.s.ReadAt(dest, off)
	if err != nil && !errors.Is(err, io.EOF) {
		feedback.Warning(err.Error())
		return nil, syscall.EIO
	}
	return fuse.ReadResultData(dest[:n]), 0
}
