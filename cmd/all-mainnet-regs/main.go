package main

import (
	"bytes"
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
	startBlock := uint64(21162202) // deployment block

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

	// 1) B: deduped without sanity check
	allDeduped := dedupeValidators(optedInValidators)

	// 2) A: sanity checked (on deduped) and deduped again for safety (idempotent)
	routerPassing := sanityCheckAgainstRouter(allDeduped, routerCaller)

	// 3) Diff = B − A  (registered but fail the router sanity check)
	routerFailing := diffByPubKey(allDeduped, routerPassing)

	// 4) Exports (optional but handy)
	exportToCsvAs("unique_optedIn_unchecked.csv", allDeduped)
	exportToCsvAs("unique_optedIn_checked.csv", routerPassing)
	exportToCsvAs("unique_check_failing_validators.csv", routerFailing)
}

// helper
func dedupeValidators(in []optedInValidator) []optedInValidator {
	m := make(map[string]optedInValidator, len(in))

	for _, v := range in {
		key := hex.EncodeToString(v.pubKey)

		if exist, ok := m[key]; ok {
			// keep earliest opt-in block
			if v.optInBlock != 0 && (exist.optInBlock == 0 || v.optInBlock < exist.optInBlock) {
				exist.optInBlock = v.optInBlock
			}
			// prefer first non-empty/zero values
			if exist.optInType == "" && v.optInType != "" {
				exist.optInType = v.optInType
			}
			if (exist.podOwner == common.Address{}) && (v.podOwner != common.Address{}) {
				exist.podOwner = v.podOwner
			}
			if (exist.vault == common.Address{}) && (v.vault != common.Address{}) {
				exist.vault = v.vault
			}
			if (exist.operator == common.Address{}) && (v.operator != common.Address{}) {
				exist.operator = v.operator
			}
			if (exist.withdrawalAddr == common.Address{}) && (v.withdrawalAddr != common.Address{}) {
				exist.withdrawalAddr = v.withdrawalAddr
			}

			m[key] = exist
		} else {
			m[key] = v
		}
	}

	out := make([]optedInValidator, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}

	// deterministic order: by optInBlock asc, then by pubkey bytes
	sort.Slice(out, func(i, j int) bool {
		if out[i].optInBlock == out[j].optInBlock {
			return bytes.Compare(out[i].pubKey, out[j].pubKey) < 0
		}
		return out[i].optInBlock < out[j].optInBlock
	})

	return out
}

func sanityCheckAgainstRouter(optedInValidators []optedInValidator, routerCaller *validatoroptinrouter.ValidatoroptinrouterCaller) []optedInValidator {
	batchSize := 50
	filtered := make([]optedInValidator, 0, len(optedInValidators))
	for i := 0; i < len(optedInValidators); i += batchSize {
		end := i + batchSize
		fmt.Printf("Checking batch %d to %d against router\n", i, end)
		if end > len(optedInValidators) {
			end = len(optedInValidators)
		}
		batch := make([][]byte, 0, end-i)
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
				filtered = append(filtered, optedInValidators[i+idxValidator])
			}
		}
	}
	return filtered
}

func diffByPubKey(minuend, subtrahend []optedInValidator) []optedInValidator {
	// minuend: typically "all deduped"
	// subtrahend: typically "sanity-checked deduped"
	has := make(map[string]struct{}, len(subtrahend))
	for _, v := range subtrahend {
		has[hex.EncodeToString(v.pubKey)] = struct{}{}
	}
	out := make([]optedInValidator, 0)
	for _, v := range minuend {
		key := hex.EncodeToString(v.pubKey)
		if _, ok := has[key]; !ok {
			out = append(out, v)
		}
	}
	return out
}

func exportToCsvAs(filename string, validators []optedInValidator) {
	fmt.Printf("Exporting %d validators to %s\n", len(validators), filename)
	csvFile, err := os.Create(filename)
	if err != nil {
		log.Fatalf("Failed to create CSV file: %v", err)
	}
	defer csvFile.Close()

	sort.Slice(validators, func(i, j int) bool {
		return validators[i].optInBlock < validators[j].optInBlock
	})

	writer := csv.NewWriter(csvFile)
	_ = writer.Write([]string{"pubKey", "optInBlock", "optInType", "podOwner", "vault", "operator", "withdrawalAddr"})
	for _, v := range validators {
		_ = writer.Write([]string{
			hex.EncodeToString(v.pubKey),
			fmt.Sprintf("%d", v.optInBlock),
			v.optInType,
			v.podOwner.Hex(),
			v.vault.Hex(),
			v.operator.Hex(),
			v.withdrawalAddr.Hex(),
		})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		log.Fatalf("Failed to write CSV file: %v", err)
	}
	fmt.Printf("Export complete: %s (%d rows)\n", filename, len(validators))
}
