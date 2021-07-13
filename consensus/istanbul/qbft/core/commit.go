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
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

func (c *core) broadcastCommit() {
	var err error

	logger := c.logger.New("state", c.state)

	sub := c.current.Subject()

	var header *types.Header
	if block, ok := c.current.Proposal().(*types.Block); ok {
		header = block.Header()
	}
	// Create Commit Seal
	commitSeal, err := c.backend.SignWithoutHashing(PrepareCommittedSeal(header, uint32(c.currentView().Round.Uint64())))
	if err != nil {
		logger.Error("QBFT: Failed to create commit seal", "sub", sub, "err", err)
		return
	}

	commit := qbfttypes.NewCommit(sub.View.Sequence, sub.View.Round, sub.Digest, commitSeal)
	commit.SetSource(c.Address())

	// Sign Message
	encodedPayload, err := commit.EncodePayloadForSigning()
	if err != nil {
		logger.Error("QBFT: Failed to encode payload of commit message", "msg", commit, "err", err)
		return
	}

	signature, err := c.backend.Sign(encodedPayload)
	if err != nil {
		logger.Error("QBFT: Failed to sign commit message", "msg", commit, "err", err)
		return
	}
	commit.SetSignature(signature)

	// RLP-encode message
	payload, err := rlp.EncodeToBytes(&commit)
	if err != nil {
		logger.Error("QBFT: Failed to encode commit message", "msg", commit, "err", err)
		return
	}

	logger.Info("QBFT: broadcastCommitMsg", "m", sub, "payload", hexutil.Encode(payload))
	// Broadcast RLP-encoded message
	if err = c.backend.Broadcast(c.valSet, commit.Code(), payload); err != nil {
		logger.Error("QBFT: Failed to broadcast message", "msg", commit, "err", err)
		return
	}
}

func (c *core) handleCommitMsg(commit *qbfttypes.Commit) error {
	logger := c.logger.New("state", c.state)

	logger.Info("QBFT: handleCommitMsg", "msg", &commit)

	// Check digest
	if commit.Digest != c.current.Proposal().Hash() {
		logger.Error("QBFT: Failed to check digest")
		return errInvalidMessage
	}

	// Add to received msgs
	if err := c.current.QBFTCommits.Add(commit); err != nil {
		c.logger.Error("QBFT: Failed to save commit message", "msg", commit, "err", err)
		return err
	}

	logger.Info("QBFT: commit threshold", "commits", c.current.QBFTCommits.Size(), "quorum", c.QuorumSize())
	// Check threshold and decide
	if c.current.QBFTCommits.Size() >= c.QuorumSize() {
		logger.Info("QBFT: Reached commit threshold")
		c.commitQBFT()
	}

	return nil
}

func (c *core) commitQBFT() {
	c.setState(StateCommitted)

	proposal := c.current.Proposal()
	if proposal != nil {
		committedSeals := make([][]byte, c.current.QBFTCommits.Size())
		for i, msg := range c.current.QBFTCommits.Values() {
			committedSeals[i] = make([]byte, types.IstanbulExtraSeal)
			commitMsg := msg.(*qbfttypes.Commit)
			copy(committedSeals[i][:], commitMsg.CommitSeal[:])
		}

		if err := c.backend.Commit(proposal, committedSeals, c.currentView().Round); err != nil {
			c.logger.Error("QBFT: Error committing", "err", err)
			c.broadcastNextRoundChange()
			return
		}
	}
}
