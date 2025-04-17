package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strconv"
	"strings"
	"time"
)

const (
	beaconAPIURL  = "https://ethereum-beacon-api.publicnode.com"
	queryInterval = 30 * time.Second
)

type ProposerDuty struct {
	Pubkey string
	Slot   string
}

type ProposerDutiesResponse struct {
	Data []struct {
		Pubkey string `json:"pubkey"`
		Slot   string `json:"slot"`
	} `json:"data"`
}

// Cache to store duties by epoch
type DutiesCache struct {
	duties map[uint64][]ProposerDuty
}

func NewDutiesCache() *DutiesCache {
	return &DutiesCache{
		duties: make(map[uint64][]ProposerDuty),
	}
}

func (c *DutiesCache) Store(epoch uint64, duties *ProposerDutiesResponse) {
	if duties == nil {
		c.duties[epoch] = nil
		return
	}

	dutiesList := make([]ProposerDuty, 0, len(duties.Data))
	for _, duty := range duties.Data {
		dutiesList = append(dutiesList, ProposerDuty{
			Pubkey: duty.Pubkey,
			Slot:   duty.Slot,
		})
	}
	c.duties[epoch] = dutiesList
}

func (c *DutiesCache) Get(epoch uint64) ([]ProposerDuty, bool) {
	duties, ok := c.duties[epoch]
	return duties, ok
}

func (c *DutiesCache) CompareAndUpdate(epoch uint64, newDuties *ProposerDutiesResponse) bool {
	oldDuties, exists := c.Get(epoch)

	// Convert newDuties to our internal format
	var newDutiesList []ProposerDuty
	if newDuties != nil {
		newDutiesList = make([]ProposerDuty, 0, len(newDuties.Data))
		for _, duty := range newDuties.Data {
			newDutiesList = append(newDutiesList, ProposerDuty{
				Pubkey: duty.Pubkey,
				Slot:   duty.Slot,
			})
		}
	}

	// Store the new duties
	c.Store(epoch, newDuties)

	// If we didn't have duties before, they're "new" but not "changed"
	if !exists {
		return false
	}

	// Check if duties changed
	return !reflect.DeepEqual(oldDuties, newDutiesList)
}

type Client struct {
	apiURL string
	logger *log.Logger
}

func NewClient(apiURL string) *Client {
	return &Client{
		apiURL: apiURL,
		logger: log.New(os.Stdout, "[BEACON-CLIENT] ", log.LstdFlags),
	}
}

func (c *Client) FetchProposerDuties(ctx context.Context, epoch uint64) (*ProposerDutiesResponse, error) {
	url := fmt.Sprintf("%s/eth/v1/validator/duties/proposer/%d", c.apiURL, epoch)
	c.logger.Printf("Fetching proposer duties from: %s", url)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		c.logger.Printf("Error creating request: %v", err)
		return nil, fmt.Errorf("creating request: %v", err)
	}

	httpReq.Header.Set("accept", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		c.logger.Printf("Error making request: %v", err)
		return nil, fmt.Errorf("making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.logger.Printf("Unexpected status code: %d", resp.StatusCode)

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading response body: %v", err)
		}

		bodyString := string(bodyBytes)
		if strings.Contains(bodyString, "Proposer duties were requested for a future epoch") {
			return nil, fmt.Errorf("proposer duties were requested for a future epoch")
		}

		return nil, fmt.Errorf("unexpected status code: %v, response: %s", resp.StatusCode, bodyString)
	}

	var dutiesResp ProposerDutiesResponse
	if err := json.NewDecoder(resp.Body).Decode(&dutiesResp); err != nil {
		c.logger.Printf("Error decoding response: %v", err)
		return nil, fmt.Errorf("decoding response: %v", err)
	}

	return &dutiesResp, nil
}

func (c *Client) GetCurrentEpoch(ctx context.Context) (uint64, error) {
	url := fmt.Sprintf("%s/eth/v1/beacon/headers/head", c.apiURL)
	resp, err := http.Get(url)
	if err != nil {
		return 0, fmt.Errorf("getting current head: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var headResponse struct {
		Data struct {
			Header struct {
				Message struct {
					Slot string `json:"slot"`
				} `json:"message"`
			} `json:"header"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&headResponse); err != nil {
		return 0, fmt.Errorf("decoding head response: %v", err)
	}

	slot, err := strconv.ParseUint(headResponse.Data.Header.Message.Slot, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing slot: %v", err)
	}

	epoch := slot / 32
	return epoch, nil
}

func PrintDuties(duties *ProposerDutiesResponse, changed bool) {
	// fmt.Println("==== Proposer Duties ====")

	if changed {
		fmt.Println("!!! DUTIES CHANGED FROM PREDICTION !!!")
	}

	// if duties == nil || len(duties.Data) == 0 {
	// 	fmt.Println("No proposer duties found")
	// 	return
	// }

	fmt.Println("Slot\t\tValidator Public Key")
	fmt.Println("----------------------------------")

	// for _, duty := range duties.Data {
	// 	fmt.Printf("%s\t%s\n", duty.Slot, duty.Pubkey)
	// }

	if len(duties.Data) > 0 {
		fmt.Printf("%s\t%s\n", duties.Data[0].Slot, duties.Data[0].Pubkey)
	}

	fmt.Println("==== End of Duties ====")
}

func main() {
	client := NewClient(beaconAPIURL)
	cache := NewDutiesCache()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
		os.Exit(0)
	}()

	fmt.Println("Starting proposer duties monitor...")
	fmt.Printf("Querying %s every %s\n", beaconAPIURL, queryInterval)
	fmt.Println("Press Ctrl+C to exit")

	var lastEpoch uint64

	ticker := time.NewTicker(queryInterval)
	defer ticker.Stop()

	// Initial fetch
	fetchAndTrackDuties(ctx, client, cache, 0)

	for {
		select {
		case <-ticker.C:
			currentEpoch, err := client.GetCurrentEpoch(ctx)
			if err != nil {
				fmt.Printf("Error getting current epoch: %v\n", err)
				continue
			}

			if currentEpoch != lastEpoch {
				fmt.Printf("\nEpoch changed: %d -> %d\n", lastEpoch, currentEpoch)
				lastEpoch = currentEpoch
				fetchAndTrackDuties(ctx, client, cache, currentEpoch)
			} else {
				fmt.Printf("\nStill in epoch %d, using cached duties\n", currentEpoch)
				// Print cached duties without refetching
				// cachedDuties, _ := cache.Get(currentEpoch)
				// if cachedDuties != nil {
				// 	fmt.Println("==== Current Epoch Duties (cached) ====")
				// 	printCachedDuties(cachedDuties)
				// }

				// nextEpochDuties, exists := cache.Get(currentEpoch + 1)
				// if exists {
				// 	fmt.Println("==== Next Epoch Duties (cached) ====")
				// 	// printCachedDuties(nextEpochDuties)
				// }
			}

		case <-ctx.Done():
			return
		}
	}
}

func printCachedDuties(duties []ProposerDuty) {
	if len(duties) == 0 {
		fmt.Println("No proposer duties found")
		return
	}

	// fmt.Println("Slot\t\tValidator Public Key")
	// fmt.Println("----------------------------------")

	// for _, duty := range duties {
	// 	fmt.Printf("%s\t%s\n", duty.Slot, duty.Pubkey)
	// }

	// fmt.Println("==== End of Duties ====")
}

func fetchAndTrackDuties(ctx context.Context, client *Client, cache *DutiesCache, currentEpoch uint64) {
	// For first run
	if currentEpoch == 0 {
		var err error
		currentEpoch, err = client.GetCurrentEpoch(ctx)
		if err != nil {
			fmt.Printf("Error getting current epoch: %v\n", err)
			return
		}
	}

	fmt.Printf("\nFetching proposer duties for current epoch %d\n", currentEpoch)
	currentDuties, err := client.FetchProposerDuties(ctx, currentEpoch)
	if err != nil {
		fmt.Printf("Error fetching current epoch duties: %v\n", err)
	} else {
		// Check if duties changed from our prediction
		changed := cache.CompareAndUpdate(currentEpoch, currentDuties)
		if changed {
			fmt.Printf("!!! DUTIES CHANGED FOR EPOCH %d !!!\n", currentEpoch)
		}
		PrintDuties(currentDuties, changed)
	}

	nextEpoch := currentEpoch + 1
	fmt.Printf("\nFetching proposer duties for next epoch %d\n", nextEpoch)
	nextDuties, err := client.FetchProposerDuties(ctx, nextEpoch)
	if err != nil {
		if strings.Contains(err.Error(), "future epoch") {
			fmt.Printf("Next epoch duties not yet available: %v\n", err)
		} else {
			fmt.Printf("Error fetching next epoch duties: %v\n", err)
		}
	} else {
		cache.Store(nextEpoch, nextDuties)
		PrintDuties(nextDuties, false)
	}
}
