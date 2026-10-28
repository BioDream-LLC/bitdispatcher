package drone

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
	"github.com/pion/rtp"
)

// A drone's picture arrives as H.264 over RTSP, the way drones' cameras and video radios offer it. The
// Hub turns each frame into a fragment of an MP4 file the moment it arrives (no waiting for a whole group
// of pictures), so a browser plays it through Media Source Extensions about as fast as it comes in, over
// the same HTTPS connection as everything else. A recording is the same bytes written to a file, which
// any video player opens.

const timeScale = 90000

// Fragment is one frame of video as an MP4 fragment.
type Fragment struct {
	Data []byte
	Key  bool // starts a group of pictures: a player can start here
	At   time.Time
	PTS  time.Duration // from the start of the stream
}

// Stream is one run of video with one set of parameters; a change of picture size starts a new one.
type Stream struct {
	Init  []byte // the MP4 header
	Codec string // for the browser: avc1.42c01e
	Start time.Time
}

// Hub carries one drone's picture to everyone watching and to the recorder.
type Hub struct {
	mu      sync.Mutex
	stream  *Stream
	gop     []Fragment // since the last key frame, for a new viewer to start at once
	subs    map[chan Fragment]*Stream
	lastAt  time.Time
	frames  uint64
	lastErr string
}

// NewHub makes an empty hub.
func NewHub() *Hub { return &Hub{subs: map[chan Fragment]*Stream{}} }

// Live is whether frames came in during the last 3 seconds.
func (h *Hub) Live() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.stream != nil && time.Since(h.lastAt) < 3*time.Second
}

// Status is a word on the video for the console.
func (h *Hub) Status() (live bool, frames uint64, lastErr string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.stream != nil && time.Since(h.lastAt) < 3*time.Second, h.frames, h.lastErr
}

// Subscribe gives the current stream (nil when there is none), the fragments from its last key frame,
// and a channel of the ones that follow. The channel is closed when the stream ends or the watcher is
// too slow; cancel stops watching.
func (h *Hub) Subscribe() (*Stream, []Fragment, <-chan Fragment, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stream == nil {
		return nil, nil, nil, func() {}
	}
	c := make(chan Fragment, 256)
	h.subs[c] = h.stream
	gop := append([]Fragment(nil), h.gop...)
	return h.stream, gop, c, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[c]; ok {
			delete(h.subs, c)
			close(c)
		}
	}
}

func (h *Hub) begin(s *Stream) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.endLocked()
	h.stream, h.gop = s, nil
}

func (h *Hub) end(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err != nil {
		h.lastErr = err.Error()
	}
	h.endLocked()
}

func (h *Hub) endLocked() {
	for c := range h.subs {
		close(c)
		delete(h.subs, c)
	}
	h.stream, h.gop = nil, nil
}

func (h *Hub) publish(f Fragment) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastAt, h.lastErr = f.At, ""
	h.frames++
	if f.Key {
		h.gop = h.gop[:0]
	}
	if len(h.gop) < 600 {
		h.gop = append(h.gop, f)
	}
	for c := range h.subs {
		select {
		case c <- f:
		default: // too slow: it starts again from a key frame
			close(c)
			delete(h.subs, c)
		}
	}
}

// Feed takes H.264 access units (each a set of NAL units, one picture) with their presentation time and
// publishes them. It is what the RTSP reader calls, and what tests call directly.
type Feed struct {
	hub      *Hub
	sps, pps []byte
	dts      *h264.DTSExtractor
	first    int64
	started  bool
	seq      uint32
	lastDur  uint32
	lastDTS  int64
	haveLast bool
}

// NewFeed starts feeding a hub.
func NewFeed(h *Hub) *Feed { return &Feed{hub: h} }

// Push adds one access unit, with its presentation time in 90 kHz ticks.
func (f *Feed) Push(au [][]byte, pts int64) error {
	var sps, pps []byte
	var key bool
	for _, n := range au {
		if len(n) == 0 {
			continue
		}
		switch h264.NALUType(n[0] & 0x1f) {
		case h264.NALUTypeSPS:
			sps = n
		case h264.NALUTypePPS:
			pps = n
		case h264.NALUTypeIDR:
			key = true
		}
	}
	if sps != nil && pps != nil && (!bytesEqual(sps, f.sps) || !bytesEqual(pps, f.pps)) {
		if err := f.restart(sps, pps); err != nil {
			return err
		}
	}
	if !f.started {
		if !key || f.sps == nil {
			return nil // wait for a key frame with its parameters
		}
		f.started, f.first = true, pts
		f.dts = &h264.DTSExtractor{}
		f.dts.Initialize()
		f.haveLast = false
	}
	pts -= f.first
	dts, err := f.dts.Extract(au, pts)
	if err != nil {
		if key {
			f.dts.Initialize()
			dts = pts
		} else {
			return nil
		}
	}
	// Drop the parameter sets and delimiters from the samples: they are in the MP4 header.
	var nal [][]byte
	for _, n := range au {
		if len(n) == 0 {
			continue
		}
		switch h264.NALUType(n[0] & 0x1f) {
		case h264.NALUTypeSPS, h264.NALUTypePPS, h264.NALUTypeAccessUnitDelimiter:
			continue
		}
		nal = append(nal, n)
	}
	if len(nal) == 0 {
		return nil
	}
	if f.haveLast && dts > f.lastDTS && dts-f.lastDTS < timeScale {
		f.lastDur = uint32(dts - f.lastDTS)
	}
	if f.lastDur == 0 {
		f.lastDur = timeScale / 30
	}
	f.lastDTS, f.haveLast = dts, true
	s, err := fmp4.NewSampleH264(int32(pts-dts), nal)
	if err != nil {
		return err
	}
	s.Duration = f.lastDur
	f.seq++
	part := fmp4.Part{SequenceNumber: f.seq, Tracks: []*fmp4.PartTrack{{ID: 1, BaseTime: uint64(max(dts, 0)), Samples: []*fmp4.Sample{s}}}}
	var buf seekablebuffer.Buffer
	if err := part.Marshal(&buf); err != nil {
		return err
	}
	f.hub.publish(Fragment{Data: buf.Bytes(), Key: key, At: time.Now(), PTS: time.Duration(pts) * time.Second / timeScale})
	return nil
}

func (f *Feed) restart(sps, pps []byte) error {
	var p h264.SPS
	if err := p.Unmarshal(sps); err != nil {
		return fmt.Errorf("the drone's video has an unreadable SPS: %w", err)
	}
	init := fmp4.Init{Tracks: []*fmp4.InitTrack{{ID: 1, TimeScale: timeScale, Codec: &fmp4.CodecH264{SPS: sps, PPS: pps}}}}
	var buf seekablebuffer.Buffer
	if err := init.Marshal(&buf); err != nil {
		return err
	}
	f.sps, f.pps = append([]byte(nil), sps...), append([]byte(nil), pps...)
	f.started, f.seq = false, 0
	f.hub.begin(&Stream{Init: buf.Bytes(), Codec: fmt.Sprintf("avc1.%02x%02x%02x", sps[1], sps[2], sps[3]), Start: time.Now()})
	return nil
}

// End says the video stopped.
func (f *Feed) End(err error) {
	f.sps, f.pps, f.started = nil, nil, false
	f.hub.end(err)
}

func bytesEqual(a, b []byte) bool { return string(a) == string(b) }

// ReadRTSP plays an RTSP address into a hub until ctx ends, connecting again after a failure. TCP is
// used for the video (it gets through firewalls and over cellular links that drop UDP).
func ReadRTSP(ctx context.Context, address string, h *Hub) {
	for ctx.Err() == nil {
		err := readOnce(ctx, address, h)
		if ctx.Err() != nil {
			return
		}
		h.end(err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func readOnce(ctx context.Context, address string, h *Hub) error {
	u, err := base.ParseURL(address)
	if err != nil {
		return fmt.Errorf("the video address is not an RTSP address: %w", err)
	}
	tcp := gortsplib.ProtocolTCP
	c := gortsplib.Client{Scheme: u.Scheme, Host: u.Host, Protocol: &tcp, ReadTimeout: 8 * time.Second, WriteTimeout: 8 * time.Second}
	if err := c.Start(); err != nil {
		return err
	}
	defer c.Close()
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			c.Close()
		case <-stop:
		}
	}()
	desc, _, err := c.Describe(u)
	if err != nil {
		return cleanErr(err, address)
	}
	var forma *format.H264
	medi := desc.FindFormat(&forma)
	if medi == nil {
		return errors.New("the drone's video is not H.264")
	}
	dec, err := forma.CreateDecoder()
	if err != nil {
		return err
	}
	if _, err := c.Setup(desc.BaseURL, medi, 0, 0); err != nil {
		return cleanErr(err, address)
	}
	feed := NewFeed(h)
	// The parameters may come only in the description, not in the stream.
	params := [][]byte{}
	if forma.SPS != nil && forma.PPS != nil {
		params = [][]byte{forma.SPS, forma.PPS}
	}
	c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
		pts, ok := c.PacketPTS(medi, pkt)
		if !ok {
			return
		}
		au, err := dec.Decode(pkt)
		if err != nil {
			if !errors.Is(err, rtph264.ErrNonStartingPacketAndNoPrevious) && !errors.Is(err, rtph264.ErrMorePacketsNeeded) {
				h.mu.Lock()
				h.lastErr = err.Error()
				h.mu.Unlock()
			}
			return
		}
		if h264.IsRandomAccess(au) && len(params) > 0 {
			au = append(append([][]byte{}, params...), au...)
		}
		// The RTSP clock is 90 kHz for video.
		feed.Push(au, pts)
	})
	if _, err := c.Play(nil); err != nil {
		return cleanErr(err, address)
	}
	err = c.Wait()
	feed.End(nil)
	return cleanErr(err, address)
}

// cleanErr keeps a password in the video address out of messages.
func cleanErr(err error, address string) error {
	if err == nil {
		return nil
	}
	u, perr := base.ParseURL(address)
	msg := err.Error()
	if perr == nil && u.User != nil {
		if p, ok := u.User.Password(); ok && p != "" {
			msg = strings.ReplaceAll(msg, p, "••••")
		}
	}
	return errors.New(msg)
}
