package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
)

type optedInSlot struct {
	slot           uint64
	blockNumber    uint64
	pubKey         string
	optInBlock     uint64
	optInType      string
	podOwner       common.Address
	vault          common.Address
	operator       common.Address
	withdrawalAddr common.Address
	// Only populated at end of script
	missed bool
}

type SentioResponse struct {
	SyncSqlResponse struct {
		RuntimeCost string `json:"runtimeCost"`
		Result      struct {
			Columns     []string            `json:"columns"`
			ColumnTypes map[string]string   `json:"columnTypes"`
			Rows        []map[string]string `json:"rows"`
		} `json:"result"`
	} `json:"syncSqlResponse"`
}

type OpenedCommit struct {
	BlockNumber     uint64
	BidAmt          string
	TransactionHash string
}

func main() {
	optedInSlots, err := loadOptedInSlots()
	if err != nil {
		log.Fatalf("Error loading opted-in slots: %v\n", err)
	}

	fmt.Printf("Loaded %d opted-in slots from CSV\n", len(optedInSlots))

	openedCommits, err := fetchOpenedCommits()
	if err != nil {
		log.Fatalf("Error fetching opened commits: %v\n", err)
	}

	fmt.Printf("Loaded %d opened commits from Sentio\n", len(openedCommits))

	for blockNumber, slot := range optedInSlots {
		if commit, ok := openedCommits[blockNumber]; ok {
			fmt.Printf("Not missed: %d %d\n", slot.slot, commit.BlockNumber)
			slot.missed = false
		} else {
			fmt.Printf("Missed: %d %d\n", slot.slot, blockNumber)
			slot.missed = true
		}
	}

	err = writeToCsv(optedInSlots)
	if err != nil {
		log.Fatalf("Error writing to CSV: %v\n", err)
	}
}

func fetchOpenedCommits() (map[uint64]OpenedCommit, error) {
	url := "https://endpoint.sentio.xyz/primev/mevcommit/opened_commits_apr_22"
	apiKey := "iFhXK2RmifCsv0quNQL38UrMMefVtTv1q"

	reqBody := []byte("{}")

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %v", err)
	}

	var sentioResp SentioResponse
	if err := json.Unmarshal(body, &sentioResp); err != nil {
		return nil, fmt.Errorf("error parsing JSON: %v", err)
	}

	commits := map[uint64]OpenedCommit{}
	for _, row := range sentioResp.SyncSqlResponse.Result.Rows {
		blockNum, _ := strconv.ParseUint(row["blockNumber"], 10, 64)
		commit := OpenedCommit{
			BlockNumber:     blockNum,
			BidAmt:          row["bidAmt"],
			TransactionHash: row["transaction_hash"],
		}
		commits[blockNum] = commit
	}

	return commits, nil
}

func loadOptedInSlots() (map[uint64]*optedInSlot, error) {

	csvPath := filepath.Join("..", "opted-in-slots", "opted_in_slots.csv")

	file, err := os.Open(csvPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)

	header, err := reader.Read()
	if err != nil {
		return nil, err
	}
	fmt.Printf("CSV Headers: %v\n", header)
	optedInSlots := map[uint64]*optedInSlot{}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("Error reading CSV record: %v\n", err)
			continue
		}

		slot, err := strconv.ParseUint(record[0], 10, 64)
		if err != nil {
			log.Fatalf("Error parsing slot: %v\n", err)
		}

		blockNumber, err := strconv.ParseUint(record[1], 10, 64)
		if err != nil {
			log.Fatalf("Error parsing block number: %v\n", err)
		}

		optInBlock, err := strconv.ParseUint(record[3], 10, 64)
		if err != nil {
			log.Fatalf("Error parsing opt-in block: %v\n", err)
		}

		optedInSlots[blockNumber] = &optedInSlot{
			slot:           slot,
			blockNumber:    blockNumber,
			pubKey:         record[2],
			optInBlock:     optInBlock,
			optInType:      record[4],
			podOwner:       common.HexToAddress(record[5]),
			vault:          common.HexToAddress(record[6]),
			operator:       common.HexToAddress(record[7]),
			withdrawalAddr: common.HexToAddress(record[8]),
		}
	}
	fmt.Printf("Loaded %d opted-in slots from CSV\n", len(optedInSlots))
	return optedInSlots, nil
}

func writeToCsv(optedInSlots map[uint64]*optedInSlot) error {
	csvPath := filepath.Join("..", "missed-slots", "missed_slots.csv")

	file, err := os.Create(csvPath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	writer.Write([]string{"slot", "blockNumber", "pubKey", "optInBlock", "optInType", "podOwner", "vault", "operator", "withdrawalAddr", "missed"})
	for _, slot := range optedInSlots {
		writer.Write([]string{
			fmt.Sprintf("%d", slot.slot),
			fmt.Sprintf("%d", slot.blockNumber),
			slot.pubKey,
			fmt.Sprintf("%d", slot.optInBlock),
			slot.optInType,
			slot.podOwner.Hex(),
			slot.vault.Hex(),
			slot.operator.Hex(),
			slot.withdrawalAddr.Hex(),
			fmt.Sprintf("%t", slot.missed),
		})
	}
	return nil
}
