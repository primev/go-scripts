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
	vr "github.com/primevprotocol/validator-registry/pkg/validatorregistry"
)

type Event struct {
	TxOriginator string   `json:"tx_originator"`
	ValBLSPubKey string   `json:"val_bls_pub_key"`
	Amount       *big.Int `json:"amount"`
	Block        uint64   `json:"block"`
}

func main() {

	client, err := ethclient.Dial("https://chainrpc.testnet.mev-commit.xyz")
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		log.Fatalf("Failed to get chain id: %v", err)
	}
	fmt.Println("Chain ID: ", chainID)

	contractAddress := common.HexToAddress("0xF263483500e849Bd8d452c9A0F075B606ee64087") // Accurate as of 4/24/2024
	vrf, err := vr.NewValidatorregistryFilterer(contractAddress, client)
	if err != nil {
		log.Fatalf("Failed to create Validator Registry filterer: %v", err)
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

	fmt.Println("Querying all Staked events from mev-commit chain genesis...")
	iter, err := vrf.FilterStaked(filterOpts, nil)
	if err != nil {
		log.Fatalf("Failed to get staked validators: %v", err)
	}

	var stakedEvents []Event
	for iter.Next() {
		event := iter.Event
		stakedEvents = append(stakedEvents, Event{
			TxOriginator: event.TxOriginator.Hex(),
			ValBLSPubKey: common.Bytes2Hex(event.ValBLSPubKey),
			Amount:       event.Amount,
			Block:        event.Raw.BlockNumber,
		})
	}
	if err := iter.Error(); err != nil {
		log.Fatalf("Error encountered during iteration: %v", err)
	}
	serializeEvents(fmt.Sprintf("staked_events_%s_block_%d.json", currentDate, blockNumber), stakedEvents)

	fmt.Println("Querying all Unstaked events from mev-commit chain genesis...")
	iterUnstaked, err := vrf.FilterUnstaked(filterOpts, nil)
	if err != nil {
		log.Fatalf("Failed to get unstaked validators: %v", err)
	}

	var unstakedEvents []Event
	for iterUnstaked.Next() {
		event := iterUnstaked.Event
		unstakedEvents = append(unstakedEvents, Event{
			TxOriginator: event.TxOriginator.Hex(),
			ValBLSPubKey: common.Bytes2Hex(event.ValBLSPubKey),
			Amount:       event.Amount,
			Block:        event.Raw.BlockNumber,
		})
	}
	if err := iterUnstaked.Error(); err != nil {
		log.Fatalf("Error encountered during iteration: %v", err)
	}
	serializeEvents(fmt.Sprintf("unstaked_events_%s_block_%d.json", currentDate, blockNumber), unstakedEvents)

	fmt.Println("Querying all Withdraw events from mev-commit chain genesis...")
	iterWithdraw, err := vrf.FilterStakeWithdrawn(filterOpts, nil)
	if err != nil {
		log.Fatalf("Failed to get withdraw events: %v", err)
	}

	var withdrawEvents []Event
	for iterWithdraw.Next() {
		event := iterWithdraw.Event
		withdrawEvents = append(withdrawEvents, Event{
			TxOriginator: event.TxOriginator.Hex(),
			ValBLSPubKey: common.Bytes2Hex(event.ValBLSPubKey),
			Amount:       event.Amount,
			Block:        event.Raw.BlockNumber,
		})
	}
	if err := iterWithdraw.Error(); err != nil {
		log.Fatalf("Error encountered during iteration: %v", err)
	}
	serializeEvents(fmt.Sprintf("withdraw_events_%s_block_%d.json", currentDate, blockNumber), withdrawEvents)

	fmt.Println("Events have been serialized to JSON files.")
}
