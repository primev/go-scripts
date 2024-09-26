package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/primevprotocol/validator-registry/pkg/mevcommitavsv3"
)

func main() {

	client, err := ethclient.Dial("https://ethereum-holesky-rpc.publicnode.com")
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		log.Fatalf("Failed to get chain id: %v", err)
	}
	fmt.Println("Chain ID: ", chainID)

	mevCommitAVSAddress := common.HexToAddress("0xededb8ed37a43fd399108a44646b85b780d85dd4")

	avsFilterer, err := mevcommitavsv3.NewMevcommitavsv3Filterer(mevCommitAVSAddress, client)
	if err != nil {
		log.Fatalf("Failed to create Validator Registry caller: %v", err)
	}

	// Get the latest block number
	latestBlock, err := client.BlockNumber(context.Background())
	if err != nil {
		log.Fatalf("Failed to get latest block number: %v", err)
	}

	batchSize := uint64(50000)
	startBlock := uint64(0)

	for startBlock <= latestBlock {
		endBlock := startBlock + batchSize - 1
		if endBlock > latestBlock {
			endBlock = latestBlock
		}

		opts := &bind.FilterOpts{
			Start:   startBlock,
			End:     &endBlock,
			Context: context.Background(),
		}

		events, err := avsFilterer.FilterValidatorRegistered(opts, nil)
		if err != nil {
			log.Fatalf("Failed to filter Validator Registered events for blocks %d to %d: %v", startBlock, endBlock, err)
		}

		for events.Next() {
			fmt.Printf("Block: %d, Validator PubKey: %s, Pod Owner: %s\n",
				events.Event.Raw.BlockNumber,
				common.Bytes2Hex(events.Event.ValidatorPubKey[:]),
				events.Event.PodOwner)
		}

		startBlock = endBlock + 1
	}
}
