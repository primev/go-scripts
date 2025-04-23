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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/sync/errgroup"
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

	startEpoch := uint64(348700) // https://beaconcha.in/epoch/348700 from Feb-27-2025 22:40:23 UTC-8
	endEpoch := uint64(360736)   // latest as of Apr-22-2025 11:30:47 UTC-7

	apiURL := trimApiURL("https://ethereum-beacon-api.publicnode.com")

	errGroup, ctx := errgroup.WithContext(context.Background())

	oneThirtyth := (endEpoch - startEpoch) / 30
	ranges := [][]uint64{
		{startEpoch, startEpoch + oneThirtyth},
		{startEpoch + oneThirtyth + 1, startEpoch + 2*oneThirtyth},
		{startEpoch + 2*oneThirtyth + 1, startEpoch + 3*oneThirtyth},
		{startEpoch + 3*oneThirtyth + 1, startEpoch + 4*oneThirtyth},
		{startEpoch + 4*oneThirtyth + 1, startEpoch + 5*oneThirtyth},
		{startEpoch + 5*oneThirtyth + 1, startEpoch + 6*oneThirtyth},
		{startEpoch + 6*oneThirtyth + 1, startEpoch + 7*oneThirtyth},
		{startEpoch + 7*oneThirtyth + 1, startEpoch + 8*oneThirtyth},
		{startEpoch + 8*oneThirtyth + 1, startEpoch + 9*oneThirtyth},
		{startEpoch + 9*oneThirtyth + 1, startEpoch + 10*oneThirtyth},
		{startEpoch + 10*oneThirtyth + 1, startEpoch + 11*oneThirtyth},
		{startEpoch + 11*oneThirtyth + 1, startEpoch + 12*oneThirtyth},
		{startEpoch + 12*oneThirtyth + 1, startEpoch + 13*oneThirtyth},
		{startEpoch + 13*oneThirtyth + 1, startEpoch + 14*oneThirtyth},
		{startEpoch + 14*oneThirtyth + 1, startEpoch + 15*oneThirtyth},
		{startEpoch + 15*oneThirtyth + 1, startEpoch + 16*oneThirtyth},
		{startEpoch + 16*oneThirtyth + 1, startEpoch + 17*oneThirtyth},
		{startEpoch + 17*oneThirtyth + 1, startEpoch + 18*oneThirtyth},
		{startEpoch + 18*oneThirtyth + 1, startEpoch + 19*oneThirtyth},
		{startEpoch + 19*oneThirtyth + 1, startEpoch + 20*oneThirtyth},
		{startEpoch + 20*oneThirtyth + 1, startEpoch + 21*oneThirtyth},
		{startEpoch + 21*oneThirtyth + 1, startEpoch + 22*oneThirtyth},
		{startEpoch + 22*oneThirtyth + 1, startEpoch + 23*oneThirtyth},
		{startEpoch + 23*oneThirtyth + 1, startEpoch + 24*oneThirtyth},
		{startEpoch + 24*oneThirtyth + 1, startEpoch + 25*oneThirtyth},
		{startEpoch + 25*oneThirtyth + 1, startEpoch + 26*oneThirtyth},
		{startEpoch + 26*oneThirtyth + 1, startEpoch + 27*oneThirtyth},
		{startEpoch + 27*oneThirtyth + 1, startEpoch + 28*oneThirtyth},
		{startEpoch + 28*oneThirtyth + 1, startEpoch + 29*oneThirtyth},
		{startEpoch + 29*oneThirtyth + 1, endEpoch},
	}

	m := sync.Mutex{}
	optedInSlots := []optedInSlot{}

	for _, r := range ranges {
		errGroup.Go(func() error {
			slots, err := queryForOptedInSlots(ctx, r[0], r[1], apiURL, validators)
			if err != nil {
				return err
			}
			m.Lock()
			optedInSlots = append(optedInSlots, slots...)
			m.Unlock()
			return nil
		})
	}

	if err := errGroup.Wait(); err != nil {
		log.Fatalf("Failed to query for opted-in slots: %v", err)
	}

	exportToCsv(optedInSlots)
}

func trimApiURL(apiURL string) string {
	return strings.TrimSuffix(apiURL, "/")
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

func queryForOptedInSlots(
	ctx context.Context,
	startEpoch uint64,
	endEpoch uint64,
	apiURL string,
	validators map[string]optedInValidator,
) ([]optedInSlot, error) {

	optedInSlots := []optedInSlot{}
	for epoch := startEpoch; epoch <= endEpoch; epoch++ {
		start := time.Now()
		fmt.Printf("Fetching proposer duties for epoch %d. Epochs left for this worker: %d\n", epoch, endEpoch-epoch)

		var duties *ProposerDutiesResponse
		var err error
		for retries := 0; retries < 5; retries++ {
			duties, err = fetchProposerDuties(ctx, epoch, apiURL)
			if err != nil {
				fmt.Printf("Failed to fetch proposer duties: %v\n", err)
				if retries == 4 {
					log.Fatalf("Failed to fetch proposer duties: %v", err)
				}
			} else {
				break
			}
			time.Sleep(time.Duration(retries) * time.Second)
		}
		for _, duty := range duties.Data {
			pubkey := strings.TrimPrefix(duty.Pubkey, "0x")
			validator, ok := validators[pubkey]
			if ok {
				slot, err := strconv.ParseUint(duty.Slot, 10, 64)
				if err != nil {
					log.Fatalf("Failed to parse slot: %v", err)
				}
				var blockNumber uint64
				for retries := 0; retries < 5; retries++ {
					blockNumber, err = getBlockNumberForSlot(ctx, slot, apiURL)
					if err != nil {
						fmt.Printf("Failed to get block number for slot: %v\n", err)
						if retries == 4 {
							log.Fatalf("Failed to get block number for slot: %v", err)
						}
					} else {
						break
					}
					time.Sleep(time.Duration(retries) * time.Second)
				}
				if blockNumber >= validator.optInBlock {
					optedInSlots = append(optedInSlots, optedInSlot{
						slot:             slot,
						blockNumber:      blockNumber,
						optedInValidator: validator,
					})
					fmt.Printf("Found opted-in slot. Slot number: %d, block number: %d, pubkey: %s\n",
						slot, blockNumber, validator.pubKey)
				}
			}
		}
		fmt.Printf("Time taken for epoch %d: %v\n", epoch, time.Since(start))
	}
	return optedInSlots, nil
}

func exportToCsv(optedInSlots []optedInSlot) {
	fmt.Printf("Exporting %d opted-in slots to csv\n", len(optedInSlots))
	csvFile, err := os.Create("opted_in_slots.csv")
	if err != nil {
		log.Fatalf("Failed to create CSV file: %v", err)
	}
	defer csvFile.Close()

	sort.Slice(optedInSlots, func(i, j int) bool {
		return optedInSlots[i].optedInValidator.optInBlock < optedInSlots[j].optedInValidator.optInBlock
	})

	writer := csv.NewWriter(csvFile)
	writer.Write([]string{"slot", "blockNumber", "pubKey", "optInBlock", "optInType", "podOwner", "vault", "operator", "withdrawalAddr"})
	for _, slot := range optedInSlots {
		writer.Write([]string{
			fmt.Sprintf("%d", slot.slot),
			fmt.Sprintf("%d", slot.blockNumber),
			slot.optedInValidator.pubKey,
			fmt.Sprintf("%d", slot.optedInValidator.optInBlock),
			slot.optedInValidator.optInType,
			slot.optedInValidator.podOwner.Hex(),
			slot.optedInValidator.vault.Hex(),
			slot.optedInValidator.operator.Hex(),
			slot.optedInValidator.withdrawalAddr.Hex(),
		})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		log.Fatalf("Failed to write CSV file: %v", err)
	}
	fmt.Printf("Exported %d opted-in slots to csv\n", len(optedInSlots))
}
