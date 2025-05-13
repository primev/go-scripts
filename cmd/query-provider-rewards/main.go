package main

import (
	"context"
	"fmt"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/primevprotocol/validator-registry/pkg/bidderregistry"
	"github.com/primevprotocol/validator-registry/pkg/preconfmanager"
)

func main() {

	client, err := ethclient.Dial("https://chainrpc.mev-commit.xyz/")
	if err != nil {
		log.Fatalf("Failed to connect to the mev-commit chain client: %v", err)
	}

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		log.Fatalf("Failed to get chain id: %v", err)
	}
	fmt.Println("Chain ID: ", chainID)

	preconfManagerAddr := common.HexToAddress("0x3761bF3932cD22d684A7485002E1424c3aCCD69c")
	preconfManager, err := preconfmanager.NewPreconfmanagerFilterer(preconfManagerAddr, client)
	if err != nil {
		log.Fatalf("Failed to create preconfmanager: %v", err)
	}

	bidderRegistryAddr := common.HexToAddress("0xC973D09e51A20C9Ab0214c439e4B34Dbac52AD67")
	bidderRegistry, err := bidderregistry.NewBidderregistryFilterer(bidderRegistryAddr, client)
	if err != nil {
		log.Fatalf("Failed to create bidderregistry: %v", err)
	}

	block, err := client.BlockByNumber(context.Background(), nil)
	if err != nil {
		log.Fatalf("Failed to get current block: %v", err)
	}
	fmt.Println("Current block: ", block.Number())

	endBlock := block.Number().Uint64()
	opts := &bind.FilterOpts{
		Start: 0,
		End:   &endBlock,
	}
	iter, err := preconfManager.FilterOpenedCommitmentStored(opts, nil)
	if err != nil {
		log.Fatalf("Failed to get opened commitment stored: %v", err)
	}

	providerInQuestion := common.HexToAddress("0xE3d71EF44D20917b93AA93e12Bd35b0859824A8F")

	totalBidAmt := big.NewInt(0)
	numEvents := 0
	for iter.Next() {
		commitment := iter.Event
		if commitment.Committer == providerInQuestion {
			numEvents++
			totalBidAmt.Add(totalBidAmt, commitment.BidAmt)
		}
	}
	fmt.Println("Number of events: ", numEvents)
	fmt.Println("Total bid amount: ", totalBidAmt)

	iter2, err := bidderRegistry.FilterFundsRewarded(opts, nil, nil, []common.Address{providerInQuestion})
	if err != nil {
		log.Fatalf("Failed to get funds rewarded: %v", err)
	}

	totatlFundsRewarded := big.NewInt(0)
	for iter2.Next() {
		reward := iter2.Event
		totatlFundsRewarded.Add(totatlFundsRewarded, reward.Amount)
	}
	fmt.Println("Total funds rewarded: ", totatlFundsRewarded)
}
