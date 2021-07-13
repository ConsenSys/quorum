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
	"github.com/ethereum/go-ethereum/common/hexutil"
	qbfttypes "github.com/ethereum/go-ethereum/consensus/istanbul/qbft/types"
	"github.com/ethereum/go-ethereum/rlp"
)

func (c *core) broadcastPrepare() {
	var err error

	logger := c.logger.New("state", c.state)

	sub := c.current.Subject()
	prepare := qbfttypes.NewPrepare(sub.View.Sequence, sub.View.Round, sub.Digest)
	prepare.SetSource(c.Address())

	// Sign Message
	encodedPayload, err := prepare.EncodePayloadForSigning()
	if err != nil {
		logger.Error("QBFT: Failed to encode payload of prepare message", "msg", prepare, "err", err)
		return
	}
	signature, err := c.backend.Sign(encodedPayload)
	if err != nil {
		logger.Error("QBFT: Failed to sign prepare message", "msg", prepare, "err", err)
		return
	}
	prepare.SetSignature(signature)

	// RLP-encode message
	payload, err := rlp.EncodeToBytes(&prepare)
	if err != nil {
		logger.Error("QBFT: Failed to encode prepare message", "msg", prepare, "err", err)
		return
	}

	logger.Info("QBFT: broadcastPrepare", "m", sub, "payload", hexutil.Encode(payload))
	// Broadcast RLP-encoded message
	if err = c.backend.Broadcast(c.valSet, prepare.Code(), payload); err != nil {
		logger.Error("QBFT: Failed to broadcast message", "msg", prepare, "err", err)
		return
	}
}

func (c *core) handlePrepare(prepare *qbfttypes.Prepare) error {
	logger := c.logger.New("state", c.state)

	logger.Info("QBFT: handlePrepare", "msg", &prepare)

	// Check digest
	if prepare.Digest != c.current.Proposal().Hash() {
		logger.Error("QBFT: Failed to check digest")
		return errInvalidMessage
	}

	// Add to received msgs
	if err := c.current.QBFTPrepares.Add(prepare); err != nil {
		c.logger.Error("QBFT: Failed to save prepare message", "msg", prepare, "err", err)
		return err
	}

	// Change to Prepared state if we've received enough PREPARE messages
	// and we are in earlier state before Prepared state.
	if (c.current.QBFTPrepares.Size() >= c.QuorumSize()) && c.state.Cmp(StatePrepared) < 0 {

		logger.Info("QBFT: have quorum of prepares")
		// IBFT REDUX
		c.current.preparedRound = c.currentView().Round
		c.QBFTPreparedPrepares = make([]*qbfttypes.Prepare, 0)
		for _, m := range c.current.QBFTPrepares.Values() {
			c.QBFTPreparedPrepares = append(
				c.QBFTPreparedPrepares,
				qbfttypes.NewPrepareWithSigAndSource(
					m.View().Sequence, m.View().Round, m.(*qbfttypes.Prepare).Digest, m.Signature(), m.Source()))
		}

		if c.current.Proposal() != nil && c.current.Proposal().Hash() == prepare.Digest {
			logger.Info("QBFT: the prepare matches the proposal", "proposal", c.current.Proposal().Hash(), "prepare", prepare.Digest)
			c.current.preparedBlock = c.current.Proposal()
		}

		c.setState(StatePrepared)
		c.broadcastCommit()
	}

	return nil
}
