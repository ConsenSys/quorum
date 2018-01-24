#!/bin/bash

#
# This is used at Container start up to run the constellation and geth nodes
#

set -u
set -e

### Configuration Options
TMCONF=/qdata/constellation/tm.conf

if [ $# -eq 2 ]; then
  GETH_ARGS="--datadir /qdata/ethereum --permissioned --raft --rpc --rpcaddr 0.0.0.0 --rpcapi admin,db,eth,debug,miner,net,shh,txpool,personal,web3,raft --unlock 0 --password /qdata/ethereum/passwords.txt --verbosity 4 --bootnodes $1 --raftjoinexisting $2"
else
  GETH_ARGS="--datadir /qdata/ethereum --permissioned --raft --rpc --rpcaddr 0.0.0.0 --rpcapi admin,db,eth,debug,miner,net,shh,txpool,personal,web3,raft --unlock 0 --password /qdata/ethereum/passwords.txt --verbosity 4 --bootnodes $1"
fi

if [ ! -d /qdata/ethereum/geth/chaindata ]; then
  echo "[*] Mining Genesis block"
  /usr/local/bin/geth --datadir /qdata/ethereum init /qdata/ethereum/genesis.json
fi

echo "[*] Starting node"
PRIVATE_CONFIG=$TMCONF nohup /usr/local/bin/geth $GETH_ARGS 2>>/qdata/logs/geth.log
