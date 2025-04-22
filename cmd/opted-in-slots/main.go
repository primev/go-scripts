package main

import (
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
)

type optedInValidator struct {
	pubKey         []byte
	optInBlock     uint64
	optInType      string
	podOwner       common.Address
	vault          common.Address
	operator       common.Address
	withdrawalAddr common.Address
}

func main() {
	validators, err := loadValidatorsFromCSV()
	if err != nil {
		log.Fatalf("Failed to load validators from CSV: %v", err)
	}

	fmt.Println(validators)
}

func loadValidatorsFromCSV() ([]optedInValidator, error) {

	csvPath := filepath.Join("..", "all-mainnet-regs", "opted_in_validators.csv")

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
	validators := []optedInValidator{}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("Error reading CSV record: %v\n", err)
			continue
		}

		pubKey, err := hex.DecodeString(record[0])
		if err != nil {
			fmt.Printf("Error decoding pubKey: %v\n", err)
			continue
		}

		optInBlock, err := strconv.ParseUint(record[1], 10, 64)
		if err != nil {
			fmt.Printf("Error parsing optInBlock: %v\n", err)
			continue
		}

		validator := optedInValidator{
			pubKey:         pubKey,
			optInBlock:     optInBlock,
			optInType:      record[2],
			podOwner:       common.HexToAddress(record[3]),
			vault:          common.HexToAddress(record[4]),
			operator:       common.HexToAddress(record[5]),
			withdrawalAddr: common.HexToAddress(record[6]),
		}

		validators = append(validators, validator)
	}
	fmt.Printf("Loaded %d validators from CSV\n", len(validators))
	return validators, nil
}
