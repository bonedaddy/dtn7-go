package tcpcl

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// TransferRefusalCode is the one-octet refusal reason code for a XFER_REFUSE message.
type TransferRefusalCode uint8

const (
	// RefusalUnknown indicates an unknown or not specified reason.
	RefusalUnknown TransferRefusalCode = 0x00

	// RefusalExtensionFailure indicates a failure processing the Transfer Extension Items.
	RefusalExtensionFailure TransferRefusalCode = 0x01

	// RefusalCompleted indicates that the receiver already has the complete bundle.
	RefusalCompleted TransferRefusalCode = 0x02

	// RefusalNoResources indicate that the receiver's resources are exhausted.
	RefusalNoResources TransferRefusalCode = 0x03

	// RefusalRetransmit indicates a problem on the receiver's side. This requires
	// the complete bundle to be retransmitted.
	RefusalRetransmit TransferRefusalCode = 0x04
)

// IsValid checks if this TransferRefusalCode represents a valid value.
func (trc TransferRefusalCode) IsValid() bool {
	switch trc {
	case RefusalUnknown, RefusalExtensionFailure, RefusalCompleted,
		RefusalNoResources, RefusalRetransmit:
		return true
	default:
		return false
	}
}

func (trc TransferRefusalCode) String() string {
	switch trc {
	case RefusalUnknown:
		return "Unknown"
	case RefusalExtensionFailure:
		return "Extension Failure"
	case RefusalCompleted:
		return "Completed"
	case RefusalNoResources:
		return "No Resources"
	case RefusalRetransmit:
		return "Retransmit"
	default:
		return "INVALID"
	}
}

// XFER_REFUSE is the Message Header code for a Transfer Refusal Message.
const XFER_REFUSE uint8 = 0x03

// TransferRefusalMessage is the XFER_REFUSE message for transfer refusals.
type TransferRefusalMessage struct {
	ReasonCode TransferRefusalCode
	TransferId uint64
}

// NewTransferRefusalMessage creates a new TransferRefusalMessage with given fields.
func NewTransferRefusalMessage(reason TransferRefusalCode, tid uint64) TransferRefusalMessage {
	return TransferRefusalMessage{
		ReasonCode: reason,
		TransferId: tid,
	}
}

func (trm TransferRefusalMessage) String() string {
	return fmt.Sprintf(
		"XFER_REFUSE(Reason Code=%v, Transfer ID=%d)",
		trm.ReasonCode, trm.TransferId)
}

// MarshalBinary encodes this TransferRefusalMessage into its binary form.
func (trm TransferRefusalMessage) MarshalBinary() (data []byte, err error) {
	var buf = new(bytes.Buffer)
	var fields = []interface{}{XFER_REFUSE, trm.ReasonCode, trm.TransferId}

	for _, field := range fields {
		if binErr := binary.Write(buf, binary.BigEndian, field); binErr != nil {
			err = binErr
			return
		}
	}

	data = buf.Bytes()
	return
}

// UnmarshalBinary decodes a TransferRefusalMessage from its binary form.
func (trm *TransferRefusalMessage) UnmarshalBinary(data []byte) error {
	var buf = bytes.NewReader(data)

	var messageHeader uint8
	if err := binary.Read(buf, binary.BigEndian, &messageHeader); err != nil {
		return err
	} else if messageHeader != XFER_REFUSE {
		return fmt.Errorf("XFER_REFUSE's Message Header is wrong: %d instead of %d", messageHeader, XFER_REFUSE)
	}

	var fields = []interface{}{&trm.ReasonCode, &trm.TransferId}

	for _, field := range fields {
		if err := binary.Read(buf, binary.BigEndian, field); err != nil {
			return err
		}
	}

	if !trm.ReasonCode.IsValid() {
		return fmt.Errorf("XFER_REFUSE's Reason Code %x is invalid", trm.ReasonCode)
	}

	return nil
}
