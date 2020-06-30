package core

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/private/engine"
)

type pmcStateTransitionAPI interface {
	SetTxPrivacyMetadata(pm *types.PrivacyMetadata)
	IsPrivacyEnhancementsEnabled() bool
	RevertToSnapshot(int)
	GetStatePrivacyMetadata(addr common.Address) (*state.PrivacyMetadata, error)
	CalculateMerkleRoot() (common.Hash, error)
	AffectedContracts() []common.Address
}

func newPMC(st pmcStateTransitionAPI) *privateMessageContext {
	return &privateMessageContext{stAPI: st}
}

type privateMessageContext struct {
	stAPI pmcStateTransitionAPI

	isPrivate         bool
	hasPrivatePayload bool

	snapshot                int
	receivedPrivacyMetadata *engine.ExtraMetadata

	// tx status
	failed bool
}

func (pmc *privateMessageContext) mustVerify() bool {
	return pmc.isPrivate && pmc.hasPrivatePayload && pmc.receivedPrivacyMetadata != nil && pmc.stAPI.IsPrivacyEnhancementsEnabled()
}

// checks and adjusts privacy metadata in the state transition context
// returns false if TransitionDb needs to exit early
// true otherwise
func (pmc *privateMessageContext) prepare(contractCreation bool) bool {
	if pmc.receivedPrivacyMetadata != nil {
		// if privacy enhancements are disabled we should treat all transactions as StandardPrivate
		if !pmc.stAPI.IsPrivacyEnhancementsEnabled() && pmc.receivedPrivacyMetadata.PrivacyFlag.IsNotStandardPrivate() {
			log.Warn("Non StandardPrivate transaction received but PrivacyEnhancements are disabled. Enhanced privacy metadata will be ignored.")
			pmc.receivedPrivacyMetadata = &engine.ExtraMetadata{
				ACHashes:     make(common.EncryptedPayloadHashes),
				ACMerkleRoot: common.Hash{},
				PrivacyFlag:  engine.PrivacyFlagStandardPrivate}
		}

		if !contractCreation && pmc.receivedPrivacyMetadata.PrivacyFlag == engine.PrivacyFlagStateValidation && common.EmptyHash(pmc.receivedPrivacyMetadata.ACMerkleRoot) {
			log.Error("Privacy metadata has empty MR for stateValidation flag")
			pmc.failed = true
			return false
		}
		privMetadata := types.NewTxPrivacyMetadata(pmc.receivedPrivacyMetadata.PrivacyFlag)
		pmc.stAPI.SetTxPrivacyMetadata(privMetadata)
	}
	return true
}

//If the list of affected CA Transactions by the time evm executes is different from the list of affected contract transactions returned from Tessera
//an Error should be thrown and the state should not be updated
//This validation is to prevent cases where the list of affected contract will have changed by the time the evm actually executes transaction
// failed = true will make sure receipt is marked as "failure"
// return error will crash the node and only use when that's the case
func (pmc *privateMessageContext) verify(vmerr error) (bool, error) {
	// convenient function to return error. It has the same signature as the main function
	returnErrorFunc := func(anError error, logMsg string, ctx ...interface{}) (exitEarly bool, err error) {
		if logMsg != "" {
			log.Error(logMsg, ctx...)
		}
		pmc.stAPI.RevertToSnapshot(pmc.snapshot)
		exitEarly = true
		pmc.failed = true
		if anError != nil {
			err = fmt.Errorf("vmerr=%s, err=%s", vmerr, anError)
		}
		return
	}
	actualACAddresses := pmc.stAPI.AffectedContracts()
	log.Trace("Verify hashes of affected contracts", "expectedHashes", pmc.receivedPrivacyMetadata.ACHashes, "numberOfAffectedAddresses", len(actualACAddresses))
	privacyFlag := pmc.receivedPrivacyMetadata.PrivacyFlag
	actualACHashesLength := 0
	for _, addr := range actualACAddresses {
		actualPrivacyMetadata, err := pmc.stAPI.GetStatePrivacyMetadata(addr)
		//when privacyMetadata should have been recovered but wasnt (includes non-party)
		//non party will only be caught here if sender provides privacyFlag
		if err != nil && privacyFlag.IsNotStandardPrivate() {
			return returnErrorFunc(nil, "PrivacyMetadata unable to be found", "err", err)
		}
		log.Trace("Privacy metadata", "affectedAddress", addr.Hex(), "metadata", actualPrivacyMetadata)
		// both public and standard private contracts will be nil and can be skipped in acoth check
		// public contracts - evm error for write, no error for reads
		// standard private - only error if privacyFlag sent with tx or if no flag sent but other affecteds have privacyFlag
		if actualPrivacyMetadata == nil {
			continue
		}
		// Check that the affected contracts privacy flag matches the transaction privacy flag.
		// I know that this is also checked by tessera, but it only checks for non standard private transactions.
		if actualPrivacyMetadata.PrivacyFlag != pmc.receivedPrivacyMetadata.PrivacyFlag {
			return returnErrorFunc(nil, "Mismatched privacy flags",
				"affectedContract.Address", addr.Hex(),
				"affectedContract.PrivacyFlag", actualPrivacyMetadata.PrivacyFlag,
				"received.PrivacyFlag", pmc.receivedPrivacyMetadata.PrivacyFlag)
		}
		// acoth check - case where node isn't privy to one of actual affecteds
		if pmc.receivedPrivacyMetadata.ACHashes.NotExist(actualPrivacyMetadata.CreationTxHash) {
			return returnErrorFunc(nil, "Participation check failed",
				"affectedContractAddress", addr.Hex(),
				"missingCreationTxHash", actualPrivacyMetadata.CreationTxHash.Hex())
		}
		actualACHashesLength++
	}
	// acoth check - case where node is missing privacyMetadata for an affected it should be privy to
	if len(pmc.receivedPrivacyMetadata.ACHashes) != actualACHashesLength {
		return returnErrorFunc(nil, "Participation check failed",
			"missing", len(pmc.receivedPrivacyMetadata.ACHashes)-actualACHashesLength)
	}
	// check the psv merkle root comparison - for both creation and msg calls
	if !common.EmptyHash(pmc.receivedPrivacyMetadata.ACMerkleRoot) {
		log.Trace("Verify merkle root", "merkleRoot", pmc.receivedPrivacyMetadata.ACMerkleRoot)
		actualACMerkleRoot, err := pmc.stAPI.CalculateMerkleRoot()
		if err != nil {
			return returnErrorFunc(err, "")
		}
		if actualACMerkleRoot != pmc.receivedPrivacyMetadata.ACMerkleRoot {
			return returnErrorFunc(nil, "Merkle Root check failed", "actual", actualACMerkleRoot,
				"expect", pmc.receivedPrivacyMetadata.ACMerkleRoot)
		}
	}
	return false, nil
}
