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

	bloxrouteOld := "0x4d2793E5F9B477732F1b0c7199Bd8A4D866dA34B"
	bloxrouteNew := "0xc84912EC313C27FA0c93747442048326aFBE76Bc"
	for i := 3; i < 103; i++ {
		pubkey := getPlaceholderPubkey(i)
		entry := UpdateEntry{
			PubKey:  pubkey,
			OldAddr: bloxrouteOld,
			NewAddr: bloxrouteNew,
		}
		resp, err := UpdateAddr(ctx, http.DefaultClient, pointsUrl, authToken, entry)
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

type UpdateEntry struct {
	PubKey  string `json:"pubkey"`
	OldAddr string `json:"old_addr"`
	NewAddr string `json:"new_addr"`
}

func UpdateAddr(
	ctx context.Context,
	client *http.Client,
	baseURL, bearerToken string,
	entry UpdateEntry,
) ([]byte, error) {
	body, err := json.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		fmt.Sprintf("%s/admin/update_addr", baseURL),
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
