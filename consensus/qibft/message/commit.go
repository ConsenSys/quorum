package message

import (
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
)

// A QBFT COMMIT message.
type Commit struct {
	CommonPayload
	Digest     common.Hash
	CommitSeal []byte
}

func NewCommit(sequence *big.Int, round *big.Int, digest common.Hash, seal []byte) *Commit {
	return &Commit{
		CommonPayload: CommonPayload{
			code:     CommitCode,
			Sequence: sequence,
			Round:    round,
		},
		Digest:     digest,
		CommitSeal: seal,
	}
}

func (m *Commit) EncodePayload() ([]byte, error) {
	return rlp.EncodeToBytes([]interface{}{m.Sequence, m.Round, m.Digest, m.CommitSeal})
}

func (m *Commit) decodePayload(stream *rlp.Stream) error {
	var payload struct {
		Sequence   *big.Int
		Round      *big.Int
		Digest     common.Hash
		CommitSeal []byte
	}
	if err := stream.Decode(&payload); err != nil {
		return err
	}
	m.Sequence = payload.Sequence
	m.Round = payload.Round
	m.Digest = payload.Digest
	m.CommitSeal = payload.CommitSeal
	return nil
}

func (m *Commit) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		[]interface{}{m.Sequence, m.Round, m.Digest, m.CommitSeal},
		m.signature})
}

func (m *Commit) DecodeRLP(stream *rlp.Stream) error {
	var message struct {
		Payload struct {
			Sequence   *big.Int
			Round      *big.Int
			Digest     common.Hash
			CommitSeal []byte
		}
		Signature []byte
	}
	if err := stream.Decode(&message); err != nil {
		return err
	}
	m.Sequence = message.Payload.Sequence
	m.Round = message.Payload.Round
	m.Digest = message.Payload.Digest
	m.CommitSeal = message.Payload.CommitSeal
	m.signature = message.Signature
	return nil
}
