package main

import (
	"context"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"os"
	"sort"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/primevprotocol/validator-registry/pkg/mevcommitavs"
	"github.com/primevprotocol/validator-registry/pkg/mevcommitmiddleware"
	"github.com/primevprotocol/validator-registry/pkg/validatoroptinrouter"
	"github.com/primevprotocol/validator-registry/pkg/vanillaregistry"
)

type optedInValidator struct {
	pubKey         []byte
	optInType      string
	optInBlock     uint64
	podOwner       common.Address
	vault          common.Address
	operator       common.Address
	withdrawalAddr common.Address
}

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

	validatorOptInRouterAddress := common.HexToAddress("0x821798d7b9d57dF7Ed7616ef9111A616aB19ed64")
	routerCaller, err := validatoroptinrouter.NewValidatoroptinrouterCaller(validatorOptInRouterAddress, client)
	if err != nil {
		log.Fatalf("Failed to create Validator Registry caller: %v", err)
	}

	latestBlock, err := client.BlockNumber(context.Background())
	if err != nil {
		log.Fatalf("Failed to get latest block number: %v", err)
	}

	batchSize := uint64(50000)
	startBlock := uint64(21950000) // TODO: revisit start block

	optedInValidators := make([]optedInValidator, 0, 1000)

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
			optedInValidators = append(optedInValidators, optedInValidator{
				pubKey:     events.Event.ValidatorPubKey,
				optInType:  "Eigen",
				optInBlock: events.Event.Raw.BlockNumber,
				podOwner:   events.Event.PodOwner,
			})
		}

		middlewareEvents, err := middlewareFilterer.FilterValRecordAdded(opts, nil, nil, nil)
		if err != nil {
			log.Fatalf("Failed to filter Validator Registered events for blocks %d to %d: %v", startBlock, endBlock, err)
		}

		for middlewareEvents.Next() {
			optedInValidators = append(optedInValidators, optedInValidator{
				pubKey:     middlewareEvents.Event.BlsPubkey,
				optInType:  "Symbiotic",
				optInBlock: middlewareEvents.Event.Raw.BlockNumber,
				vault:      middlewareEvents.Event.Vault,
				operator:   middlewareEvents.Event.Operator,
			})
		}

		vanillaEvents, err := vanillaFilterer.FilterStaked(opts, nil, nil)
		if err != nil {
			log.Fatalf("Failed to filter Validator Registered events for blocks %d to %d: %v", startBlock, endBlock, err)
		}

		for vanillaEvents.Next() {
			optedInValidators = append(optedInValidators, optedInValidator{
				pubKey:         vanillaEvents.Event.ValBLSPubKey,
				optInType:      "Vanilla",
				optInBlock:     vanillaEvents.Event.Raw.BlockNumber,
				withdrawalAddr: vanillaEvents.Event.WithdrawalAddress,
			})
		}

		startBlock = endBlock + 1
	}
	sanityCheckAgainstRouter(optedInValidators, routerCaller)
	exportToCsv(optedInValidators)
}

func sanityCheckAgainstRouter(optedInValidators []optedInValidator, routerCaller *validatoroptinrouter.ValidatoroptinrouterCaller) {
	batchSize := 50
	for i := 0; i < len(optedInValidators); i += batchSize {
		end := i + batchSize
		fmt.Printf("Checking batch %d to %d against router\n", i, end)
		if end > len(optedInValidators) {
			end = len(optedInValidators)
		}
		batch := make([][]byte, 0)
		for _, validator := range optedInValidators[i:end] {
			batch = append(batch, validator.pubKey)
		}
		isOptedIn, err := routerCaller.AreValidatorsOptedIn(nil, batch)
		if err != nil {
			log.Fatalf("Failed to check if validators are opted in: %v", err)
		}
		for idxValidator := range optedInValidators[i:end] {
			if isOptedIn[idxValidator].IsAvsOptedIn ||
				isOptedIn[idxValidator].IsMiddlewareOptedIn ||
				isOptedIn[idxValidator].IsVanillaOptedIn {
				fmt.Printf("Val pubkey %s is opted in\n", hex.EncodeToString(optedInValidators[i+idxValidator].pubKey))
			} else {
				panic(fmt.Sprintf("Val pubkey %s is not opted in", hex.EncodeToString(optedInValidators[i+idxValidator].pubKey)))
			}
		}
	}
}

func exportToCsv(optedInValidators []optedInValidator) {
	fmt.Printf("Exporting %d opted in validators to csv\n", len(optedInValidators))
	csvFile, err := os.Create("opted_in_validators.csv")
	if err != nil {
		log.Fatalf("Failed to create CSV file: %v", err)
	}
	defer csvFile.Close()

	sort.Slice(optedInValidators, func(i, j int) bool {
		return optedInValidators[i].optInBlock < optedInValidators[j].optInBlock
	})

	writer := csv.NewWriter(csvFile)
	writer.Write([]string{"pubKey", "optInBlock", "podOwner", "vault", "operator", "withdrawalAddr"})
	for _, validator := range optedInValidators {
		writer.Write([]string{
			hex.EncodeToString(validator.pubKey),
			fmt.Sprintf("%d", validator.optInBlock),
			validator.podOwner.Hex(),
			validator.vault.Hex(),
			validator.operator.Hex(),
			validator.withdrawalAddr.Hex(),
		})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		log.Fatalf("Failed to write CSV file: %v", err)
	}
	fmt.Printf("Exported %d opted in validators to csv\n", len(optedInValidators))
}
