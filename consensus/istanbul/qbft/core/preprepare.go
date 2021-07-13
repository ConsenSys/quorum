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
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus"
	qbfttypes "github.com/ethereum/go-ethereum/consensus/istanbul/qbft/types"
	"github.com/ethereum/go-ethereum/rlp"
)

func (c *core) sendPreprepareMsg(request *Request) {
	logger := c.logger.New("state", c.state)

	// If I'm the proposer and I have the same sequence with the proposal
	if c.current.Sequence().Cmp(request.Proposal.Number()) == 0 && c.IsProposer() {
		curView := c.currentView()
		preprepare := qbfttypes.NewPreprepare(curView.Sequence, curView.Round, request.Proposal)
		preprepare.SetSource(c.Address())

		// Sign payload
		encodedPayload, err := preprepare.EncodePayloadForSigning()
		if err != nil {
			logger.Error("QBFT: Failed to encode payload of pre-prepare message", "msg", preprepare, "err", err)
			return
		}
		signature, err := c.backend.Sign(encodedPayload)
		if err != nil {
			logger.Error("QBFT: Failed to sign pre-prepare message", "msg", preprepare, "err", err)
			return
		}
		preprepare.SetSignature(signature)

		// Justification
		if request.RCMessages != nil {
			preprepare.JustificationRoundChanges = make([]*qbfttypes.SignedRoundChangePayload, 0)
			for _, m := range request.RCMessages.Values() {
				preprepare.JustificationRoundChanges = append(preprepare.JustificationRoundChanges, &m.(*qbfttypes.RoundChange).SignedRoundChangePayload)
				logger.Info("QBFT: Appending RC justification", "rc", m.(*qbfttypes.RoundChange).SignedRoundChangePayload)
			}
			logger.Info("QBFT: On Pre-prepare", "rc justification", preprepare.JustificationRoundChanges)
		}
		if request.PrepareMessages != nil {
			preprepare.JustificationPrepares = request.PrepareMessages
			logger.Info("QBFT: On Pre-prepare", "prepare justification", preprepare.JustificationPrepares)
		}

		// RLP-encode message
		payload, err := rlp.EncodeToBytes(&preprepare)
		if err != nil {
			logger.Error("QBFT: Failed to encode pre-prepare message", "msg", preprepare, "err", err)
			return
		}

		logger.Info("QBFT: sendPreprepareMsg", "m", preprepare, "payload", hexutil.Encode(payload))
		// Broadcast RLP-encoded message
		if err = c.backend.Broadcast(c.valSet, preprepare.Code(), payload); err != nil {
			logger.Error("QBFT: Failed to broadcast message", "msg", preprepare, "err", err)
			return
		}

		// Set the preprepareSent to the current round
		c.current.preprepareSent = curView.Round
	}
}

func (c *core) handlePreprepareMsg(preprepare *qbfttypes.Preprepare) error {
	logger := c.logger.New("state", c.state)

	c.logger.Info("QBFT: handlePreprepareMsg", "view", preprepare.View(), "m", preprepare)

	// Check if the message comes from current proposer
	logger.Warn("QBFT who's proposer?", "source", preprepare.Source(), "proposer", c.valSet.GetProposer().Address())
	if !c.valSet.IsProposer(preprepare.Source()) {
		logger.Warn("Ignore preprepare messages from non-proposer")
		return errNotFromProposer
	}

	// Justification
	if preprepare.Round.Uint64() > 0 {
		if err := isJustified(preprepare.Proposal, preprepare.JustificationRoundChanges, preprepare.JustificationPrepares, c.QuorumSize()); err != nil {
			logger.Error("QBFT: Unable to justify PRE-PREPARE message", "err", err)
			return errInvalidPreparedBlock
		}
	}

	// Verify the proposal we received
	if duration, err := c.backend.Verify(preprepare.Proposal); err != nil {
		// if it's a future block, we will handle it again after the duration
		if err == consensus.ErrFutureBlock {
			logger.Info("Proposed block will be handled in the future", "err", err, "duration", duration)
			c.stopFuturePreprepareTimer()
			c.futurePreprepareTimer = time.AfterFunc(duration, func() {
				_, validator := c.valSet.GetByAddress(preprepare.Source())
				c.sendEvent(backlogEvent{
					src: validator,
					msg: preprepare,
				})
			})
		}
		return err
	}

	// Here is about to accept the PRE-PREPARE
	if c.state == StateAcceptRequest {
		c.newRoundChangeTimer()
		c.consensusTimestamp = time.Now()
		c.current.SetPreprepare(preprepare)
		c.setState(StatePreprepared)
		c.broadcastPrepare()
	}

	return nil
}
