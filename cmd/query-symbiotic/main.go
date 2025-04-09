package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/primevprotocol/validator-registry/pkg/mevcommitmiddleware"
)

func main() {
	client, err := ethclient.Dial("https://ethereum-rpc.publicnode.com")
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		log.Fatalf("Failed to get chain id: %v", err)
	}
	fmt.Println("Chain ID: ", chainID)

	mevCommitMiddlewareAddress := common.HexToAddress("0x21fD239311B050bbeE7F32850d99ADc224761382")

	middlewareFilterer, err := mevcommitmiddleware.NewMevcommitmiddlewareFilterer(mevCommitMiddlewareAddress, client)
	if err != nil {
		log.Fatalf("Failed to create Validator Registry caller: %v", err)
	}

	currentBlock, err := client.BlockByNumber(context.Background(), nil)
	if err != nil {
		log.Fatalf("Failed to get current block: %v", err)
	}

	startBlock := uint64(21633063)
	batchSize := uint64(50000)

	for i := startBlock; i < currentBlock.NumberU64(); i += batchSize {
		start := i
		end := i + batchSize
		if end > currentBlock.NumberU64() {
			end = currentBlock.NumberU64()
		}
		opts := &bind.FilterOpts{
			Start:   start,
			End:     &end,
			Context: context.Background(),
		}
		operators, err := middlewareFilterer.FilterOperatorRegistered(opts, nil)
		if err != nil {
			log.Fatalf("Failed to get registered operators for blocks %d to %d: %v", start, end, err)
		}
		for operators.Next() {
			operator := operators.Event.Operator
			fmt.Println("Operator: ", operator.Hex(), "Registered in tx hash: ", operators.Event.Raw.TxHash.Hex())
		}
		if err := operators.Error(); err != nil {
			log.Fatalf("Failed to iterate through registered operators: %v", err)
		}
	}

	for i := startBlock; i < currentBlock.NumberU64(); i += batchSize {
		start := i
		end := i + batchSize
		if end > currentBlock.NumberU64() {
			end = currentBlock.NumberU64()
		}
		opts := &bind.FilterOpts{
			Start:   start,
			End:     &end,
			Context: context.Background(),
		}
		vaults, err := middlewareFilterer.FilterVaultRegistered(opts, nil)
		if err != nil {
			log.Fatalf("Failed to get registered vaults for blocks %d to %d: %v", start, end, err)
		}
		for vaults.Next() {
			vault := vaults.Event.Vault
			fmt.Println("Vault: ", vault.Hex(), "Registered in tx hash: ", vaults.Event.Raw.TxHash.Hex())
		}
		if err := vaults.Error(); err != nil {
			log.Fatalf("Failed to iterate through registered vaults: %v", err)
		}
	}
}
