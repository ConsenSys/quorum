// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	qbfttypes "github.com/ethereum/go-ethereum/consensus/istanbul/qbft/types"
	"github.com/ethereum/go-ethereum/rlp"
)

// Start implements core.Engine.Start
func (c *core) Start() error {
	// Tests will handle events itself, so we have to make subscribeEvents()
	// be able to call in test.
	c.subscribeEvents()
	go c.handleEvents()

	// Start a new round from last sequence + 1
	c.startNewRound(common.Big0)

	return nil
}

// Stop implements core.Engine.Stop
func (c *core) Stop() error {
	c.stopTimer()
	c.unsubscribeEvents()

	// Make sure the handler goroutine exits
	c.handlerWg.Wait()
	return nil
}

// ----------------------------------------------------------------------------

// Subscribe both internal and external events
func (c *core) subscribeEvents() {
	c.events = c.backend.EventMux().Subscribe(
		// external events
		istanbul.RequestEvent{},
		istanbul.MessageEvent{},
		// internal events
		backlogEvent{},
	)
	c.timeoutSub = c.backend.EventMux().Subscribe(
		timeoutEvent{},
	)
	c.finalCommittedSub = c.backend.EventMux().Subscribe(
		istanbul.FinalCommittedEvent{},
	)
}

// Unsubscribe all events
func (c *core) unsubscribeEvents() {
	c.events.Unsubscribe()
	c.timeoutSub.Unsubscribe()
	c.finalCommittedSub.Unsubscribe()
}

func (c *core) handleEvents() {
	// Clear state
	defer func() {
		c.current = nil
		c.handlerWg.Done()
	}()

	c.handlerWg.Add(1)
	for {
		select {
		case event, ok := <-c.events.Chan():
			if !ok {
				return
			}

			// A real event arrived, process interesting content
			switch ev := event.Data.(type) {
			case istanbul.RequestEvent:
				r := &Request{
					Proposal: ev.Proposal,
				}
				err := c.handleRequest(r)
				if err == errFutureMessage {
					c.storeRequestMsg(r)
				}
			case istanbul.MessageEvent:
				if _, ok := qbfttypes.MessageCodes()[ev.Code]; !ok {
					c.logger.Error("QBFT: Invalid message code on MessageEvent", "code", ev.Code)
					continue
				}
				//c.logger.Warn("QBFT: MessageEvent", "code", ev.Code)
				if err := c.handleEncodedMsg(ev.Code, ev.Payload); err != nil {
					continue
				}
				c.backend.Gossip(c.valSet, ev.Code, ev.Payload)
			case backlogEvent:
				c.logger.Warn("QBFT: BacklogEvent", "code", ev.msg.Code())
				// No need to check signature for internal messages
				if err := c.handleDecodedMessage(ev.msg); err != nil {
					c.logger.Error("QBFT: Error handling message from backlog", "msg", ev.msg, "err", err)
				}
				data, err := rlp.EncodeToBytes(ev.msg)
				if err != nil {
					c.logger.Error("QBFT: Error encoding backlog message", "err", err)
					continue
				}
				c.backend.Gossip(c.valSet, ev.msg.Code(), data)
			}
		case _, ok := <-c.timeoutSub.Chan():
			if !ok {
				return
			}
			c.handleTimeoutMsg()
		case event, ok := <-c.finalCommittedSub.Chan():
			if !ok {
				return
			}
			switch event.Data.(type) {
			case istanbul.FinalCommittedEvent:
				c.handleFinalCommitted()
			}
		}
	}
}

// sendEvent sends events to mux
func (c *core) sendEvent(ev interface{}) {
	c.backend.EventMux().Post(ev)
}

func (c *core) handleEncodedMsg(code uint64, data []byte) error {
	// Decode data into a QBFTMessage
	m, err := qbfttypes.Decode(code, data)
	if err != nil {
		c.logger.Error("QBFT: Error decoding message", "code", code, "err", err)
		return err
	}

	// Verify signatures and set source address
	if err = c.verifySignatures(m); err != nil {
		return err
	}

	return c.handleDecodedMessage(m)

}

func (c *core) handleDecodedMessage(m qbfttypes.QBFTMessage) error {
	view := m.View()
	//c.logger.Info("QBFT: handleDecodedMessage", "code", m.Code(), "view", view)

	if err := c.checkMessage(m.Code(), &view); err != nil {
		// Store in the backlog it it's a future message
		if err == errFutureMessage {
			c.storeQBFTBacklog(m)
		}
		return err
	}
	return c.deliverMessage(m)
}

// Deliver to specific message handler
func (c *core) deliverMessage(m qbfttypes.QBFTMessage) error {
	var err error

	switch m.Code() {
	case qbfttypes.PreprepareCode:
		err = c.handlePreprepareMsg(m.(*qbfttypes.Preprepare))
	case qbfttypes.PrepareCode:
		err = c.handlePrepare(m.(*qbfttypes.Prepare))
	case qbfttypes.CommitCode:
		err = c.handleCommitMsg(m.(*qbfttypes.Commit))
	case qbfttypes.RoundChangeCode:
		err = c.handleRoundChange(m.(*qbfttypes.RoundChange))
	default:
		c.logger.Error("QBFT: Error invalid message code", "code", m.Code())
		return errInvalidMessage
	}

	return err
}

func (c *core) handleTimeoutMsg() {
	// Start the new round
	round := c.current.Round()
	nextRound := new(big.Int).Add(round, common.Big1)
	c.logger.Warn("QBFT: TIMER CHANGING ROUND", "pr", c.current.preparedRound)
	c.startNewRound(nextRound)
	c.logger.Warn("QBFT: TIMER CHANGED ROUND", "pr", c.current.preparedRound)
	// Send Round Change
	c.broadcastRoundChange(nextRound)
}

// Verifies the signature of the message m and of any justification payloads
// piggybacked in m, if any. It also sets the source address on the messages
// and justification payloads.
func (c *core) verifySignatures(m qbfttypes.QBFTMessage) error {
	// Anonymous function to verify the signature of a single message or payload
	verify := func(m qbfttypes.QBFTMessage) error {
		payload, err := m.EncodePayloadForSigning()
		if err != nil {
			c.logger.Error("QBFT: Error encoding payload", "code", m.Code(), "err", err)
		}
		source, err := c.validateFn(payload, m.Signature())
		if err != nil {
			c.logger.Error("QBFT: Error verifying signature", "msg", m, "err", err)
			return errInvalidSigner
		}
		m.SetSource(source)
		return nil
	}

	// Verifies the signature of the message
	if err := verify(m); err != nil {
		return err
	}

	// Verifies the signature of piggybacked justification payloads.
	switch msgType := m.(type) {
	case *qbfttypes.RoundChange:
		signedPreparePayloads := msgType.Justification
		for _, p := range signedPreparePayloads {
			if err := verify(p); err != nil {
				return err
			}
		}
	case *qbfttypes.Preprepare:
		signedRoundChangePayloads := msgType.JustificationRoundChanges
		for _, p := range signedRoundChangePayloads {
			if err := verify(p); err != nil {
				return err
			}
		}
	}

	return nil
}
