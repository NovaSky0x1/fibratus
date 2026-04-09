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
	"github.com/rabbitstack/fibratus/pkg/event"
	"github.com/rabbitstack/fibratus/pkg/util/bytes"
	log "github.com/sirupsen/logrus"
)

const kcapMagic = 0x6669627261747573

// Section type constants (matching pkg/cap/section)
const (
	sectionHandle uint8 = 2
	sectionEvent  uint8 = 3
)

// Section version constants (matching pkg/cap/version)
const (
	handleV1 uint8 = 1
	eventV2  uint8 = 2
)

// writeSection writes a 10-byte section header.
func writeSection(w *zstd.Encoder, typ, ver uint8, length, size uint32) error {
	var sec [10]byte
	sec[0] = typ
	sec[1] = ver
	copy(sec[2:6], bytes.WriteUint32(length))
	copy(sec[6:], bytes.WriteUint32(size))
	_, err := w.Write(sec[:])
	return err
}

// kcapWriter writes events to a .kcap file using pure-Go zstd compression.
type kcapWriter struct {
	mu      sync.Mutex
	f       *os.File
	zw      *zstd.Encoder
	count   uint64
	nbytes  uint64
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

	// Write empty handle section (no handles — replay still works, just without handle context)
	if err := writeSection(zw, sectionHandle, handleV1, 0, 0); err != nil {
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

	if err := writeSection(w.zw, sectionEvent, eventV2, 0, uint32(len(raw))); err != nil {
		return
	}
	if _, err := w.zw.Write(raw); err != nil {
		return
	}

	w.count++
	w.nbytes += uint64(len(raw))
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

	log.Infof("kcap: wrote %d events (%d bytes) in %s", w.count, w.nbytes, time.Since(w.started).Round(time.Millisecond))
}
