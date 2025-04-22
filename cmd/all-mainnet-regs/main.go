package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/primevprotocol/validator-registry/pkg/mevcommitavs"
	"github.com/primevprotocol/validator-registry/pkg/mevcommitmiddleware"
	"github.com/primevprotocol/validator-registry/pkg/vanillaregistry"
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
	if chainID.Cmp(big.NewInt(1)) != 0 {
		log.Fatalf("Chain ID is not mainnet: %v", chainID)
	}

	mevCommitAVSAddress := common.HexToAddress("0xBc77233855e3274E1903771675Eb71E602D9DC2e")
	avsFilterer, err := mevcommitavs.NewMevcommitavsFilterer(mevCommitAVSAddress, client)
	if err != nil {
		log.Fatalf("Failed to create Validator Registry caller: %v", err)
	}

	mevCommitMiddlewareAddress := common.HexToAddress("0x21fD239311B050bbeE7F32850d99ADc224761382")
	middlewareFilterer, err := mevcommitmiddleware.NewMevcommitmiddlewareFilterer(mevCommitMiddlewareAddress, client)
	if err != nil {
		log.Fatalf("Failed to create Validator Registry caller: %v", err)
	}

	vanillaRegistryAddress := common.HexToAddress("0x47afdcB2B089C16CEe354811EA1Bbe0DB7c335E9")
	vanillaFilterer, err := vanillaregistry.NewVanillaregistryFilterer(vanillaRegistryAddress, client)
	if err != nil {
		log.Fatalf("Failed to create Validator Registry caller: %v", err)
	}

	// validatorOptInRouterAddress := common.HexToAddress("0x821798d7b9d57dF7Ed7616ef9111A616aB19ed64")
	// routerCaller, err := validatoroptinrouter.NewValidatoroptinrouterCaller(validatorOptInRouterAddress, client)
	// if err != nil {
	// 	log.Fatalf("Failed to create Validator Registry caller: %v", err)
	// }

	// Get the latest block number
	latestBlock, err := client.BlockNumber(context.Background())
	if err != nil {
		log.Fatalf("Failed to get latest block number: %v", err)
	}

	batchSize := uint64(50000)
	startBlock := uint64(20000000) // TODO: revisit start block

	totalOptIns := 0

	for startBlock <= latestBlock {
		fmt.Printf("Processing blocks %d to %d\n", startBlock, startBlock+batchSize-1)
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
				hex.EncodeToString(events.Event.ValidatorPubKey),
				events.Event.PodOwner)
			totalOptIns++
		}

		middlewareEvents, err := middlewareFilterer.FilterValRecordAdded(opts, nil, nil, nil)
		if err != nil {
			log.Fatalf("Failed to filter Validator Registered events for blocks %d to %d: %v", startBlock, endBlock, err)
		}

		for middlewareEvents.Next() {
			fmt.Printf("Block: %d, Validator PubKey: %s, Vault: %s, Operator: %s\n",
				middlewareEvents.Event.Raw.BlockNumber,
				hex.EncodeToString(middlewareEvents.Event.BlsPubkey),
				middlewareEvents.Event.Vault,
				middlewareEvents.Event.Operator,
			)
			totalOptIns++
		}

		vanillaEvents, err := vanillaFilterer.FilterStaked(opts, nil, nil)
		if err != nil {
			log.Fatalf("Failed to filter Validator Registered events for blocks %d to %d: %v", startBlock, endBlock, err)
		}

		for vanillaEvents.Next() {
			fmt.Printf("Block: %d, Validator PubKey: %s, Withdrawal Address: %s\n",
				vanillaEvents.Event.Raw.BlockNumber,
				hex.EncodeToString(vanillaEvents.Event.ValBLSPubKey),
				vanillaEvents.Event.WithdrawalAddress,
			)
			totalOptIns++
		}

		startBlock = endBlock + 1
	}

	// isOptedIn, err := routerCaller.AreValidatorsOptedIn(nil, [][]byte{vanillaEvents.Event.ValBLSPubKey})
	// if err != nil {
	// 	log.Fatalf("Failed to check if validators are opted in: %v", err)
	// }
	// fmt.Printf("Is Opted In: %t\n", isOptedIn)

	fmt.Printf("Total opt ins: %d\n", totalOptIns)
}
