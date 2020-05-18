package engine

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
)

var (
	ErrPrivateTxManagerNotinUse     = errors.New("private transaction manager is not in use")
	ErrPrivateTxManagerNotReady     = errors.New("private transaction manager is not ready")
	ErrPrivateTxManagerNotSupported = errors.New("private transaction manager does not suppor this operation")
)

type NotInUsePrivateTxManager struct{}

func (dn *NotInUsePrivateTxManager) Send(data []byte, from string, to []string, extra *ExtraMetadata) (common.EncryptedPayloadHash, error) {
	return common.EncryptedPayloadHash{}, ErrPrivateTxManagerNotinUse
}

func (dn *NotInUsePrivateTxManager) StoreRaw(data []byte, from string) (common.EncryptedPayloadHash, error) {
	return common.EncryptedPayloadHash{}, ErrPrivateTxManagerNotinUse
}

func (dn *NotInUsePrivateTxManager) SendSignedTx(data common.EncryptedPayloadHash, to []string, extra *ExtraMetadata) ([]byte, error) {
	return nil, ErrPrivateTxManagerNotinUse
}

func (dn *NotInUsePrivateTxManager) Receive(data common.EncryptedPayloadHash) ([]byte, *ExtraMetadata, error) {
	return nil, nil, ErrPrivateTxManagerNotinUse
}

func (dn *NotInUsePrivateTxManager) ReceiveRaw(data common.EncryptedPayloadHash) ([]byte, *ExtraMetadata, error) {
	return nil, nil, ErrPrivateTxManagerNotinUse
}

func (dn *NotInUsePrivateTxManager) Name() string {
	return "NotInUse"
}

// Additional information for the private transaction that Private Transaction Manager carries
type ExtraMetadata struct {
	// Hashes of affected Contracts
	ACHashes common.EncryptedPayloadHashes
	// Root Hash of a Merkle Trie containing all affected contract account in state objects
	ACMerkleRoot common.Hash
	//Privacy flag for contract: standardPrivate, partyProtection, psv
	PrivacyFlag PrivacyFlagType
}

type Client struct {
	HttpClient *http.Client
	BaseURL    string
}

func (c *Client) FullPath(path string) string {
	return fmt.Sprintf("%s%s", c.BaseURL, path)
}

func (c *Client) Get(path string) (*http.Response, error) {
	return c.HttpClient.Get(c.FullPath(path))
}

type PrivacyFlagType uint64

const (
	PrivacyFlagStandardPrivate PrivacyFlagType = iota                              // 0
	PrivacyFlagPartyProtection PrivacyFlagType = 1 << PrivacyFlagType(iota-1)      // 1
	PrivacyFlagStateValidation                 = iota | PrivacyFlagPartyProtection // 3 which includes PrivacyFlagPartyProtection
)

func (f PrivacyFlagType) IsNotStandardPrivate() bool {
	return !f.IsStandardPrivate()
}

func (f PrivacyFlagType) IsStandardPrivate() bool {
	return f == PrivacyFlagStandardPrivate
}

func (f PrivacyFlagType) Has(other PrivacyFlagType) bool {
	return other&f == other
}

func (f PrivacyFlagType) HasAll(others ...PrivacyFlagType) bool {
	var all PrivacyFlagType
	for _, flg := range others {
		all = all | flg
	}
	return f.Has(all)
}

func (f PrivacyFlagType) Validate() error {
	if f == PrivacyFlagStandardPrivate || f == PrivacyFlagPartyProtection || f == PrivacyFlagStateValidation {
		return nil
	}
	return fmt.Errorf("invalid privacy flag")
}
