// SPDX-FileCopyrightText: 2020 Alvar Penning
//
// SPDX-License-Identifier: GPL-3.0-or-later

package soclp

import (
	"fmt"
	"io"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/dtn7/dtn7-go/bundle"
	"github.com/dtn7/dtn7-go/cla"
)

// Session between two peers (this node and another) for the Socket Convergence Layer Protocol (SoCLP).
//
// A session requires some kind of stream protocol to operate on, e.g., TCP, QUIC, or Unix Domain Sockets. However, a
// Session itself is "protocol agnostic".
//
// To create a SoCLP connection, a Session must be wrapped into its underlying protocol. Therefore a new Session
// instance needs to created. The exported fields (upper case fields) needs to be configured accordingly. Other fields
// will be instantiated correctly within the Start method.
type Session struct {
	// In and Out are the streams to operate on.
	In  io.Reader
	Out io.Writer

	// Closer closes the underlying stream, might be nil.
	Closer io.Closer

	// Restartable is true iff this Session might be restarted, e.g., after connectivity issues.
	Restartable bool

	// wasStartedOnce is a flag to be only raised, must be done in the Start method. Is used together with Restartable.
	wasStartedOnce bool

	// StartFunc represents additional startup code, e.g., to establish a TCP connection. The parameter will be this
	// very session. Might be nil.
	StartFunc func(*Session) (error, bool)

	// AddressFunc generates this Session's Address. The parameter will be this very Session.
	AddressFunc func(*Session) string

	// Permanent is true iff this Session should be permanent resp. not be removed on connection issues.
	Permanent bool

	// Endpoint is this node's Endpoint ID; this node, not the peer.
	Endpoint bundle.EndpointID

	// peerEndpoint is the Endpoint ID of the peer and has its mutex for read/write access.
	peerEndpoint     bundle.EndpointID
	peerEndpointLock sync.RWMutex

	// statusChannel is outgoing, see Channel().
	statusChannel chan cla.ConvergenceStatus

	// outChannel and outStopChannel are to communicate with the outgoing handler.
	outChannel     chan Message
	outStopChannel chan struct{}

	// heartbeatStopChannel is to communicate with the heartbeat handler.
	heartbeatStopChannel chan struct{}

	// transferAcks stores received ack identifiers, sync.Map[uint64]struct{}
	transferAcks sync.Map

	// HeartbeatTimeout defines the maximum idle duration. Heartbeat StatusMessage will be sent for prevention.
	HeartbeatTimeout time.Duration

	// lastReceive and lastSent are holding the time of the last incoming resp. outgoing Messages.
	lastReceive     time.Time
	lastReceiveLock sync.RWMutex
	lastSent        time.Time
	lastSentLock    sync.RWMutex

	// closeOnce ensures that the code of closeAction is only executed once.
	closeOnce sync.Once

	// isActive indicates if this very Session is active resp. not in a closed state.
	isActive     bool
	isActiveLock sync.RWMutex
}

// closeAction performs the closing within a sync.Once.
//
// This method is called from the exported Close method, which starts with sending a Shutdown StatusMessage.
func (s *Session) closeAction() {
	s.closeOnce.Do(func() {
		s.logger().Info("Closing down")

		s.isActiveLock.Lock()
		s.isActive = false
		s.isActiveLock.Unlock()

		s.Channel() <- cla.NewConvergencePeerDisappeared(s, s.GetPeerEndpointID())

		close(s.heartbeatStopChannel)
		close(s.outStopChannel)
		// close(s.statusChannel)

		if s.Closer != nil {
			if err := s.Closer.Close(); err != nil {
				s.logger().WithError(err).Warn("Closing down errored")
			}
		}
	})
}

// Close down this session and try telling the peer to do the same.
func (s *Session) Close() {
	s.isActiveLock.RLock()
	if s.isActive && s.outChannel != nil {
		s.outChannel <- Message{NewShutdownStatusMessage()}
	}
	s.isActiveLock.RUnlock()

	s.closeAction()
}

// Start this Session. In case of an error, retry indicates that another try should be made later.
func (s *Session) Start() (err error, retry bool) {
	if !s.Restartable && s.wasStartedOnce {
		err = fmt.Errorf("session was already started once and is marked as not restartable")
		retry = false
		return
	}

	s.wasStartedOnce = true
	s.peerEndpoint = bundle.EndpointID{}
	s.peerEndpointLock = sync.RWMutex{}
	s.statusChannel = make(chan cla.ConvergenceStatus)
	s.outChannel = make(chan Message)
	s.outStopChannel = make(chan struct{})
	s.heartbeatStopChannel = make(chan struct{})
	s.transferAcks = sync.Map{}
	s.lastReceive = time.Now()
	s.lastSentLock = sync.RWMutex{}
	s.lastSent = time.Now()
	s.lastSentLock = sync.RWMutex{}
	s.closeOnce = sync.Once{}
	s.isActive = true
	s.isActiveLock = sync.RWMutex{}

	s.logger().Info("Starting new SoCLP session")

	if s.StartFunc != nil {
		if err, retry = s.StartFunc(s); err != nil {
			return
		}
	}

	go s.handleIn()
	go s.handleOut()
	go s.handleHeartbeat()

	s.outChannel <- Message{NewIdentityMessage(s.Endpoint)}

	return
}

// Channel for status information and received Bundles.
func (s *Session) Channel() chan cla.ConvergenceStatus {
	return s.statusChannel
}

// Address for this Session's instance, should be kind of unique.
func (s *Session) Address() string {
	return s.AddressFunc(s)
}

// IsPermanent returns true, if this CLA should not be removed after failures.
func (s *Session) IsPermanent() bool {
	return s.Permanent
}

// Send a Bundle to the peer and wait for a reception acknowledgement.
func (s *Session) Send(b *bundle.Bundle) error {
	if tm, tmErr := NewTransferMessage(*b); tmErr != nil {
		return tmErr
	} else {
		s.outChannel <- Message{MessageType: tm}

		for {
			s.isActiveLock.RLock()
			active := s.isActive
			s.isActiveLock.RUnlock()

			if !active {
				return fmt.Errorf("connection timed out before an acknowledgement was received")
			} else if _, ack := s.transferAcks.Load(tm.Identifier); ack {
				s.transferAcks.Delete(tm.Identifier)
				return nil
			} else {
				time.Sleep(50 * time.Millisecond)
			}
		}
	}
}

// GetEndpointID returns this instance's endpoint identifier.
func (s *Session) GetEndpointID() bundle.EndpointID {
	return s.Endpoint
}

// GetPeerEndpointID returns the peer's endpoint identifier, if known. Otherwise, dtn:none will be returned.
func (s *Session) GetPeerEndpointID() bundle.EndpointID {
	s.peerEndpointLock.RLock()
	defer s.peerEndpointLock.RUnlock()

	if s.peerEndpoint == (bundle.EndpointID{}) {
		return bundle.DtnNone()
	} else {
		return s.peerEndpoint
	}
}

// logger returns a new logrus.Entry.
func (s *Session) logger() (e *log.Entry) {
	e = log.WithField("soclp-session", s)

	if peer := s.GetPeerEndpointID(); peer != bundle.DtnNone() {
		e = e.WithField("peer", peer)
	}

	return
}

func (s *Session) String() string {
	return s.Address()
}
