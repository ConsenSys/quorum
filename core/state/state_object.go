// Copyright 2014 The go-ethereum Authors
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

package state

import (
	"bytes"
	"fmt"
	"io"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/private/engine"
	"github.com/ethereum/go-ethereum/rlp"
)

var emptyCodeHash = crypto.Keccak256(nil)

type Code []byte

func (c Code) String() string {
	return string(c) //strings.Join(Disassemble(c), " ")
}

type Storage map[common.Hash]common.Hash

func (s Storage) String() (str string) {
	for key, value := range s {
		str += fmt.Sprintf("%X : %X\n", key, value)
	}

	return
}

func (s Storage) Copy() Storage {
	cpy := make(Storage)
	for key, value := range s {
		cpy[key] = value
	}

	return cpy
}

// stateObject represents an Ethereum account which is being modified.
//
// The usage pattern is as follows:
// First you need to obtain a state object.
// Account values can be accessed and modified through the object.
// Finally, call CommitTrie to write the modified storage trie into a database.
type stateObject struct {
	address  common.Address
	addrHash common.Hash // hash of ethereum address of the account
	data     Account
	db       *StateDB

	// DB error.
	// State objects are used by the consensus core and VM which are
	// unable to deal with database-level errors. Any error that occurs
	// during a database read is memoized here and will eventually be returned
	// by StateDB.Commit.
	dbErr error

	// Write caches.
	trie Trie // storage trie, which becomes non-nil on first access
	code Code // contract bytecode, which gets set when code is loaded

	// Quorum
	// contains extra data that is linked to the account
	accountExtraData *AccountExtraData

	originStorage  Storage // Storage cache of original entries to dedup rewrites, reset for every transaction
	pendingStorage Storage // Storage entries that need to be flushed to disk, at the end of an entire block
	dirtyStorage   Storage // Storage entries that have been modified in the current transaction execution
	fakeStorage    Storage // Fake storage which constructed by caller for debugging purpose.

	// Cache flags.
	// When an object is marked suicided it will be delete from the trie
	// during the "update" phase of the state transition.
	dirtyCode bool // true if the code was updated
	suicided  bool
	touched   bool
	deleted   bool
	// Quorum
	// flag to track changes in AccountExtraData
	dirtyAccountExtraData bool
}

// empty returns whether the account is considered empty.
func (s *stateObject) empty() bool {
	return s.data.Nonce == 0 && s.data.Balance.Sign() == 0 && bytes.Equal(s.data.CodeHash, emptyCodeHash)
}

// Account is the Ethereum consensus representation of accounts.
// These objects are stored in the main account trie.
type Account struct {
	Nonce    uint64
	Balance  *big.Int
	Root     common.Hash // merkle root of the storage trie
	CodeHash []byte
}

// Quorum
// attached to every private contract account
type PrivacyMetadata struct {
	CreationTxHash common.EncryptedPayloadHash `json:"creationTxHash"`
	PrivacyFlag    engine.PrivacyFlagType      `json:"privacyFlag"`
}

// Quorum
// privacyMetadataRLP struct is to make sure
// field order is preserved regardless changes in the PrivacyMetadata and its internal
//
// Edit this struct with care to make sure forward and backward compatibility
type privacyMetadataRLP struct {
	CreationTxHash common.EncryptedPayloadHash
	PrivacyFlag    engine.PrivacyFlagType

	Rest []rlp.RawValue `rlp:"tail"` // to maintain forward compatibility
}

func (p *PrivacyMetadata) DecodeRLP(stream *rlp.Stream) error {
	var dataRLP privacyMetadataRLP
	if err := stream.Decode(&dataRLP); err != nil {
		return err
	}
	p.CreationTxHash = dataRLP.CreationTxHash
	p.PrivacyFlag = dataRLP.PrivacyFlag
	return nil
}

func (p *PrivacyMetadata) EncodeRLP(writer io.Writer) error {
	return rlp.Encode(writer, privacyMetadataRLP{
		CreationTxHash: p.CreationTxHash,
		PrivacyFlag:    p.PrivacyFlag,
	})
}

// Quorum
// AccountExtraData is to contain extra data that supplements existing Account data.
// It is also maintained in a trie to support rollback.
// Note: it's important to update copy() method to make sure data history is kept
type AccountExtraData struct {
	PrivacyMetadata *PrivacyMetadata
}

// Quorum
// accountExtraDataRLP struct is to make sure
// field order is preserved regardless changes in the AccountExtraData and its internal
//
// Edit this struct with care to make sure forward and backward compatibility.
type accountExtraDataRLP struct {
	// from state.PrivacyMetadata, this is required to support
	// backward compatibility with RLP-encoded state.PrivacyMetadata.
	// Refer to rlp/doc.go for decoding rules.
	CreationTxHash common.EncryptedPayloadHash
	// from state.PrivacyMetadata, this is required to support
	// backward compatibility with RLP-encoded state.PrivacyMetadata.
	// Refer to rlp/doc.go for decoding rules.
	PrivacyFlag engine.PrivacyFlagType

	Rest []rlp.RawValue `rlp:"tail"` // to maintain forward compatibility
}

func (qmd *AccountExtraData) DecodeRLP(stream *rlp.Stream) error {
	var dataRLP accountExtraDataRLP
	if err := stream.Decode(&dataRLP); err != nil {
		return err
	}
	qmd.PrivacyMetadata = &PrivacyMetadata{
		CreationTxHash: dataRLP.CreationTxHash,
		PrivacyFlag:    dataRLP.PrivacyFlag,
	}
	return nil
}

func (qmd *AccountExtraData) EncodeRLP(writer io.Writer) error {
	return rlp.Encode(writer, accountExtraDataRLP{
		CreationTxHash: qmd.PrivacyMetadata.CreationTxHash,
		PrivacyFlag:    qmd.PrivacyMetadata.PrivacyFlag,
	})
}

func (qmd *AccountExtraData) copy() *AccountExtraData {
	if qmd == nil {
		return nil
	}
	var copyPM *PrivacyMetadata
	if qmd.PrivacyMetadata != nil {
		copyPM = &PrivacyMetadata{
			CreationTxHash: qmd.PrivacyMetadata.CreationTxHash,
			PrivacyFlag:    qmd.PrivacyMetadata.PrivacyFlag,
		}
	}
	return &AccountExtraData{
		PrivacyMetadata: copyPM,
	}
}

// newObject creates a state object.
func newObject(db *StateDB, address common.Address, data Account) *stateObject {
	if data.Balance == nil {
		data.Balance = new(big.Int)
	}
	if data.CodeHash == nil {
		data.CodeHash = emptyCodeHash
	}
	if data.Root == (common.Hash{}) {
		data.Root = emptyRoot
	}
	return &stateObject{
		db:             db,
		address:        address,
		addrHash:       crypto.Keccak256Hash(address[:]),
		data:           data,
		originStorage:  make(Storage),
		pendingStorage: make(Storage),
		dirtyStorage:   make(Storage),
	}
}

func NewStatePrivacyMetadata(creationTxHash common.EncryptedPayloadHash, privacyFlag engine.PrivacyFlagType) *PrivacyMetadata {
	return &PrivacyMetadata{
		CreationTxHash: creationTxHash,
		PrivacyFlag:    privacyFlag,
	}
}

// EncodeRLP implements rlp.Encoder.
func (s *stateObject) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, s.data)
}

// setError remembers the first non-nil error it is called with.
func (s *stateObject) setError(err error) {
	if s.dbErr == nil {
		s.dbErr = err
	}
}

func (s *stateObject) markSuicided() {
	s.suicided = true
}

func (s *stateObject) touch() {
	s.db.journal.append(touchChange{
		account: &s.address,
	})
	if s.address == ripemd {
		// Explicitly put it in the dirty-cache, which is otherwise generated from
		// flattened journals.
		s.db.journal.dirty(s.address)
	}
	s.touched = true
}

func (s *stateObject) getTrie(db Database) Trie {
	if s.trie == nil {
		var err error
		s.trie, err = db.OpenStorageTrie(s.addrHash, s.data.Root)
		if err != nil {
			s.trie, _ = db.OpenStorageTrie(s.addrHash, common.Hash{})
			s.setError(fmt.Errorf("can't create storage trie: %v", err))
		}
	}
	return s.trie
}

func (so *stateObject) storageRoot(db Database) common.Hash {
	return so.getTrie(db).Hash()
}

// GetState retrieves a value from the account storage trie.
func (s *stateObject) GetState(db Database, key common.Hash) common.Hash {
	// If the fake storage is set, only lookup the state here(in the debugging mode)
	if s.fakeStorage != nil {
		return s.fakeStorage[key]
	}
	// If we have a dirty value for this state entry, return it
	value, dirty := s.dirtyStorage[key]
	if dirty {
		return value
	}
	// Otherwise return the entry's original value
	return s.GetCommittedState(db, key)
}

// GetCommittedState retrieves a value from the committed account storage trie.
func (s *stateObject) GetCommittedState(db Database, key common.Hash) common.Hash {
	// If the fake storage is set, only lookup the state here(in the debugging mode)
	if s.fakeStorage != nil {
		return s.fakeStorage[key]
	}
	// If we have a pending write or clean cached, return that
	if value, pending := s.pendingStorage[key]; pending {
		return value
	}
	if value, cached := s.originStorage[key]; cached {
		return value
	}
	// Track the amount of time wasted on reading the storage trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { s.db.StorageReads += time.Since(start) }(time.Now())
	}
	// Otherwise load the value from the database
	enc, err := s.getTrie(db).TryGet(key[:])
	if err != nil {
		s.setError(err)
		return common.Hash{}
	}
	var value common.Hash
	if len(enc) > 0 {
		_, content, _, err := rlp.Split(enc)
		if err != nil {
			s.setError(err)
		}
		value.SetBytes(content)
	}
	s.originStorage[key] = value
	return value
}

// SetState updates a value in account storage.
func (s *stateObject) SetState(db Database, key, value common.Hash) {
	// If the fake storage is set, put the temporary state update here.
	if s.fakeStorage != nil {
		s.fakeStorage[key] = value
		return
	}
	// If the new value is the same as old, don't set
	prev := s.GetState(db, key)
	if prev == value {
		return
	}
	// New value is different, update and journal the change
	s.db.journal.append(storageChange{
		account:  &s.address,
		key:      key,
		prevalue: prev,
	})
	s.setState(key, value)
}

// SetStorage replaces the entire state storage with the given one.
//
// After this function is called, all original state will be ignored and state
// lookup only happens in the fake state storage.
//
// Note this function should only be used for debugging purpose.
func (s *stateObject) SetStorage(storage map[common.Hash]common.Hash) {
	// Allocate fake storage if it's nil.
	if s.fakeStorage == nil {
		s.fakeStorage = make(Storage)
	}
	for key, value := range storage {
		s.fakeStorage[key] = value
	}
	// Don't bother journal since this function should only be used for
	// debugging and the `fake` storage won't be committed to database.
}

func (s *stateObject) setState(key, value common.Hash) {
	s.dirtyStorage[key] = value
}

// finalise moves all dirty storage slots into the pending area to be hashed or
// committed later. It is invoked at the end of every transaction.
func (s *stateObject) finalise() {
	for key, value := range s.dirtyStorage {
		s.pendingStorage[key] = value
	}
	if len(s.dirtyStorage) > 0 {
		s.dirtyStorage = make(Storage)
	}
}

// updateTrie writes cached storage modifications into the object's storage trie.
func (s *stateObject) updateTrie(db Database) Trie {
	// Make sure all dirty slots are finalized into the pending storage area
	s.finalise()

	// Track the amount of time wasted on updating the storge trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { s.db.StorageUpdates += time.Since(start) }(time.Now())
	}
	// Insert all the pending updates into the trie
	tr := s.getTrie(db)
	for key, value := range s.pendingStorage {
		// Skip noop changes, persist actual changes
		if value == s.originStorage[key] {
			continue
		}
		s.originStorage[key] = value

		if (value == common.Hash{}) {
			s.setError(tr.TryDelete(key[:]))
			continue
		}
		// Encoding []byte cannot fail, ok to ignore the error.
		v, _ := rlp.EncodeToBytes(common.TrimLeftZeroes(value[:]))
		s.setError(tr.TryUpdate(key[:], v))
	}
	if len(s.pendingStorage) > 0 {
		s.pendingStorage = make(Storage)
	}
	return tr
}

// UpdateRoot sets the trie root to the current root hash of
func (s *stateObject) updateRoot(db Database) {
	s.updateTrie(db)

	// Track the amount of time wasted on hashing the storge trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { s.db.StorageHashes += time.Since(start) }(time.Now())
	}
	s.data.Root = s.trie.Hash()
}

// CommitTrie the storage trie of the object to db.
// This updates the trie root.
func (s *stateObject) CommitTrie(db Database) error {
	s.updateTrie(db)
	if s.dbErr != nil {
		return s.dbErr
	}
	// Track the amount of time wasted on committing the storge trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { s.db.StorageCommits += time.Since(start) }(time.Now())
	}
	root, err := s.trie.Commit(nil)
	if err == nil {
		s.data.Root = root
	}
	return err
}

// AddBalance removes amount from c's balance.
// It is used to add funds to the destination account of a transfer.
func (s *stateObject) AddBalance(amount *big.Int) {
	// EIP158: We must check emptiness for the objects such that the account
	// clearing (0,0,0 objects) can take effect.
	if amount.Sign() == 0 {
		if s.empty() {
			s.touch()
		}

		return
	}
	s.SetBalance(new(big.Int).Add(s.Balance(), amount))
}

// SubBalance removes amount from c's balance.
// It is used to remove funds from the origin account of a transfer.
func (s *stateObject) SubBalance(amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	s.SetBalance(new(big.Int).Sub(s.Balance(), amount))
}

func (s *stateObject) SetBalance(amount *big.Int) {
	s.db.journal.append(balanceChange{
		account: &s.address,
		prev:    new(big.Int).Set(s.data.Balance),
	})
	s.setBalance(amount)
}

func (s *stateObject) setBalance(amount *big.Int) {
	s.data.Balance = amount
}

// Return the gas back to the origin. Used by the Virtual machine or Closures
func (s *stateObject) ReturnGas(gas *big.Int) {}

func (s *stateObject) deepCopy(db *StateDB) *stateObject {
	stateObject := newObject(db, s.address, s.data)
	if s.trie != nil {
		stateObject.trie = db.db.CopyTrie(s.trie)
	}
	stateObject.code = s.code
	stateObject.dirtyStorage = s.dirtyStorage.Copy()
	stateObject.originStorage = s.originStorage.Copy()
	stateObject.pendingStorage = s.pendingStorage.Copy()
	stateObject.suicided = s.suicided
	stateObject.dirtyCode = s.dirtyCode
	stateObject.deleted = s.deleted
	// Quorum - copy privacy metadata fields
	stateObject.accountExtraData = s.accountExtraData
	stateObject.dirtyAccountExtraData = s.dirtyAccountExtraData

	return stateObject
}

//
// Attribute accessors
//

// Returns the address of the contract/account
func (s *stateObject) Address() common.Address {
	return s.address
}

// Code returns the contract code associated with this object, if any.
func (s *stateObject) Code(db Database) []byte {
	if s.code != nil {
		return s.code
	}
	if bytes.Equal(s.CodeHash(), emptyCodeHash) {
		return nil
	}
	code, err := db.ContractCode(s.addrHash, common.BytesToHash(s.CodeHash()))
	if err != nil {
		s.setError(fmt.Errorf("can't load code hash %x: %v", s.CodeHash(), err))
	}
	s.code = code
	return code
}

func (s *stateObject) SetCode(codeHash common.Hash, code []byte) {
	prevcode := s.Code(s.db.db)
	s.db.journal.append(codeChange{
		account:  &s.address,
		prevhash: s.CodeHash(),
		prevcode: prevcode,
	})
	s.setCode(codeHash, code)
}

func (s *stateObject) setCode(codeHash common.Hash, code []byte) {
	s.code = code
	s.data.CodeHash = codeHash[:]
	s.dirtyCode = true
}

func (s *stateObject) SetNonce(nonce uint64) {
	s.db.journal.append(nonceChange{
		account: &s.address,
		prev:    s.data.Nonce,
	})
	s.setNonce(nonce)
}

func (s *stateObject) setNonce(nonce uint64) {
	s.data.Nonce = nonce
}

// Quorum
// SetAccountExtraData modifies the AccountExtraData reference and journals it
func (s *stateObject) SetAccountExtraData(extraData *AccountExtraData) {
	current, _ := s.AccountExtraData()
	s.db.journal.append(accountExtraDataChange{
		account: &s.address,
		prev:    current,
	})
	s.setAccountExtraData(extraData)
}

// Quorum
// UpdatePrivacyMetadata updates the AccountExtraData and journals it.
// A new AccountExtraData will be created if not exists.
func (s *stateObject) SetStatePrivacyMetadata(pm *PrivacyMetadata) {
	current, _ := s.AccountExtraData()
	s.db.journal.append(accountExtraDataChange{
		account: &s.address,
		prev:    current.copy(),
	})
	if current == nil {
		current = &AccountExtraData{}
	}
	current.PrivacyMetadata = pm
	s.setAccountExtraData(current)
}

// Quorum
// setAccountExtraData modifies the AccountExtraData reference in this state object
func (s *stateObject) setAccountExtraData(extraData *AccountExtraData) {
	s.accountExtraData = extraData
	s.dirtyAccountExtraData = true
}

func (s *stateObject) CodeHash() []byte {
	return s.data.CodeHash
}

func (s *stateObject) Balance() *big.Int {
	return s.data.Balance
}

func (s *stateObject) Nonce() uint64 {
	return s.data.Nonce
}

// Quorum
// AccountExtraData returns the extra data in this state object.
// It will also update the reference by searching the accountExtraDataTrie.
//
// This method enforces on returning error and never returns (nil, nil).
func (s *stateObject) AccountExtraData() (*AccountExtraData, error) {
	if s.accountExtraData != nil {
		return s.accountExtraData, nil
	}
	val, err := s.GetCommittedAccountExtraData()
	if err != nil {
		return nil, err
	}
	s.accountExtraData = val
	return val, nil
}

// Quorum
// GetCommittedAccountExtraData looks for an entry in accountExtraDataTrie.
//
// This method enforces on returning error and never returns (nil, nil).
func (s *stateObject) GetCommittedAccountExtraData() (*AccountExtraData, error) {
	val, err := s.db.accountExtraDataTrie.TryGet(s.address.Bytes())
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve data from the accountExtraDataTrie. Cause: %v", err)
	}
	if len(val) == 0 {
		return nil, fmt.Errorf("unable to retrieve extra data for contract %s. Cause: %v", s.address.Hex(), err)
	}
	var extraData AccountExtraData
	if err := rlp.DecodeBytes(val, &extraData); err != nil {
		return nil, fmt.Errorf("unable to decode to AccountExtraData. Cause: %v", err)
	}
	return &extraData, nil
}

// Quorum - Privacy Enhancements
func (s *stateObject) PrivacyMetadata() (*PrivacyMetadata, error) {
	extraData, err := s.AccountExtraData()
	if err != nil {
		return nil, err
	}
	// extraData can't be nil. Refer to s.AccountExtraData()
	return extraData.PrivacyMetadata, nil
}

func (s *stateObject) GetCommittedPrivacyMetadata() (*PrivacyMetadata, error) {
	extraData, err := s.GetCommittedAccountExtraData()
	if err != nil {
		return nil, err
	}
	if extraData == nil {
		return nil, nil
	}
	return extraData.PrivacyMetadata, nil
}

// End Quorum - Privacy Enhancements

// Never called, but must be present to allow stateObject to be used
// as a vm.Account interface that also satisfies the vm.ContractRef
// interface. Interfaces are awesome.
func (s *stateObject) Value() *big.Int {
	panic("Value on stateObject should never be called")
}
