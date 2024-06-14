package events

import (
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"sort"
)

type Event struct {
	TxOriginator string   `json:"tx_originator"`
	ValBLSPubKey string   `json:"val_bls_pub_key"`
	Amount       *big.Int `json:"amount"`
	Block        uint64   `json:"block"`
}

func NewEvent(txOriginator string, valBLSPubKey string, amount *big.Int, block uint64) Event {
	return Event{TxOriginator: txOriginator, ValBLSPubKey: valBLSPubKey, Amount: amount, Block: block}
}

func ReadEvents(eventType string) ([]Event, error) {
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
