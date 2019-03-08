package private

import (
	"os"

	"github.com/ethereum/go-ethereum/private/constellation"
)

type PrivateTransactionManager interface {
	Send(data []byte, from string, to []string, affectedCATransactions []string, execHash string) ([]byte, error)
	SendSignedTx(data []byte, to []string, affectedCATransactions []string, execHash string) ([]byte, error)
	Receive(data []byte) ([]byte, []string, string, error)
}

func FromEnvironmentOrNil(name string) PrivateTransactionManager {
	cfgPath := os.Getenv(name)
	if cfgPath == "" {
		return nil
	}
	return constellation.MustNew(cfgPath)
}

var P = FromEnvironmentOrNil("PRIVATE_CONFIG")
