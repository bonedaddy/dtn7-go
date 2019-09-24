package tcpcl

import (
	"bufio"
	"fmt"
	"io"

	"github.com/dtn7/dtn7-go/bundle"
)

// OutgoingTransfer represents a Bundle OutgoingTransfer for the TCPCL.
type OutgoingTransfer struct {
	Id uint64

	startFlag  bool
	dataStream io.Reader
}

// NewOutgoingTransfer creates a new OutgoingTransfer for data written into the returned Writer.
func NewOutgoingTransfer(id uint64) (t *OutgoingTransfer, w io.Writer) {
	r, w := io.Pipe()
	t = &OutgoingTransfer{
		Id:         id,
		startFlag:  true,
		dataStream: r,
	}

	return
}

func (t OutgoingTransfer) String() string {
	return fmt.Sprintf("OUTGOING_TRANSFER(%d)", t.Id)
}

// NewBundleOutgoingTransfer creates a new OutgoingTransfer for a Bundle.
func NewBundleOutgoingTransfer(id uint64, b bundle.Bundle) *OutgoingTransfer {
	var t, w = NewOutgoingTransfer(id)

	go func(w *io.PipeWriter) {
		bw := bufio.NewWriter(w)

		_ = b.MarshalCbor(bw)
		_ = bw.Flush()
		_ = w.Close()
	}(w.(*io.PipeWriter))

	return t
}

// NextSegment creates the next XFER_SEGMENT for the given MRU or an EOF in case
// of a finished Writer.
func (t *OutgoingTransfer) NextSegment(mru uint64) (dtm DataTransmissionMessage, err error) {
	var segFlags SegmentFlags

	if t.startFlag {
		t.startFlag = false
		segFlags |= SegmentStart
	}

	var buf = make([]byte, mru)
	if n, rErr := t.dataStream.Read(buf); rErr != nil {
		err = rErr
		return
	} else if uint64(n) < mru {
		buf = buf[:n]
		segFlags |= SegmentEnd
	}

	dtm = NewDataTransmissionMessage(segFlags, t.Id, buf)
	return
}
