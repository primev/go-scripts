package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

func main() {
	ctx := context.Background()
	authToken, ok := os.LookupEnv("AUTH_TOKEN")
	if !ok || authToken == "" {
		log.Fatal("AUTH_TOKEN environment variable not found")
	}
	pointsUrl, ok := os.LookupEnv("POINTS_URL")
	if !ok || pointsUrl == "" {
		log.Fatal("POINTS_URL environment variable not found")
	}

	march1stBlock := uint64(21948292)

	infraSingularity := "0x53730f4088b116c807875eb67f71cbb1b065f530"
	for _, i := range []int{1, 2} {
		pubkey := getPlaceholderPubkey(i)
		entry := ManualEntry{
			PubKey:  pubkey,
			Adder:   infraSingularity,
			InBlock: march1stBlock,
		}
		resp, err := AddManualEntry(ctx, http.DefaultClient, pointsUrl, authToken, entry)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(string(resp))
	}

	bloxroute := "0x4d2793E5F9B477732F1b0c7199Bd8A4D866dA34B"
	for i := 3; i < 103; i++ {
		pubkey := getPlaceholderPubkey(i)
		entry := ManualEntry{
			PubKey:  pubkey,
			Adder:   bloxroute,
			InBlock: march1stBlock,
		}
		resp, err := AddManualEntry(ctx, http.DefaultClient, pointsUrl, authToken, entry)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(string(resp))
	}
}

func getPlaceholderPubkey(idx int) string {
	const prefix = "88889999"
	const total = 96
	rem := total - len(prefix)
	return fmt.Sprintf("0x%s%0*d", prefix, rem, idx)
}

type ManualEntry struct {
	PubKey  string `json:"pubkey"`
	Adder   string `json:"adder"`
	InBlock uint64 `json:"in_block"`
}

func AddManualEntry(
	ctx context.Context,
	client *http.Client,
	baseURL, bearerToken string,
	entry ManualEntry,
) ([]byte, error) {
	body, err := json.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/admin/add_manual_entry", baseURL),
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return respBody, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return respBody, nil
}
