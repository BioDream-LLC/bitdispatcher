package drone

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"os"
	"path/filepath"
	"time"
)

// Recorder writes a drone's video to a file, one clip at a time, and says at checkpoints how long the
// file is and the SHA-256 of its bytes so far; the caller puts that in the ledger. A clip starts at a key
// frame, with the MP4 header first, so every clip plays on its own.
type Recorder struct {
	Path    string
	f       *os.File
	sum     hash.Hash
	Size    int64
	Start   time.Time
	End     time.Time
	Codec   string
	written bool
}

// Checkpoint is a recorder's state at a moment.
type Checkpoint struct {
	Size   int64
	SHA256 string
	Start  time.Time
	End    time.Time
}

// NewRecorder opens a new clip; it fails if the file exists, so a recording is never overwritten.
func NewRecorder(path string, s *Stream) (*Recorder, error) {
	if s == nil {
		return nil, errors.New("no video to record")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return nil, err
	}
	r := &Recorder{Path: path, f: f, sum: sha256.New(), Codec: s.Codec}
	if err := r.write(s.Init); err != nil {
		f.Close()
		return nil, err
	}
	return r, nil
}

func (r *Recorder) write(b []byte) error {
	n, err := r.f.Write(b)
	r.sum.Write(b[:n])
	r.Size += int64(n)
	return err
}

// Add writes a fragment. The first must be a key frame; until one comes, fragments are skipped.
func (r *Recorder) Add(f Fragment) error {
	if !r.written {
		if !f.Key {
			return nil
		}
		r.written, r.Start = true, f.At
	}
	r.End = f.At
	return r.write(f.Data)
}

// Started is whether any picture is in the clip yet.
func (r *Recorder) Started() bool { return r.written }

// Checkpoint flushes the file to disk and gives its length and hash.
func (r *Recorder) Checkpoint() (Checkpoint, error) {
	err := r.f.Sync()
	return Checkpoint{Size: r.Size, SHA256: hex.EncodeToString(r.sum.Sum(nil)), Start: r.Start, End: r.End}, err
}

// Close ends the clip and gives its last checkpoint.
func (r *Recorder) Close() (Checkpoint, error) {
	c, err := r.Checkpoint()
	if cerr := r.f.Close(); err == nil {
		err = cerr
	}
	return c, err
}
