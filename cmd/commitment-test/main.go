package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/primevprotocol/validator-registry/pkg/preconfmanager"
)

func main() {

	client, err := ethclient.Dial("https://chainrpc.mev-commit.xyz/")
	if err != nil {
		log.Fatalf("Failed to connect to the mev-commit chain client: %v", err)
	}

	fmt.Println("Connected to mev-commit chain")

	preconfManagerAddr := common.HexToAddress("0x2ee9e88f57a7db801e114a4df7a99eb7257871e2")

	preconfManager, err := preconfmanager.NewPreconfmanagerFilterer(preconfManagerAddr, client)
	if err != nil {
		log.Fatalf("Failed to create preconfmanager: %v", err)
	}

	block, err := client.BlockByNumber(context.Background(), nil)
	if err != nil {
		log.Fatalf("Failed to get current block: %v", err)
	}

	endBlock := block.Number().Uint64()

	opts := &bind.FilterOpts{
		Start: 0,
		End:   &endBlock,
	}
	iter, err := preconfManager.FilterOpenedCommitmentStored(opts, nil)
	if err != nil {
		log.Fatalf("Failed to get unopened commitment stored: %v", err)
	}

	for iter.Next() {
		fmt.Println(iter.Event.BlockNumber)
	}

	if err := iter.Error(); err != nil {
		log.Fatalf("Failed to iterate through unopened commitment stored: %v", err)
	}
}
