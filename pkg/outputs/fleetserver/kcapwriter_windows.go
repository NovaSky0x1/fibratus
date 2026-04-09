/*
 * Standalone .kcap writer for fleet captures. Uses pure-Go zstd
 * (klauspost/compress) so it works without CGO or the 'cap' build tag.
 * Writes a minimal but valid .kcap file that fibratus replay can read.
 */

package fleetserver

import (
	"encoding/binary"
	"math"
	"os"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/rabbitstack/fibratus/pkg/cap/section"
	capver "github.com/rabbitstack/fibratus/pkg/cap/version"
	"github.com/rabbitstack/fibratus/pkg/event"
	log "github.com/sirupsen/logrus"
)

const kcapMagic = 0x6669627261747573

// kcapWriter writes events to a .kcap file using pure-Go zstd compression.
type kcapWriter struct {
	mu      sync.Mutex
	f       *os.File
	zw      *zstd.Encoder
	count   uint64
	bytes   uint64
	started time.Time
}

// newKcapWriter creates a new .kcap file and writes the header.
func newKcapWriter(path string) (*kcapWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	zw, err := zstd.NewWriter(f)
	if err != nil {
		f.Close()
		return nil, err
	}

	w := &kcapWriter{f: f, zw: zw, started: time.Now()}

	// Write header: magic (8) + major (1) + minor (1) + flags (8) = 18 bytes
	if err := binary.Write(zw, binary.LittleEndian, uint64(kcapMagic)); err != nil {
		w.close()
		return nil, err
	}
	if _, err := zw.Write([]byte{2, 0}); err != nil { // major=2, minor=0
		w.close()
		return nil, err
	}
	if err := binary.Write(zw, binary.LittleEndian, uint64(0)); err != nil { // flags
		w.close()
		return nil, err
	}

	// Write empty handle section (no handles — replay will still work)
	sec := section.New(section.Handle, capver.HandleV1, 0, 0)
	if _, err := zw.Write(sec[:]); err != nil {
		w.close()
		return nil, err
	}

	return w, nil
}

// writeEvent serializes and writes a single event to the .kcap file.
func (w *kcapWriter) writeEvent(evt *event.Event) {
	raw := evt.MarshalRaw()
	if len(raw) == 0 || uint64(len(raw)) > math.MaxUint32 {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	sec := section.New(section.Event, capver.EventV2, 0, uint32(len(raw)))
	if _, err := w.zw.Write(sec[:]); err != nil {
		return
	}
	if _, err := w.zw.Write(raw); err != nil {
		return
	}

	w.count++
	w.bytes += uint64(len(raw))
}

// close flushes and closes the .kcap file.
func (w *kcapWriter) close() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.zw != nil {
		w.zw.Close()
	}
	if w.f != nil {
		w.f.Close()
	}

	log.Infof("kcap: wrote %d events (%d bytes) in %s", w.count, w.bytes, time.Since(w.started).Round(time.Millisecond))
}
