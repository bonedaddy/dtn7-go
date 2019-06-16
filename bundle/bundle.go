package bundle

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/dtn7/cboring"
	"github.com/hashicorp/go-multierror"
	"github.com/ugorji/go/codec"
)

// Bundle represents a bundle as defined in section 4.2.1. Each Bundle contains
// one primary block and multiple canonical blocks.
type Bundle struct {
	PrimaryBlock    PrimaryBlock
	CanonicalBlocks []CanonicalBlock
}

// NewBundle creates a new Bundle. The values and flags of the blocks will be
// checked and an error might be returned.
func NewBundle(primary PrimaryBlock, canonicals []CanonicalBlock) (b Bundle, err error) {
	b = MustNewBundle(primary, canonicals)
	err = b.checkValid()

	return
}

// MustNewBundle creates a new Bundle like NewBundle, but skips the validity
// check. No panic will be called!
func MustNewBundle(primary PrimaryBlock, canonicals []CanonicalBlock) Bundle {
	return Bundle{
		PrimaryBlock:    primary,
		CanonicalBlocks: canonicals,
	}
}

// forEachBlock applies the given function for each of this Bundle's blocks.
func (b *Bundle) forEachBlock(f func(block)) {
	f(&b.PrimaryBlock)
	for i := 0; i < len(b.CanonicalBlocks); i++ {
		f(&b.CanonicalBlocks[i])
	}
}

// ExtensionBlock returns this Bundle's canonical block/extension block
// matching the requested block type code. If no such block was found,
// an error will be returned.
func (b *Bundle) ExtensionBlock(blockType CanonicalBlockType) (*CanonicalBlock, error) {
	for i := 0; i < len(b.CanonicalBlocks); i++ {
		cb := &b.CanonicalBlocks[i]
		if (*cb).BlockType == blockType {
			return cb, nil
		}
	}

	return nil, newBundleError(fmt.Sprintf(
		"No CanonicalBlock with block type %d was found in Bundle", blockType))
}

// PayloadBlock returns this Bundle's payload block or an error, if it does
// not exists.
func (b *Bundle) PayloadBlock() (*CanonicalBlock, error) {
	return b.ExtensionBlock(PayloadBlock)
}

// AddExtensionBlock adds a new ExtensionBlock to this Bundle. The block number
// will be calculated and overwritten within this method.
func (b *Bundle) AddExtensionBlock(block CanonicalBlock) {
	var blockNumbers []uint64
	for i := 0; i < len(b.CanonicalBlocks); i++ {
		blockNumbers = append(blockNumbers, b.CanonicalBlocks[i].BlockNumber)
	}

	var blockNumber uint64 = 1
	for {
		flag := true
		for _, no := range blockNumbers {
			if blockNumber == no {
				flag = false
				break
			}
		}

		if flag {
			break
		} else {
			blockNumber += 1
		}
	}

	block.BlockNumber = blockNumber
	b.CanonicalBlocks = append(b.CanonicalBlocks, block)
}

// SetCRCType sets the given CRCType for each block. To also calculate and set
// the CRC value, one should also call the CalculateCRC method.
func (b *Bundle) SetCRCType(crcType CRCType) {
	b.forEachBlock(func(blck block) {
		blck.SetCRCType(crcType)
	})
}

// CalculateCRC calculates and sets the CRC value for each block.
func (b *Bundle) CalculateCRC() {
	b.forEachBlock(func(blck block) {
		blck.CalculateCRC()
	})
}

// ID returns a kind of uniquene representation of this bundle, containing
// the souce node and creation timestamp. If this bundle is a fragment, the
// offset is also present.
func (b Bundle) ID() string {
	var bldr strings.Builder

	fmt.Fprintf(&bldr, "%v-%d-%d",
		b.PrimaryBlock.SourceNode,
		b.PrimaryBlock.CreationTimestamp[0],
		b.PrimaryBlock.CreationTimestamp[1])

	if pb := b.PrimaryBlock; pb.BundleControlFlags.Has(IsFragment) {
		fmt.Fprintf(&bldr, "-%d", pb.FragmentOffset)
	}

	return bldr.String()
}

func (b Bundle) String() string {
	return b.ID()
}

// CheckCRC checks the CRC value of each block and returns false if some
// value does not match. This method changes the block's CRC value temporary
// and is not thread safe.
func (b *Bundle) CheckCRC() bool {
	var flag = true

	b.forEachBlock(func(blck block) {
		if !blck.CheckCRC() {
			flag = false
		}
	})

	return flag
}

func (b Bundle) checkValid() (errs error) {
	// Check blocks for errors
	b.forEachBlock(func(blck block) {
		if blckErr := blck.checkValid(); blckErr != nil {
			errs = multierror.Append(errs, blckErr)
		}
	})

	// Check CanonicalBlocks for errors
	if b.PrimaryBlock.BundleControlFlags.Has(AdministrativeRecordPayload) ||
		b.PrimaryBlock.SourceNode == DtnNone() {
		for _, cb := range b.CanonicalBlocks {
			if cb.BlockControlFlags.Has(StatusReportBlock) {
				errs = multierror.Append(errs,
					newBundleError("Bundle: Bundle Processing Control Flags indicate that "+
						"this bundle's payload is an administrative record or the source "+
						"node is omitted, but the \"Transmit status report if block canot "+
						"be processed\" Block Processing Control Flag was set in a "+
						"Canonical Block"))
			}
		}
	}

	// Check uniqueness of block numbers
	var cbBlockNumbers = make(map[uint64]bool)
	// Check max 1 occurrence of extension blocks
	var cbBlockTypes = make(map[CanonicalBlockType]bool)

	for _, cb := range b.CanonicalBlocks {
		if _, ok := cbBlockNumbers[cb.BlockNumber]; ok {
			errs = multierror.Append(errs,
				newBundleError(fmt.Sprintf(
					"Bundle: Block number %d occurred multiple times", cb.BlockNumber)))
		}
		cbBlockNumbers[cb.BlockNumber] = true

		switch cb.BlockType {
		case PreviousNodeBlock, BundleAgeBlock, HopCountBlock:
			if _, ok := cbBlockTypes[cb.BlockType]; ok {
				errs = multierror.Append(errs,
					newBundleError(fmt.Sprintf(
						"Bundle: Block type %d occurred multiple times", cb.BlockType)))
			}
			cbBlockTypes[cb.BlockType] = true
		}
	}

	if b.PrimaryBlock.CreationTimestamp[0] == 0 {
		if _, ok := cbBlockTypes[BundleAgeBlock]; !ok {
			errs = multierror.Append(errs, newBundleError(
				"Bundle: Creation Timestamp is zero, but no Bundle Age block is present"))
		}
	}

	return
}

// IsAdministrativeRecord returns if this Bundle's control flags indicate this
// has an administrative record payload.
func (b Bundle) IsAdministrativeRecord() bool {
	return b.PrimaryBlock.BundleControlFlags.Has(AdministrativeRecordPayload)
}

// WriteCbor serializes this Bundle as a CBOR indefinite-length array into the
// given Writer.
func (b Bundle) writeCbor(w io.Writer) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = newBundleError(fmt.Sprintf("Bundle: Encoding CBOR failed, %v", r))
		}
	}()

	// It seems to be tricky using both definite-length and indefinite-length
	// arays with the codec library. However, an indefinite-length array is just
	// a byte array wrapped between the start and "break" code, which are
	// exported as consts from the codec library.

	var bw = bufio.NewWriter(w)
	defer bw.Flush()

	var cborEncoder = codec.NewEncoder(bw, new(codec.CborHandle))

	bw.WriteByte(codec.CborStreamArray)

	b.forEachBlock(func(blck block) {
		cborEncoder.MustEncode(blck)
	})

	bw.WriteByte(codec.CborStreamBreak)

	return
}

// ToCbor creates a byte array representing a CBOR indefinite-length array of
// this Bundle with all its blocks, as defined in section 4 of the Bundle
// Protocol Version 7.
func (b Bundle) ToCbor() []byte {
	var blocks []byte
	var cborEncoder = codec.NewEncoderBytes(&blocks, new(codec.CborHandle))

	b.forEachBlock(func(blck block) {
		cborEncoder.MustEncode(blck)
	})

	var buf bytes.Buffer

	buf.WriteByte(codec.CborStreamArray)
	buf.Write(blocks)
	buf.WriteByte(codec.CborStreamBreak)

	return buf.Bytes()
}

// decodeBundleBlock decodes an already generic decoded block to its
// determinated data structure.
// The NewBundleFromCbor function decodes an array of interface{} which results
// in an array of arrays, as codec tries to decode the whole data. This method
// will re-encode this "anonymous" array to CBOR and will decode it to its
// struct, which is referenced as the target pointer.
func decodeBundleBlock(data *interface{}, target interface{}) {
	var r, w = io.Pipe()

	go func() {
		bw := bufio.NewWriter(w)
		codec.NewEncoder(bw, new(codec.CborHandle)).MustEncode(data)
		bw.Flush()
	}()

	codec.NewDecoder(bufio.NewReader(r), new(codec.CborHandle)).MustDecode(target)
}

// NewBundleFromCbor decodes the given data from the CBOR into a Bundle.
func NewBundleFromCbor(data *[]byte) (b Bundle, err error) {
	// The decoding might panic and would be recovered in the following function,
	// which returns an error.
	defer func() {
		if r := recover(); r != nil {
			err = newBundleError(fmt.Sprintf("Bundle: Decoding CBOR failed, %v", r))
		}
	}()

	var reader = bytes.NewReader(*data)
	var handle = new(codec.CborHandle)

	// Skip array starting symbol
	reader.ReadByte()

	var pb PrimaryBlock
	if err = codec.NewDecoder(reader, handle).Decode(&pb); err != nil {
		return
	}

	var cbs []CanonicalBlock
	for fin := false; !fin; {
		switch cbType, _ := reader.ReadByte(); cbType {
		case 0x85:
			reader.UnreadByte()

			var cb5 canonicalBlock5
			codec.NewDecoder(reader, handle).Decode(&cb5)
			cbs = append(cbs, *cb5.toCanonicalBlock())

		case 0x86:
			reader.UnreadByte()

			var cb6 canonicalBlock6
			codec.NewDecoder(reader, handle).Decode(&cb6)
			cbs = append(cbs, *cb6.toCanonicalBlock())

		case 0xFF:
			fin = true

		default:
			err = fmt.Errorf("Unexpected cbType %x while decoding canonicals", cbType)
			return
		}
	}

	b = Bundle{pb, cbs}

	if chkVldErr := b.checkValid(); chkVldErr != nil {
		err = multierror.Append(err, chkVldErr)
	}

	if !b.CheckCRC() {
		err = multierror.Append(err, newBundleError("CRC failed"))
	}

	return
}

func (b *Bundle) MarshalCbor(w io.Writer) error {
	if _, err := w.Write([]byte{cboring.IndefiniteArray}); err != nil {
		return err
	}

	if err := cboring.Marshal(&b.PrimaryBlock, w); err != nil {
		return fmt.Errorf("PrimaryBlock failed: %v", err)
	}

	for i := 0; i < len(b.CanonicalBlocks); i++ {
		if err := cboring.Marshal(&b.CanonicalBlocks[i], w); err != nil {
			return fmt.Errorf("CanonicalBlock failed: %v", err)
		}
	}

	if _, err := w.Write([]byte{cboring.BreakCode}); err != nil {
		return err
	}

	return nil
}

func (b *Bundle) UnmarshalCbor(r io.Reader) error {
	if err := cboring.ReadExpect(cboring.IndefiniteArray, r); err != nil {
		return err
	}

	if err := cboring.Unmarshal(&b.PrimaryBlock, r); err != nil {
		return fmt.Errorf("PrimaryBlock failed: %v", err)
	}

	for {
		cb := CanonicalBlock{}
		if err := cboring.Unmarshal(&cb, r); err == cboring.FlagBreakCode {
			break
		} else if err != nil {
			return fmt.Errorf("CanonicalBlock failed: %v", err)
		} else {
			b.CanonicalBlocks = append(b.CanonicalBlocks, cb)
		}
	}

	return nil
}
