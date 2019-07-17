package core

import (
	"fmt"
	"github.com/dtn7/cboring"
	"github.com/dtn7/dtn7-go/bundle"
	"io"
	"time"

	log "github.com/sirupsen/logrus"
)

// timestampNow outputs the current UNIX-time as an unsigned int64
// (I have no idea, why this is signed by default... does the kernel even allow you to set a negative time?)
func timestampNow() uint64 {
	return uint64(time.Now().Unix())
}

// DTLSR is an implementation of "Delay Tolerant Link State Routing"
type DTLSR struct {
	c *Core
	// routingTable is a [endpoint]forwardingNode mapping
	routingTable map[bundle.EndpointID]bundle.EndpointID
	// peerChange denotes whether there has been a change in our direct connections
	// since we last calculated our routing table/broadcast our peer data
	peerChange bool
	// peers is our own peerData
	peers peerData
	// receivedChange denotes whether we received new data since we last computed our routing table
	receivedChange bool
	// receivedData is peerData received from other nodes
	receivedData map[bundle.EndpointID]peerData
}

// peerData contains a peer's connection data
type peerData struct {
	// id is the node's endpoint id
	id bundle.EndpointID
	// timestamp is the time the last change occured
	// when receiving other node's data, we only update if the timestamp in newer
	timestamp uint64
	// peers is a mapping of previously seen peers and the respective timestamp of the last encounter
	peers map[bundle.EndpointID]uint64
}

func (pd *peerData) isNewerThan(other peerData) bool {
	return pd.timestamp > other.timestamp
}

func NewDTLSR(c *Core) DTLSR {
	log.Debug("Initialised DTLSR")
	return DTLSR{
		c:            c,
		routingTable: make(map[bundle.EndpointID]bundle.EndpointID),
		peerChange:   false,
		peers: peerData{
			id:        c.NodeId,
			timestamp: timestampNow(),
			peers:     make(map[bundle.EndpointID]uint64),
		},
		receivedChange: false,
		receivedData:   make(map[bundle.EndpointID]peerData),
	}
}

const ExtBlockTypeDTLSRBlock uint64 = 193

// DTLSRBlock contains routing metadata
type DTLSRBlock peerData

func NewDTLSRBlock(data peerData) *DTLSRBlock {
	newBlock := DTLSRBlock(data)
	return &newBlock
}

func (dtlsrb *DTLSRBlock) getPeerData() peerData {
	return peerData(*dtlsrb)
}

func (dtlsrb *DTLSRBlock) BlockTypeCode() uint64 {
	return ExtBlockTypeDTLSRBlock
}

func (dtlsrb *DTLSRBlock) CheckValid() error {
	return nil
}

func (dtlsrb *DTLSRBlock) MarshalCbor(w io.Writer) error {
	// start with the (apparently) required outer array
	if err := cboring.WriteArrayLength(3, w); err != nil {
		return err
	}

	// write our won endpoint id
	if err := cboring.Marshal(&dtlsrb.id, w); err != nil {
		return err
	}

	// write the timestamp
	if err := cboring.WriteUInt(dtlsrb.timestamp, w); err != nil {
		return err
	}

	// write the peer data keys
	if err := cboring.WriteArrayLength(uint64(len(dtlsrb.peers)), w); err != nil {
		return err
	}
	for peerID := range dtlsrb.peers {
		if err := cboring.Marshal(&peerID, w); err != nil {
			return err
		}
	}

	// write the peer data
	if err := cboring.WriteArrayLength(uint64(len(dtlsrb.peers)), w); err != nil {
		return err
	}
	for _, timestamp := range dtlsrb.peers {
		if err := cboring.WriteUInt(timestamp, w); err != nil {
			return err
		}
	}

	return nil
}

func (dtlsrb *DTLSRBlock) UnmarshalCbor(r io.Reader) error {
	// read the (apparently) required outer array
	if l, err := cboring.ReadArrayLength(r); err != nil {
		return err
	} else if l != 3 {
		return fmt.Errorf("expected 3 fields, got %d", l)
	}

	// read endpoint id
	id := bundle.EndpointID{}
	if err := cboring.Unmarshal(&id, r); err != nil {
		return err
	} else {
		dtlsrb.id = id
	}

	// read the timestamp
	if timestamp, err := cboring.ReadUInt(r); err != nil {
		return err
	} else {
		dtlsrb.timestamp = timestamp
	}

	var lenKeys uint64
	var lenData uint64

	// read length of key array
	lenKeys, err := cboring.ReadArrayLength(r)
	if err != nil {
		return err
	}

	// read key array
	var i uint64
	keys := make([]bundle.EndpointID, lenKeys)
	for i = 0; i < lenKeys; i++ {
		key := bundle.EndpointID{}
		if err := cboring.Unmarshal(&key, r); err != nil {
			return err
		}
		keys[i] = key
	}

	// read length of data array
	lenData, err = cboring.ReadArrayLength(r)
	if err != nil {
		return err
	}

	// both arrays must be of the same size
	if lenKeys != lenData {
		return fmt.Errorf("Key-Value array size mismatch: Keys: %d, Values: %d", lenKeys, lenData)
	}

	data := make([]uint64, lenData)
	for i = 0; i < lenData; i++ {
		value, err := cboring.ReadUInt(r)
		if err != nil {
			return err
		}
		data[i] = value
	}

	// recreate map from two arrays
	peers := make(map[bundle.EndpointID]uint64)
	for i = 0; i < lenKeys; i++ {
		peers[keys[i]] = data[i]
	}

	dtlsrb.peers = peers

	return nil
}