package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type optedInValidator struct {
	pubKey         string
	optInBlock     uint64
	optInType      string
	podOwner       common.Address
	vault          common.Address
	operator       common.Address
	withdrawalAddr common.Address
}

type optedInSlot struct {
	slot             uint64
	blockNumber      uint64
	optedInValidator optedInValidator
}

func main() {
	validators, err := loadValidatorsFromCSV()
	if err != nil {
		log.Fatalf("Failed to load validators from CSV: %v", err)
	}

	startEpoch := uint64(335000) // TODO: Followup on start/end
	endEpoch := uint64(360607)

	apiURL := "https://ethereum-beacon-api.publicnode.com"

	apiURL = strings.TrimSuffix(apiURL, "/")

	optedInSlots := []optedInSlot{}

	for epoch := startEpoch; epoch <= endEpoch; epoch++ {
		start := time.Now()
		duties, err := fetchProposerDuties(context.Background(), epoch, apiURL)
		if err != nil {
			log.Fatalf("Failed to fetch proposer duties: %v", err)
		}
		for _, duty := range duties.Data {
			validator, ok := validators[duty.Pubkey]
			if ok {
				fmt.Printf("Opted-in validator %s is proposer for epoch %d and slot %s\n", validator.pubKey, epoch, duty.Slot)
				slot, err := strconv.ParseUint(duty.Slot, 10, 64)
				if err != nil {
					log.Fatalf("Failed to parse slot: %v", err)
				}
				blockNumber, err := getBlockNumberForSlot(context.Background(), slot, apiURL)
				if err != nil {
					log.Fatalf("Failed to get block number for slot: %v", err)
				}
				fmt.Printf("Block number for slot %s: %d\n", duty.Slot, blockNumber)
				if blockNumber >= validator.optInBlock {
					optedInSlots = append(optedInSlots, optedInSlot{
						slot:             slot,
						blockNumber:      blockNumber,
						optedInValidator: validator,
					})
					fmt.Println(optedInSlots)
					panic("stop here for now. Then export to csv")
				}
			}
		}
		fmt.Printf("Time taken for epoch %d: %v\n", epoch, time.Since(start))
	}
}

func loadValidatorsFromCSV() (map[string]optedInValidator, error) {

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
	validators := map[string]optedInValidator{}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("Error reading CSV record: %v\n", err)
			continue
		}

		optInBlock, err := strconv.ParseUint(record[1], 10, 64)
		if err != nil {
			fmt.Printf("Error parsing optInBlock: %v\n", err)
			continue
		}

		validators[record[0]] = optedInValidator{
			pubKey:         record[0],
			optInBlock:     optInBlock,
			optInType:      record[2],
			podOwner:       common.HexToAddress(record[3]),
			vault:          common.HexToAddress(record[4]),
			operator:       common.HexToAddress(record[5]),
			withdrawalAddr: common.HexToAddress(record[6]),
		}
	}
	fmt.Printf("Loaded %d validators from CSV\n", len(validators))
	return validators, nil
}

type ProposerDutiesResponse struct {
	Data []struct {
		Pubkey string `json:"pubkey"`
		Slot   string `json:"slot"`
	} `json:"data"`
}

func fetchProposerDuties(ctx context.Context, epoch uint64, apiURL string) (*ProposerDutiesResponse, error) {
	fmt.Printf("Fetching proposer duties for epoch %d\n", epoch)
	url := fmt.Sprintf("%s/eth/v1/validator/duties/proposer/%d", apiURL, epoch)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "creating request: %v", err)
	}

	httpReq.Header.Set("accept", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("unexpected status code: %v\n", resp.StatusCode)

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "reading response body: %v", err)
		}

		bodyString := string(bodyBytes)
		if strings.Contains(bodyString, "Proposer duties were requested for a future epoch") {
			return nil, status.Errorf(codes.InvalidArgument, "Proposer duties were requested for a future epoch")
		}

		return nil, status.Errorf(
			codes.Internal,
			"unexpected status code: %v, response: %s", resp.StatusCode, bodyString,
		)
	}
	var dutiesResp ProposerDutiesResponse
	if err := json.NewDecoder(resp.Body).Decode(&dutiesResp); err != nil {
		fmt.Printf("decoding response: %v\n", err)
		return nil, status.Errorf(codes.Internal, "decoding response: %v", err)
	}

	return &dutiesResp, nil
}

type beaconBlockResponse struct {
	Data struct {
		Message struct {
			Body struct {
				ExecutionPayload struct {
					BlockNumber string `json:"block_number"`
				} `json:"execution_payload"`
			} `json:"body"`
		} `json:"message"`
	} `json:"data"`
}

func getBlockNumberForSlot(ctx context.Context, slot uint64, apiURL string) (
	blockNumber uint64,
	err error,
) {
	url := fmt.Sprintf("%s/eth/v2/beacon/blocks/%d", apiURL, slot)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Add("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var blockResp beaconBlockResponse
	if err := json.NewDecoder(resp.Body).Decode(&blockResp); err != nil {
		return 0, fmt.Errorf("decoding response: %w", err)
	}

	blockNumber, err = strconv.ParseUint(blockResp.Data.Message.Body.ExecutionPayload.BlockNumber, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing block number: %w", err)
	}

	return blockNumber, nil
}
