package basic

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/log"
	binding "github.com/ethereum/go-ethereum/permission/basic/bind"
	ptype "github.com/ethereum/go-ethereum/permission/types"
)

type Basic struct {
	ContractBackend   ptype.ContractBackend
	PermInterf        *binding.PermInterface
	PermInterfSession *binding.PermInterfaceSession
}

type Audit struct {
	Backend *Basic
}

type Role struct {
	Backend *Basic
}

type Control struct {
}

type Org struct {
	Backend *Basic
}

type Node struct {
	Backend *Basic
}

type Account struct {
	Backend *Basic
}

type Init struct {
	Backend ptype.ContractBackend

	//binding contracts
	PermUpgr   *binding.PermUpgr
	PermInterf *binding.PermInterface
	PermNode   *binding.NodeManager
	PermAcct   *binding.AcctManager
	PermRole   *binding.RoleManager
	PermOrg    *binding.OrgManager

	//sessions
	PermInterfSession *binding.PermInterfaceSession
	permOrgSession    *binding.OrgManagerSession
	permNodeSession   *binding.NodeManagerSession
	permRoleSession   *binding.RoleManagerSession
	permAcctSession   *binding.AcctManagerSession
}

func (i *Init) GetAccountDetailsFromIndex(_aIndex *big.Int) (common.Address, string, string, *big.Int, bool, error) {
	return i.permAcctSession.GetAccountDetailsFromIndex(_aIndex)
}

func (i *Init) GetNumberOfAccounts() (*big.Int, error) {
	return i.permAcctSession.GetNumberOfAccounts()
}

func (i *Init) GetRoleDetailsFromIndex(_rIndex *big.Int) (struct {
	RoleId     string
	OrgId      string
	AccessType *big.Int
	Voter      bool
	Admin      bool
	Active     bool
}, error) {
	return i.permRoleSession.GetRoleDetailsFromIndex(_rIndex)
}

func (i *Init) GetNumberOfRoles() (*big.Int, error) {
	return i.permRoleSession.GetNumberOfRoles()
}

func (i *Init) GetNumberOfOrgs() (*big.Int, error) {
	return i.permOrgSession.GetNumberOfOrgs()
}

func (i *Init) UpdateNetworkBootStatus() (*types.Transaction, error) {
	return i.PermInterfSession.UpdateNetworkBootStatus()
}

func (i *Init) AddAdminAccount(_acct common.Address) (*types.Transaction, error) {
	return i.PermInterfSession.AddAdminAccount(_acct)
}

func (i *Init) AddAdminNode(url string) (*types.Transaction, error) {
	return i.PermInterfSession.AddAdminNode(url)
}

func (i *Init) SetPolicy(_nwAdminOrg string, _nwAdminRole string, _oAdminRole string) (*types.Transaction, error) {
	return i.PermInterfSession.SetPolicy(_nwAdminOrg, _nwAdminRole, _oAdminRole)
}

func (i *Init) Init(_breadth *big.Int, _depth *big.Int) (*types.Transaction, error) {
	return i.PermInterfSession.Init(_breadth, _depth)
}

func (i *Init) GetAccountDetails(_account common.Address) (common.Address, string, string, *big.Int, bool, error) {
	return i.permAcctSession.GetAccountDetails(_account)
}

func (i *Init) GetNodeDetailsFromIndex(_nodeIndex *big.Int) (string, string, *big.Int, error) {
	r, err := i.permNodeSession.GetNodeDetailsFromIndex(_nodeIndex)
	return r.OrgId, r.EnodeId, r.NodeStatus, err
}

func (i *Init) GetNumberOfNodes() (*big.Int, error) {
	return i.permNodeSession.GetNumberOfNodes()
}

func (i *Init) GetNodeDetails(enodeId string) (string, string, *big.Int, error) {
	r, err := i.permNodeSession.GetNodeDetails(enodeId)
	return r.OrgId, r.EnodeId, r.NodeStatus, err
}

func (i *Init) GetRoleDetails(_roleId string, _orgId string) (struct {
	RoleId     string
	OrgId      string
	AccessType *big.Int
	Voter      bool
	Admin      bool
	Active     bool
}, error) {
	return i.permRoleSession.GetRoleDetails(_roleId, _orgId)
}

func (i *Init) GetSubOrgIndexes(_orgId string) ([]*big.Int, error) {
	return i.permOrgSession.GetSubOrgIndexes(_orgId)
}

func (i *Init) GetOrgInfo(_orgIndex *big.Int) (string, string, string, *big.Int, *big.Int, error) {
	return i.permOrgSession.GetOrgInfo(_orgIndex)
}

func (i *Init) GetNetworkBootStatus() (bool, error) {
	return i.PermInterfSession.GetNetworkBootStatus()
}

func (i *Init) GetOrgDetails(_orgId string) (string, string, string, *big.Int, *big.Int, error) {
	return i.permOrgSession.GetOrgDetails(_orgId)
}

func (a *Audit) GetPendingOperation(_orgId string) (string, string, common.Address, *big.Int, error) {
	return a.Backend.PermInterfSession.GetPendingOp(_orgId)
}

func (c *Control) ConnectionAllowedImpl(url string) (bool, error) {
	return false, fmt.Errorf("not implemented for binding Contr")
}

func (c *Control) TransactionAllowed(_args ethapi.SendTxArgs) (bool, error) {
	return false, fmt.Errorf("not implemented for binding Contr")
}

func (r *Role) RemoveRole(_args ptype.TxArgs) (*types.Transaction, error) {
	return r.Backend.PermInterfSession.RemoveRole(_args.RoleId, _args.OrgId)
}

func (r *Role) AddNewRole(_args ptype.TxArgs) (*types.Transaction, error) {
	if _args.AccessType > 3 {
		return nil, fmt.Errorf("invalid access type given")
	}
	return r.Backend.PermInterfSession.AddNewRole(_args.RoleId, _args.OrgId, big.NewInt(int64(_args.AccessType)), _args.IsVoter, _args.IsAdmin)
}

func (o *Org) ApproveOrgStatus(_args ptype.TxArgs) (*types.Transaction, error) {
	return o.Backend.PermInterfSession.ApproveOrgStatus(_args.OrgId, big.NewInt(int64(_args.Action)))
}

func (o *Org) UpdateOrgStatus(_args ptype.TxArgs) (*types.Transaction, error) {
	return o.Backend.PermInterfSession.UpdateOrgStatus(_args.OrgId, big.NewInt(int64(_args.Action)))
}

func (o *Org) ApproveOrg(_args ptype.TxArgs) (*types.Transaction, error) {
	return o.Backend.PermInterfSession.ApproveOrg(_args.OrgId, _args.Url, _args.AcctId)
}

func (o *Org) AddSubOrg(_args ptype.TxArgs) (*types.Transaction, error) {
	return o.Backend.PermInterfSession.AddSubOrg(_args.POrgId, _args.OrgId, _args.Url)
}

func (o *Org) AddOrg(_args ptype.TxArgs) (*types.Transaction, error) {
	return o.Backend.PermInterfSession.AddOrg(_args.OrgId, _args.Url, _args.AcctId)
}

func (n *Node) ApproveBlacklistedNodeRecovery(_args ptype.TxArgs) (*types.Transaction, error) {
	return n.Backend.PermInterfSession.ApproveBlacklistedNodeRecovery(_args.OrgId, _args.Url)
}

func (n *Node) StartBlacklistedNodeRecovery(_args ptype.TxArgs) (*types.Transaction, error) {
	return n.Backend.PermInterfSession.StartBlacklistedNodeRecovery(_args.OrgId, _args.Url)
}

func (n *Node) AddNode(_args ptype.TxArgs) (*types.Transaction, error) {
	return n.Backend.PermInterfSession.AddNode(_args.OrgId, _args.Url)
}

func (n *Node) UpdateNodeStatus(_args ptype.TxArgs) (*types.Transaction, error) {
	return n.Backend.PermInterfSession.UpdateNodeStatus(_args.OrgId, _args.Url, big.NewInt(int64(_args.Action)))
}

func (a *Account) AssignAccountRole(_args ptype.TxArgs) (*types.Transaction, error) {
	return a.Backend.PermInterfSession.AssignAccountRole(_args.AcctId, _args.OrgId, _args.RoleId)
}

func (a *Account) UpdateAccountStatus(_args ptype.TxArgs) (*types.Transaction, error) {
	return a.Backend.PermInterfSession.UpdateAccountStatus(_args.OrgId, _args.AcctId, big.NewInt(int64(_args.Action)))
}

func (a *Account) StartBlacklistedAccountRecovery(_args ptype.TxArgs) (*types.Transaction, error) {
	return a.Backend.PermInterfSession.StartBlacklistedAccountRecovery(_args.OrgId, _args.AcctId)
}

func (a *Account) ApproveBlacklistedAccountRecovery(_args ptype.TxArgs) (*types.Transaction, error) {
	return a.Backend.PermInterfSession.ApproveBlacklistedAccountRecovery(_args.OrgId, _args.AcctId)
}

func (a *Account) ApproveAdminRole(_args ptype.TxArgs) (*types.Transaction, error) {
	return a.Backend.PermInterfSession.ApproveAdminRole(_args.OrgId, _args.AcctId)
}

func (a *Account) AssignAdminRole(_args ptype.TxArgs) (*types.Transaction, error) {
	return a.Backend.PermInterfSession.AssignAdminRole(_args.OrgId, _args.AcctId, _args.RoleId)
}

// This is to make sure all Contr instances are ready and initialized
//
// Required to be call after standard service start lifecycle
func (i *Init) AfterStart() error {
	log.Debug("permission service: binding contracts")
	err := i.basicBindContract()
	if err != nil {
		return err
	}
	i.initSession()
	return nil
}

func (i *Init) basicBindContract() error {
	if err := ptype.BindContract(&i.PermUpgr, func() (interface{}, error) {
		return binding.NewPermUpgr(i.Backend.PermConfig.UpgrdAddress, i.Backend.EthClnt)
	}); err != nil {
		return err
	}
	if err := ptype.BindContract(&i.PermInterf, func() (interface{}, error) {
		return binding.NewPermInterface(i.Backend.PermConfig.InterfAddress, i.Backend.EthClnt)
	}); err != nil {
		return err
	}
	if err := ptype.BindContract(&i.PermAcct, func() (interface{}, error) {
		return binding.NewAcctManager(i.Backend.PermConfig.AccountAddress, i.Backend.EthClnt)
	}); err != nil {
		return err
	}
	if err := ptype.BindContract(&i.PermNode, func() (interface{}, error) {
		return binding.NewNodeManager(i.Backend.PermConfig.NodeAddress, i.Backend.EthClnt)
	}); err != nil {
		return err
	}
	if err := ptype.BindContract(&i.PermRole, func() (interface{}, error) {
		return binding.NewRoleManager(i.Backend.PermConfig.RoleAddress, i.Backend.EthClnt)
	}); err != nil {
		return err
	}
	if err := ptype.BindContract(&i.PermOrg, func() (interface{}, error) {
		return binding.NewOrgManager(i.Backend.PermConfig.OrgAddress, i.Backend.EthClnt)
	}); err != nil {
		return err
	}
	return nil
}

func (i *Init) initSession() {
	auth := bind.NewKeyedTransactor(i.Backend.Key)
	i.PermInterfSession = &binding.PermInterfaceSession{
		Contract: i.PermInterf,
		CallOpts: bind.CallOpts{
			Pending: true,
		},
		TransactOpts: bind.TransactOpts{
			From:     auth.From,
			Signer:   auth.Signer,
			GasLimit: 47000000,
			GasPrice: big.NewInt(0),
		},
	}

	i.permOrgSession = &binding.OrgManagerSession{
		Contract: i.PermOrg,
		CallOpts: bind.CallOpts{
			Pending: true,
		},
	}

	i.permNodeSession = &binding.NodeManagerSession{
		Contract: i.PermNode,
		CallOpts: bind.CallOpts{
			Pending: true,
		},
	}

	//populate roles
	i.permRoleSession = &binding.RoleManagerSession{
		Contract: i.PermRole,
		CallOpts: bind.CallOpts{
			Pending: true,
		},
	}

	//populate accounts
	i.permAcctSession = &binding.AcctManagerSession{
		Contract: i.PermAcct,
		CallOpts: bind.CallOpts{
			Pending: true,
		},
	}
}
