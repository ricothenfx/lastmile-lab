// Package replay — perekam sesi untuk Replay Engine (Fase 5).
//
// Recorder menyimpan ring frame snapshot berbudget tetap (default 15 menit
// @ 5 Hz) di memori rider-sim: setiap frame di-marshal JSON sekali lalu
// dikompresi gzip per frame (blob ~2 KB dari ~13 KB @ 100 rider) sehingga
// ring penuh ≈ 10 MB — bukan 60 MB. Blob per-frame adalah anggota gzip
// mandiri; JSON array respons dirakit dengan menggabungkan anggota
// (header, blob, ",", blob, … footer) di bawah Content-Encoding: gzip —
// multi-member gzip stream valid dan didekompresi transparan oleh browser.
//
// Keputusan dispatch ikut direkam (dedupe by seq, bounded) supaya inspect
// rider bisa menampilkan ALASAN keputusan pada titik waktu scrub.
//
// Kontrak model.Snapshot TIDAK berubah: frame disimpan apa adanya; semua
// tipe respons baru hidup hanya di endpoint /api/replay/*.
package replay

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"sync"

	"github.com/ricothenfx/lastmile-lab/apps/services/pkg/model"
)

// Batas default ring (hitung RAM: frame ~2 KB gzip → ≤ ~10 MB + keputusan
// ~2 MB — dihitung eksplisit di reports/phase-05-replay.md).
const (
	DefaultHz         = 5                   // frame per detik (tick engine 10 Hz ÷ 2)
	DefaultMaxFrames  = 15 * 60 * DefaultHz // 4500 frame = 15 menit
	DefaultMaxBytes   = 24 << 20            // 24 MiB budget blob (kedua pagar, mana yang kena dulu)
	DefaultMaxDecides = 8000                // ≈ 2 MB struct
	gzipLevel         = gzip.BestSpeed      // 5 Hz × 13 KB — CPU ~1%, rasio ~6×
)

// Meta identitas sesi — metadata perekaman (seed, strategi, armada).
type Meta struct {
	ID            string `json:"id"`
	Seed          int64  `json:"seed"`
	Strategy      string `json:"strategy"`
	Riders        int    `json:"riders"`
	TickHz        int    `json:"tick_hz"`
	StartedWallMs int64  `json:"started_wall_ms"`
	Hz            int    `json:"hz"`
	MaxSeconds    int    `json:"max_seconds"`
}

type frameRec struct {
	T      int64  // sim time ms (dari snapshot)
	WallMs int64  // wall clock ms saat direkam (pemeta incident → sim t)
	GZ     []byte // blob gzip mandiri berisi JSON snapshot
}

// Recorder ring frame + keputusan. Aman untuk satu goroutine penulis
// (ticker perekam) dan banyak pembaca HTTP.
type Recorder struct {
	mu            sync.Mutex
	meta          Meta
	frames        []frameRec
	frameBytes    int
	maxFrames     int
	maxBytes      int
	decisions     []model.Decision // ascending by seq/t
	maxDecides    int
	lastDecSeq    uint64
	droppedFrames uint64
}

func New(meta Meta) *Recorder {
	if meta.ID == "" {
		meta.ID = "live"
	}
	if meta.Hz <= 0 {
		meta.Hz = DefaultHz
	}
	if meta.MaxSeconds <= 0 {
		meta.MaxSeconds = DefaultMaxFrames / DefaultHz
	}
	return &Recorder{
		meta:       meta,
		maxFrames:  DefaultMaxFrames,
		maxBytes:   DefaultMaxBytes,
		maxDecides: DefaultMaxDecides,
	}
}

// gzBytes mengompresi satu payload menjadi anggota gzip mandiri.
func gzBytes(p []byte) []byte {
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzipLevel)
	_, _ = zw.Write(p)
	_ = zw.Close()
	return buf.Bytes()
}

// Record menyimpan satu snapshot (dipanggil ticker 200 ms = 5 Hz).
func (r *Recorder) Record(snap model.Snapshot, wallMs int64) {
	raw, err := json.Marshal(snap)
	if err != nil {
		return
	}
	rec := frameRec{T: snap.T, WallMs: wallMs, GZ: gzBytes(raw)}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frames = append(r.frames, rec)
	r.frameBytes += len(rec.GZ)
	for (len(r.frames) > r.maxFrames || r.frameBytes > r.maxBytes) && len(r.frames) > 1 {
		r.frameBytes -= len(r.frames[0].GZ)
		r.frames = r.frames[1:]
		r.droppedFrames++
	}
}

// RecordDecisions menyerap ring keputusan engine (terbaru di depan, maks 25)
// dengan dedupe by seq — hanya seq lebih besar dari yang terakhir dicatat.
func (r *Recorder) RecordDecisions(ds []model.Decision) {
	if len(ds) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// ds terbaru-di-depan → iterasi mundur supaya append ascending.
	for i := len(ds) - 1; i >= 0; i-- {
		d := ds[i]
		if d.Seq <= r.lastDecSeq {
			continue
		}
		r.lastDecSeq = d.Seq
		r.decisions = append(r.decisions, d)
	}
	if n := len(r.decisions); n > r.maxDecides {
		r.decisions = append([]model.Decision(nil), r.decisions[n-r.maxDecides:]...)
	}
}

// SessionInfo ringkasan satu sesi rekaman (untuk GET /api/replay/sessions).
type SessionInfo struct {
	ID          string `json:"id"`
	Frames      int    `json:"frames"`
	FrameBytes  int    `json:"frame_bytes"`
	TFirst      int64  `json:"t_first"`
	TLast       int64  `json:"t_last"`
	WallFirstMs int64  `json:"wall_first_ms"`
	WallLastMs  int64  `json:"wall_last_ms"`
	Seconds     int    `json:"seconds_retained"`
	Decisions   int    `json:"decisions"`
	Dropped     uint64 `json:"frames_evicted"`
	Meta        Meta   `json:"meta"`
}

func (r *Recorder) SessionInfo() SessionInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	info := SessionInfo{ID: r.meta.ID, Frames: len(r.frames), FrameBytes: r.frameBytes,
		Decisions: len(r.decisions), Dropped: r.droppedFrames, Meta: r.meta}
	if len(r.frames) > 0 {
		info.TFirst = r.frames[0].T
		info.TLast = r.frames[len(r.frames)-1].T
		info.WallFirstMs = r.frames[0].WallMs
		info.WallLastMs = r.frames[len(r.frames)-1].WallMs
		info.Seconds = int((info.TLast - info.TFirst) / 1000)
	}
	return info
}

// Wall0/T0 pemetaan wall↔sim untuk marker incident (incident unix ms → sim t).
func (r *Recorder) Window() (wall0, t0 int64, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.frames) == 0 {
		return 0, 0, false
	}
	return r.frames[0].WallMs, r.frames[0].T, true
}

// dumpHeader/footer membungkus frames array dalam satu dokumen JSON.
type dumpHeader struct {
	Meta  Meta  `json:"meta"`
	Wall0 int64 `json:"wall0"`
	T0    int64 `json:"t0"`
	Count int   `json:"count"`
}

// WriteDump menulis dokumen GET /api/replay/sessions/{id}:
// {"meta":…,"wall0":…,"t0":…,"count":N,"frames":[…],"decisions":[…]}.
// Bila gzipOK, seluruh dokumen ditulis sebagai rangkaian anggota gzip
// (header, frame0, ",", frame1, …, footer) — tanpa re-marshal per frame.
// Bila !gzipOK, blob didekompresi on-the-fly (fallback identitas).
func (r *Recorder) WriteDump(w io.Writer, gzipOK bool) error {
	r.mu.Lock()
	frames := r.frames
	decisions := r.decisions
	meta := r.meta
	var wall0, t0 int64
	if len(frames) > 0 {
		wall0, t0 = frames[0].WallMs, frames[0].T
	}
	r.mu.Unlock()

	head, err := json.Marshal(dumpHeader{Meta: meta, Wall0: wall0, T0: t0, Count: len(frames)})
	if err != nil {
		return err
	}
	decJSON, err := json.Marshal(decisions)
	if err != nil {
		return err
	}

	if !gzipOK {
		if _, err := w.Write(bytes.TrimSuffix(head, []byte("}"))); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `,"frames":[`); err != nil {
			return err
		}
		for i, f := range frames {
			if i > 0 {
				if _, err := io.WriteString(w, ","); err != nil {
					return err
				}
			}
			if err := writeGunzip(w, f.GZ); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, `],"decisions":`); err != nil {
			return err
		}
		if _, err := w.Write(decJSON); err != nil {
			return err
		}
		_, err = io.WriteString(w, "}")
		return err
	}

	// Satu anggota gzip baru per potongan meta — gzip.Writer tidak bisa
	// dipakai ulang setelah Close; w tetap sama sehingga anggota menyambung.
	writeMember := func(p []byte) error {
		mw, err := gzip.NewWriterLevel(w, gzipLevel)
		if err != nil {
			return err
		}
		if _, err := mw.Write(p); err != nil {
			return err
		}
		return mw.Close()
	}
	// header: '{"meta":…,"frames":['
	h := append(bytes.TrimSuffix(append([]byte(nil), head...), []byte("}")), []byte(`,"frames":[`)...)
	if err := writeMember(h); err != nil {
		return err
	}
	comma := gzBytes([]byte(","))
	for i, f := range frames {
		// Tulis mentah ke w — blob sudah anggota gzip mandiri (bukan lewat zw,
		// yang akan mengompresi ulang di dalam anggota berjalan).
		if _, err := w.Write(f.GZ); err != nil {
			return err
		}
		if i < len(frames)-1 {
			if _, err := w.Write(comma); err != nil {
				return err
			}
		}
	}
	// footer: '],"decisions":[…]}'
	foot := append([]byte(`],"decisions":`), decJSON...)
	foot = append(foot, '}')
	return writeMember(foot)
}

func writeGunzip(w io.Writer, blob []byte) error {
	zr, err := gzip.NewReader(bytes.NewReader(blob))
	if err != nil {
		return err
	}
	defer zr.Close()
	_, err = io.Copy(w, zr)
	return err
}
