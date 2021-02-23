package message

import (
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/consensus/qibft"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

type Preprepare struct {
	CommonPayload
	Proposal                  qibft.Proposal
	JustificationRoundChanges []*SignedRoundChangePayload
	JustificationPrepares     []*SignedPreparePayload
}

func NewPreprepare(sequence *big.Int, round *big.Int, proposal qibft.Proposal) *Preprepare {
	return &Preprepare{
		CommonPayload: CommonPayload{
			code:     PreprepareCode,
			Sequence: sequence,
			Round:    round,
		},
		Proposal: proposal,
	}
}

func (m *Preprepare) EncodePayload() ([]byte, error) {
	return rlp.EncodeToBytes(
		[]interface{}{m.Sequence, m.Round, m.Proposal})
}

func (m *Preprepare) EncodeRLP(w io.Writer) error {
	return rlp.Encode(
		w,
		[]interface{}{
			[]interface{}{
				[]interface{}{m.Sequence, m.Round, m.Proposal},
				m.signature,
			},
			[]interface{}{
				m.JustificationRoundChanges,
				m.JustificationPrepares,
			},
		})
}

func (m *Preprepare) DecodeRLP(stream *rlp.Stream) error {
	var message struct {
		SignedPayload struct {
			Payload struct {
				Sequence *big.Int
				Round    *big.Int
				Proposal *types.Block
			}
			Signature []byte
		}
		Justification struct {
			RoundChanges []*SignedRoundChangePayload
			Prepares     []*SignedPreparePayload
		}
	}
	if err := stream.Decode(&message); err != nil {
		return err
	}
	m.Sequence = message.SignedPayload.Payload.Sequence
	m.Round = message.SignedPayload.Payload.Round
	m.Proposal = message.SignedPayload.Payload.Proposal
	m.signature = message.SignedPayload.Signature
	m.JustificationPrepares = message.Justification.Prepares
	m.JustificationRoundChanges = message.Justification.RoundChanges
	return nil
}
