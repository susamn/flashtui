// Package flash writes an image to a block device and optionally reads it back
// to verify.
package flash

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"time"

	"github.com/susamn/flashtui/internal/imagefile"
	"github.com/susamn/flashtui/internal/privilege"
)

// Phase is the stage a job has reached.
type Phase int

const (
	PhaseIdle Phase = iota
	PhaseWriting
	PhaseFlushing
	PhaseVerifying
	PhaseDone
)

func (p Phase) String() string {
	switch p {
	case PhaseWriting:
		return "writing"
	case PhaseFlushing:
		return "flushing"
	case PhaseVerifying:
		return "verifying"
	case PhaseDone:
		return "done"
	default:
		return "idle"
	}
}

// Progress is one sample of an in-flight job.
type Progress struct {
	Phase   Phase
	Bytes   int64 // bytes moved in the current phase
	Total   int64 // expected bytes, 0 when the image length is unknown
	Elapsed time.Duration
	Rate    float64 // bytes per second, smoothed
}

// Fraction returns completion in [0,1], and false when the total is unknown and
// the caller must render an indeterminate bar.
func (p Progress) Fraction() (float64, bool) {
	if p.Total <= 0 {
		return 0, false
	}
	f := float64(p.Bytes) / float64(p.Total)
	if f > 1 {
		f = 1
	}
	return f, true
}

// ETA returns the projected remaining time, and false when it cannot be
// estimated yet.
func (p Progress) ETA() (time.Duration, bool) {
	if p.Total <= 0 || p.Rate <= 0 || p.Bytes <= 0 {
		return 0, false
	}
	remaining := p.Total - p.Bytes
	if remaining <= 0 {
		return 0, true
	}
	secs := float64(remaining) / p.Rate
	// Truncated to whole seconds: sub-second precision in an ETA is noise.
	return time.Duration(secs) * time.Second, true
}

// Job describes one flash.
type Job struct {
	Image imagefile.Image
	// Target is the whole-disk path, e.g. /dev/sda.
	Target string
	// TargetSize is the device's capacity in bytes, used to refuse a write
	// that cannot fit before any data is sent. Zero skips the check.
	TargetSize int64
	Verify     bool
}

// directRead controls iflag=direct on the verify read, which bypasses the page
// cache so the comparison sees what actually landed on the device rather than
// what was written through it. Tests targeting a regular file turn it off,
// because O_DIRECT is unsupported on tmpfs.
var directRead = true

// blockSize matches what dd is told to use. 4 MiB keeps the syscall count low
// without making the progress bar jump in visibly large steps.
const blockSize = 4 << 20

// ErrTargetTooSmall is returned before any write happens.
var ErrTargetTooSmall = errors.New("image is larger than the target device")

// Run executes the job, emitting Progress on updates. It closes updates before
// returning. The caller must not write to updates.
func Run(ctx context.Context, esc *privilege.Escalator, job Job, updates chan<- Progress) error {
	defer close(updates)

	// Refuse up front rather than filling the device and failing with ENOSPC
	// partway through, which leaves an unbootable half-written disk.
	if need := job.Image.WriteSize(); need > 0 && job.TargetSize > 0 && need > job.TargetSize {
		return fmt.Errorf("%w: image needs %d bytes, %s holds %d",
			ErrTargetTooSmall, need, job.Target, job.TargetSize)
	}

	written, sum, err := write(ctx, esc, job, updates)
	if err != nil {
		return err
	}
	if job.Verify {
		if err := verify(ctx, esc, job, written, sum, updates); err != nil {
			return err
		}
	}
	send(updates, Progress{Phase: PhaseDone, Bytes: written, Total: written})
	return nil
}

// write streams the decompressed image into dd and returns the byte count and
// SHA-256 of what was sent.
func write(ctx context.Context, esc *privilege.Escalator, job Job, updates chan<- Progress) (int64, []byte, error) {
	src, err := job.Image.Reader()
	if err != nil {
		return 0, nil, err
	}
	defer src.Close()

	// conv=fsync makes dd flush before exiting, so a clean exit means the data
	// reached the device rather than the page cache.
	cmd, err := esc.Command(ctx, "dd",
		"of="+job.Target,
		fmt.Sprintf("bs=%d", blockSize),
		"conv=fsync",
		"status=none",
	)
	if err != nil {
		return 0, nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return 0, nil, err
	}
	stderr := &tailBuffer{limit: 4 << 10}
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return 0, nil, fmt.Errorf("start dd: %w", err)
	}

	h := sha256.New()
	n, copyErr := pump(ctx, stdin, src, h, job.Image.WriteSize(), PhaseWriting, updates)
	// Closing stdin is what tells dd to finish and fsync; it must happen even
	// on a copy error so Wait cannot block forever.
	closeErr := stdin.Close()

	send(updates, Progress{Phase: PhaseFlushing, Bytes: n, Total: job.Image.WriteSize()})
	waitErr := cmd.Wait()

	switch {
	case copyErr != nil:
		return n, nil, copyErr
	case waitErr != nil:
		if privilege.ErrDenied(waitErr) {
			return n, nil, errors.New("authorisation denied; nothing was written")
		}
		return n, nil, fmt.Errorf("dd: %w: %s", waitErr, stderr.String())
	case closeErr != nil:
		return n, nil, fmt.Errorf("closing dd stdin: %w", closeErr)
	}
	return n, h.Sum(nil), nil
}

// verify reads the written range back off the device and compares digests.
func verify(ctx context.Context, esc *privilege.Escalator, job Job, written int64, want []byte, updates chan<- Progress) error {
	blocks := (written + blockSize - 1) / blockSize

	args := []string{
		"if=" + job.Target,
		fmt.Sprintf("bs=%d", blockSize),
		fmt.Sprintf("count=%d", blocks),
		"status=none",
	}
	if directRead {
		args = append(args, "iflag=direct")
	}
	cmd, err := esc.Command(ctx, "dd", args...)
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr := &tailBuffer{limit: 4 << 10}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start verify read: %w", err)
	}

	h := sha256.New()
	// The final block is padded up to blockSize by dd, so only the first
	// `written` bytes take part in the digest.
	n, copyErr := pump(ctx, io.Discard, io.LimitReader(stdout, written), h, written, PhaseVerifying, updates)
	// Drain the padding so dd is never killed by SIGPIPE on a short read.
	_, _ = io.Copy(io.Discard, stdout)
	waitErr := cmd.Wait()

	if copyErr != nil {
		return copyErr
	}
	if waitErr != nil {
		return fmt.Errorf("verify read: %w: %s", waitErr, stderr.String())
	}
	if n != written {
		return fmt.Errorf("verify read %d bytes, expected %d", n, written)
	}
	if got := h.Sum(nil); !bytes.Equal(got, want) {
		return fmt.Errorf("verification failed: device contents differ from the image")
	}
	return nil
}

// pump copies src to dst, feeding every byte through h, and emits throttled
// progress. It returns the number of bytes copied.
func pump(ctx context.Context, dst io.Writer, src io.Reader, h hash.Hash, total int64, phase Phase, updates chan<- Progress) (int64, error) {
	buf := make([]byte, blockSize)
	var n int64
	start := time.Now()
	lastSend := time.Now()
	var rate float64

	for {
		select {
		case <-ctx.Done():
			return n, ctx.Err()
		default:
		}

		read, rerr := src.Read(buf)
		if read > 0 {
			chunk := buf[:read]
			h.Write(chunk)
			if _, werr := dst.Write(chunk); werr != nil {
				return n, fmt.Errorf("write to device: %w", werr)
			}
			n += int64(read)

			// Throttle to roughly 20 Hz; a 4 MiB block at 100 MB/s arrives
			// faster than a terminal can usefully repaint.
			if now := time.Now(); now.Sub(lastSend) >= 50*time.Millisecond {
				elapsed := now.Sub(start)
				rate = smooth(rate, float64(n)/elapsed.Seconds())
				send(updates, Progress{
					Phase: phase, Bytes: n, Total: total,
					Elapsed: elapsed, Rate: rate,
				})
				lastSend = now
			}
		}
		if rerr == io.EOF {
			elapsed := time.Since(start)
			if elapsed > 0 {
				rate = float64(n) / elapsed.Seconds()
			}
			send(updates, Progress{Phase: phase, Bytes: n, Total: total, Elapsed: elapsed, Rate: rate})
			return n, nil
		}
		if rerr != nil {
			return n, fmt.Errorf("read image: %w", rerr)
		}
	}
}

// smooth is an exponential moving average, so a momentary stall does not make
// the displayed rate and ETA jump.
func smooth(prev, sample float64) float64 {
	if prev == 0 {
		return sample
	}
	const alpha = 0.3
	return prev*(1-alpha) + sample*alpha
}

// send never blocks: a slow consumer drops samples instead of stalling the
// write.
func send(ch chan<- Progress, p Progress) {
	select {
	case ch <- p:
	default:
	}
}

// tailBuffer keeps only the last limit bytes written to it, so a chatty command
// cannot balloon memory while still leaving the useful part of its error.
type tailBuffer struct {
	limit int
	buf   []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.limit {
		t.buf = t.buf[len(t.buf)-t.limit:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string { return string(t.buf) }
