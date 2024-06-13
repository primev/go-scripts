package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	vr "github.com/primevprotocol/validator-registry/pkg/validatorregistry"
	"github.com/urfave/cli/v2"
)

type Event struct {
	TxOriginator string   `json:"tx_originator"`
	ValBLSPubKey string   `json:"val_bls_pub_key"`
	Amount       *big.Int `json:"amount"`
	Block        uint64   `json:"block"`
}

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

func queryEvents(vrf *vr.ValidatorregistryFilterer, filterOpts *bind.FilterOpts, eventType string) ([]Event, error) {
	var events []Event

	switch eventType {
	case "staked":
		iter, err := vrf.FilterStaked(filterOpts, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get staked events: %v", err)
		}
		for iter.Next() {
			event := iter.Event
			events = append(events, Event{
				TxOriginator: event.TxOriginator.Hex(),
				ValBLSPubKey: common.Bytes2Hex(event.ValBLSPubKey),
				Amount:       event.Amount,
				Block:        event.Raw.BlockNumber,
			})
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
			events = append(events, Event{
				TxOriginator: event.TxOriginator.Hex(),
				ValBLSPubKey: common.Bytes2Hex(event.ValBLSPubKey),
				Amount:       event.Amount,
				Block:        event.Raw.BlockNumber,
			})
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
			events = append(events, Event{
				TxOriginator: event.TxOriginator.Hex(),
				ValBLSPubKey: common.Bytes2Hex(event.ValBLSPubKey),
				Amount:       event.Amount,
				Block:        event.Raw.BlockNumber,
			})
		}
		if err := iter.Error(); err != nil {
			return nil, fmt.Errorf("error encountered during iteration: %v", err)
		}
	default:
		return nil, fmt.Errorf("unknown event type: %s", eventType)
	}

	return events, nil
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

	serializeEvents := func(filename string, events []Event) {
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
	stakedEvents, err := readEvents("staked")
	if err != nil {
		return err
	}

	withdrawnEvents, err := readEvents("withdraw")
	if err != nil {
		return err
	}

	validators := reconstructValidators(stakedEvents, withdrawnEvents)

	actualValidators, err := queryActualValidators()
	if err != nil {
		return err
	}

	if compareValidators(validators, actualValidators) {
		fmt.Println("Validator lists match.")
	} else {
		fmt.Println("Validator lists do not match.")
		fmt.Printf("Reconstructed list length: %d\n", len(validators))
		fmt.Printf("Actual list length: %d\n", len(actualValidators))
	}

	return nil
}

func readEvents(eventType string) ([]Event, error) {
	files, err := filepath.Glob(fmt.Sprintf("../../artifacts/%s_events_*.json", eventType))
	if err != nil {
		return nil, fmt.Errorf("failed to list %s event files: %v", eventType, err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no %s event files found", eventType)
	}

	sort.Slice(files, func(i, j int) bool {
		infoI, err := os.Stat(files[i])
		if err != nil {
			log.Fatalf("Failed to stat file %s: %v", files[i], err)
		}
		infoJ, err := os.Stat(files[j])
		if err != nil {
			log.Fatalf("Failed to stat file %s: %v", files[j], err)
		}
		return infoI.ModTime().After(infoJ.ModTime())
	})

	recentFile := files[0]
	fmt.Printf("Using artifact file: %s\n", recentFile)

	f, err := os.Open(recentFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %v", recentFile, err)
	}
	defer f.Close()

	var events []Event
	if err := json.NewDecoder(f).Decode(&events); err != nil {
		return nil, fmt.Errorf("failed to decode events from file %s: %v", recentFile, err)
	}

	return events, nil
}

func reconstructValidators(stakedEvents, withdrawnEvents []Event) map[string]*big.Int {
	validators := make(map[string]*big.Int)

	for _, event := range stakedEvents {
		if _, exists := validators[event.ValBLSPubKey]; !exists {
			validators[event.ValBLSPubKey] = big.NewInt(0)
		}
		validators[event.ValBLSPubKey].Add(validators[event.ValBLSPubKey], event.Amount)
	}

	for _, event := range withdrawnEvents {
		if _, exists := validators[event.ValBLSPubKey]; exists {
			validators[event.ValBLSPubKey].Sub(validators[event.ValBLSPubKey], event.Amount)
		}
	}

	return validators
}

func queryActualValidators() (map[string]*big.Int, error) {
	_, vrf, err := initClientAndFilterer()
	if err != nil {
		return nil, err
	}

	filterOpts := &bind.FilterOpts{Start: 0, End: nil}
	stakedEvents, err := queryEvents(vrf, filterOpts, "staked")
	if err != nil {
		return nil, err
	}

	withdrawnEvents, err := queryEvents(vrf, filterOpts, "withdraw")
	if err != nil {
		return nil, err
	}

	return reconstructValidators(stakedEvents, withdrawnEvents), nil
}

func compareValidators(reconstructed, actual map[string]*big.Int) bool {
	return reflect.DeepEqual(reconstructed, actual)
}
