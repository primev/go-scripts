package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	events "github.com/primevprotocol/validator-registry/pkg/events"
	utils "github.com/primevprotocol/validator-registry/pkg/utils"
	vr "github.com/primevprotocol/validator-registry/pkg/validatorregistry"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "store-events",
		Usage: "Store and validate validator registry v1 events",
		Commands: []*cli.Command{
			{
				Name:   "store",
				Usage:  "Store all events related to validator registry v1 in artifacts directory",
				Action: storeEvents,
			},
			{
				Name:   "validate",
				Usage:  "Validate events from artifacts directory",
				Action: validateEvents,
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func initClientAndFilterer() (*ethclient.Client, *vr.ValidatorregistryFilterer, error) {
	client, err := ethclient.Dial("https://chainrpc.testnet.mev-commit.xyz")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to the Ethereum client: %v", err)
	}

	contractAddress := common.HexToAddress("0xF263483500e849Bd8d452c9A0F075B606ee64087")
	vrf, err := vr.NewValidatorregistryFilterer(contractAddress, client)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Validator Registry filterer: %v", err)
	}

	return client, vrf, nil
}

func queryEvents(vrf *vr.ValidatorregistryFilterer, filterOpts *bind.FilterOpts, eventType string) ([]events.Event, error) {
	var e []events.Event

	switch eventType {
	case "staked":
		iter, err := vrf.FilterStaked(filterOpts, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get staked events: %v", err)
		}
		for iter.Next() {
			event := iter.Event
			e = append(e, events.NewEvent(
				event.TxOriginator.Hex(),
				common.Bytes2Hex(event.ValBLSPubKey),
				event.Amount,
				event.Raw.BlockNumber,
			))
		}
		if err := iter.Error(); err != nil {
			return nil, fmt.Errorf("error encountered during iteration: %v", err)
		}
	case "unstaked":
		iter, err := vrf.FilterUnstaked(filterOpts, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get unstaked events: %v", err)
		}
		for iter.Next() {
			event := iter.Event
			e = append(e, events.NewEvent(
				event.TxOriginator.Hex(),
				common.Bytes2Hex(event.ValBLSPubKey),
				event.Amount,
				event.Raw.BlockNumber,
			))
		}
		if err := iter.Error(); err != nil {
			return nil, fmt.Errorf("error encountered during iteration: %v", err)
		}
	case "withdraw":
		iter, err := vrf.FilterStakeWithdrawn(filterOpts, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get withdraw events: %v", err)
		}
		for iter.Next() {
			event := iter.Event
			e = append(e, events.NewEvent(
				event.TxOriginator.Hex(),
				common.Bytes2Hex(event.ValBLSPubKey),
				event.Amount,
				event.Raw.BlockNumber,
			))
		}
		if err := iter.Error(); err != nil {
			return nil, fmt.Errorf("error encountered during iteration: %v", err)
		}
	default:
		return nil, fmt.Errorf("unknown event type: %s", eventType)
	}

	return e, nil
}

func storeEvents(c *cli.Context) error {
	client, vrf, err := initClientAndFilterer()
	if err != nil {
		log.Fatal(err)
	}

	filterOpts := &bind.FilterOpts{Start: 0, End: nil}

	if err := os.MkdirAll("../../artifacts", os.ModePerm); err != nil {
		log.Fatalf("Failed to create artifacts directory: %v", err)
	}

	currentDate := time.Now().Format("2006-01-02_15-04-05")
	blockNumber, err := client.BlockNumber(context.Background())
	if err != nil {
		log.Fatalf("Failed to get latest block number: %v", err)
	}

	serializeEvents := func(filename string, events []events.Event) {
		file, err := os.Create(filepath.Join("../../artifacts", filename))
		if err != nil {
			log.Fatalf("Failed to create file: %v", err)
		}
		defer file.Close()

		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(events); err != nil {
			log.Fatalf("Failed to encode events to JSON: %v", err)
		}
	}

	eventTypes := []string{"staked", "unstaked", "withdraw"}
	for _, eventType := range eventTypes {
		fmt.Printf("Querying all %s events from mev-commit chain genesis...\n", eventType)
		events, err := queryEvents(vrf, filterOpts, eventType)
		if err != nil {
			log.Fatal(err)
		}
		serializeEvents(fmt.Sprintf("%s_events_%s_block_%d.json", eventType, currentDate, blockNumber), events)
	}

	fmt.Println("Events have been serialized to JSON files.")
	return nil
}

func validateEvents(c *cli.Context) error {
	stakedEvents, err := events.ReadEvents("staked")
	if err != nil {
		return err
	}

	unstakedEvents, err := events.ReadEvents("unstaked")
	if err != nil {
		return err
	}

	withdrawnEvents, err := events.ReadEvents("withdraw")
	if err != nil {
		return err
	}

	validators := reconstructValidators(stakedEvents, unstakedEvents, withdrawnEvents)

	recentEventsValidators, err := queryValidatorsFromRecentEvents()
	if err != nil {
		return err
	}

	onChainValidators, err := queryOnChainValidators()
	if err != nil {
		return err
	}

	if compareValidators(validators, recentEventsValidators) {
		fmt.Println("Validator lists match with recent events.")
	} else {
		fmt.Println("Validator lists do not match with recent events.")
		fmt.Printf("Reconstructed list length: %d\n", len(validators))
		fmt.Printf("Recent events list length: %d\n", len(recentEventsValidators))
	}

	if compareValidators(validators, onChainValidators) {
		fmt.Println("Validator lists match with on-chain data.")
		fmt.Println("NOTE THIS ASSUMES NO VALIDATOR HAS GONE THROUGH A STAKE -> UNSTAKE -> WITHDRAW -> STAKE CYCLE")
	} else {
		fmt.Println("Validator lists do not match with on-chain data.")
		fmt.Printf("Reconstructed list length: %d\n", len(validators))
		fmt.Printf("On-chain list length: %d\n", len(onChainValidators))
	}

	return nil
}

func reconstructValidators(stakedEvents, unstakedEvents, withdrawnEvents []events.Event) map[string]*big.Int {
	validators := make(map[string]*big.Int)

	for _, event := range stakedEvents {
		if _, exists := validators[event.ValBLSPubKey]; !exists {
			validators[event.ValBLSPubKey] = big.NewInt(0)
		}
		validators[event.ValBLSPubKey].Add(validators[event.ValBLSPubKey], event.Amount)
	}

	for _, event := range unstakedEvents {
		delete(validators, event.ValBLSPubKey)
	}

	for _, event := range withdrawnEvents {
		delete(validators, event.ValBLSPubKey)
	}

	return validators
}

func queryValidatorsFromRecentEvents() (map[string]*big.Int, error) {
	_, vrf, err := initClientAndFilterer()
	if err != nil {
		return nil, err
	}

	filterOpts := &bind.FilterOpts{Start: 0, End: nil}
	stakedEvents, err := queryEvents(vrf, filterOpts, "staked")
	if err != nil {
		return nil, err
	}

	unstakedEvents, err := queryEvents(vrf, filterOpts, "unstaked")
	if err != nil {
		return nil, err
	}

	withdrawnEvents, err := queryEvents(vrf, filterOpts, "withdraw")
	if err != nil {
		return nil, err
	}

	return reconstructValidators(stakedEvents, unstakedEvents, withdrawnEvents), nil
}

func queryOnChainValidators() (map[string]*big.Int, error) {
	client := utils.InitClient()
	contractAddress := common.HexToAddress("0xF263483500e849Bd8d452c9A0F075B606ee64087")
	vrc, err := vr.NewValidatorregistryCaller(contractAddress, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create Validator Registry caller: %v", err)
	}

	numStakedVals, valsetVersion, err := vrc.GetNumberOfStakedValidators(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get number of staked validators: %v", err)
	}

	aggregatedValset := utils.GetStakedValidators(vrc, numStakedVals, valsetVersion)
	validators := make(map[string]*big.Int)
	for _, val := range aggregatedValset {
		validators[common.Bytes2Hex(val)] = big.NewInt(0) // Assuming amount is not needed here
	}

	isOptedIn, err := vrc.IsStaked(nil, []byte("8ed81d776f9de04813920d48bd6f9f3804001e069b0867559a374f1de7d3d7371b4180524844655842cf5a9ffa9f4dcb"))
	if err != nil {
		return nil, fmt.Errorf("failed to get is opted in: %v", err)
	}
	fmt.Printf("Is opted in: %t\n", isOptedIn)

	return validators, nil
}

func compareValidators(reconstructed, actual map[string]*big.Int) bool {
	if len(reconstructed) != len(actual) {
		return false
	}

	toReturn := true
	for key := range reconstructed {
		if _, exists := actual[key]; !exists {
			fmt.Printf("Key %s is missing in actual validators\n", key)
			toReturn = false
		}
	}

	return toReturn
}
