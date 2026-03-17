package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/primevprotocol/validator-registry/pkg/mevcommitavs"
	"github.com/primevprotocol/validator-registry/pkg/mevcommitmiddleware"
	"github.com/primevprotocol/validator-registry/pkg/validatoroptinrouter"
	"github.com/primevprotocol/validator-registry/pkg/vanillaregistry"
	"github.com/xuri/excelize/v2"
)

type optedInValidator struct {
	pubKey         []byte
	optInType      string
	optInBlock     uint64
	podOwner       common.Address
	vault          common.Address
	operator       common.Address
	withdrawalAddr common.Address
}

func main() {
	rpcURL := flag.String("rpc-url", "https://ethereum-rpc.publicnode.com", "Ethereum RPC URL")
	beaconAPIKey := flag.String("beacon-api-key", "", "beaconcha.in API key")
	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	client, err := ethclient.Dial(*rpcURL)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		log.Fatalf("Failed to get chain id: %v", err)
	}
	if chainID.Cmp(big.NewInt(1)) != 0 {
		log.Fatalf("Chain ID is not mainnet: %v", chainID)
	}

	mevCommitAVSAddress := common.HexToAddress("0xBc77233855e3274E1903771675Eb71E602D9DC2e")
	avsFilterer, err := mevcommitavs.NewMevcommitavsFilterer(mevCommitAVSAddress, client)
	if err != nil {
		log.Fatalf("Failed to create AVS filterer: %v", err)
	}

	mevCommitMiddlewareAddress := common.HexToAddress("0x21fD239311B050bbeE7F32850d99ADc224761382")
	middlewareFilterer, err := mevcommitmiddleware.NewMevcommitmiddlewareFilterer(mevCommitMiddlewareAddress, client)
	if err != nil {
		log.Fatalf("Failed to create middleware filterer: %v", err)
	}

	vanillaRegistryAddress := common.HexToAddress("0x47afdcB2B089C16CEe354811EA1Bbe0DB7c335E9")
	vanillaFilterer, err := vanillaregistry.NewVanillaregistryFilterer(vanillaRegistryAddress, client)
	if err != nil {
		log.Fatalf("Failed to create vanilla registry filterer: %v", err)
	}

	validatorOptInRouterAddress := common.HexToAddress("0x821798d7b9d57dF7Ed7616ef9111A616aB19ed64")
	routerCaller, err := validatoroptinrouter.NewValidatoroptinrouterCaller(validatorOptInRouterAddress, client)
	if err != nil {
		log.Fatalf("Failed to create router caller: %v", err)
	}

	latestBlock, err := client.BlockNumber(context.Background())
	if err != nil {
		log.Fatalf("Failed to get latest block number: %v", err)
	}

	batchSize := uint64(50000)
	startBlock := uint64(21162202) // deployment block

	optedInValidators := make([]optedInValidator, 0, 1000)

	for startBlock <= latestBlock {
		endBlock := startBlock + batchSize - 1
		if endBlock > latestBlock {
			endBlock = latestBlock
		}
		fmt.Printf("Processing blocks %d to %d\n", startBlock, endBlock)

		opts := &bind.FilterOpts{
			Start:   startBlock,
			End:     &endBlock,
			Context: context.Background(),
		}

		events, err := avsFilterer.FilterValidatorRegistered(opts, nil)
		if err != nil {
			log.Fatalf("Failed to filter AVS ValidatorRegistered events for blocks %d to %d: %v", startBlock, endBlock, err)
		}
		for events.Next() {
			optedInValidators = append(optedInValidators, optedInValidator{
				pubKey:     events.Event.ValidatorPubKey,
				optInType:  "Eigen",
				optInBlock: events.Event.Raw.BlockNumber,
				podOwner:   events.Event.PodOwner,
			})
		}

		middlewareEvents, err := middlewareFilterer.FilterValRecordAdded(opts, nil, nil, nil)
		if err != nil {
			log.Fatalf("Failed to filter middleware ValRecordAdded events for blocks %d to %d: %v", startBlock, endBlock, err)
		}
		for middlewareEvents.Next() {
			optedInValidators = append(optedInValidators, optedInValidator{
				pubKey:     middlewareEvents.Event.BlsPubkey,
				optInType:  "Symbiotic",
				optInBlock: middlewareEvents.Event.Raw.BlockNumber,
				vault:      middlewareEvents.Event.Vault,
				operator:   middlewareEvents.Event.Operator,
			})
		}

		vanillaEvents, err := vanillaFilterer.FilterStaked(opts, nil, nil)
		if err != nil {
			log.Fatalf("Failed to filter vanilla Staked events for blocks %d to %d: %v", startBlock, endBlock, err)
		}
		for vanillaEvents.Next() {
			optedInValidators = append(optedInValidators, optedInValidator{
				pubKey:         vanillaEvents.Event.ValBLSPubKey,
				optInType:      "Vanilla",
				optInBlock:     vanillaEvents.Event.Raw.BlockNumber,
				withdrawalAddr: vanillaEvents.Event.WithdrawalAddress,
			})
		}

		startBlock = endBlock + 1
	}

	allUncheckedIncludingDuplicates := optedInValidators
	allDeduped := dedupeValidators(optedInValidators)
	routerPassing := sanityCheckAgainstRouter(allDeduped, routerCaller)
	routerFailing := diffByPubKey(allDeduped, routerPassing)

	exportWorkbookXLSX(
		"validator_registry_report.xlsx",
		routerPassing,
		allDeduped,
		routerFailing,
		allUncheckedIncludingDuplicates,
		333*time.Millisecond,
		*beaconAPIKey,
	)
}

// helper
func dedupeValidators(in []optedInValidator) []optedInValidator {
	m := make(map[string]optedInValidator, len(in))

	for _, v := range in {
		key := hex.EncodeToString(v.pubKey)

		if exist, ok := m[key]; ok {
			// keep the LATEST opt-in block and overwrite fields from the newer row
			if v.optInBlock > exist.optInBlock {
				m[key] = v
				continue
			}
			// if same block height, prefer non-empty metadata from either
			if v.optInBlock == exist.optInBlock {
				if exist.optInType == "" && v.optInType != "" {
					exist.optInType = v.optInType
				}
				if (exist.podOwner == (common.Address{})) && (v.podOwner != (common.Address{})) {
					exist.podOwner = v.podOwner
				}
				if (exist.vault == (common.Address{})) && (v.vault != (common.Address{})) {
					exist.vault = v.vault
				}
				if (exist.operator == (common.Address{})) && (v.operator != (common.Address{})) {
					exist.operator = v.operator
				}
				if (exist.withdrawalAddr == (common.Address{})) && (v.withdrawalAddr != (common.Address{})) {
					exist.withdrawalAddr = v.withdrawalAddr
				}
				m[key] = exist
			}
		} else {
			m[key] = v
		}
	}

	out := make([]optedInValidator, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}

	// deterministic order: by optInBlock asc, then pubkey
	sort.Slice(out, func(i, j int) bool {
		if out[i].optInBlock == out[j].optInBlock {
			return bytes.Compare(out[i].pubKey, out[j].pubKey) < 0
		}
		return out[i].optInBlock < out[j].optInBlock
	})

	return out
}

func routerTypeString(isAvs, isMiddleware, isVanilla bool) string {
	parts := make([]string, 0, 3)
	if isAvs {
		parts = append(parts, "Eigen")
	}
	if isMiddleware {
		parts = append(parts, "Symbiotic")
	}
	if isVanilla {
		parts = append(parts, "Vanilla")
	}
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		return "Multiple: " + strings.Join(parts, ", ")
	}
}

func sanityCheckAgainstRouter(optedInValidators []optedInValidator, routerCaller *validatoroptinrouter.ValidatoroptinrouterCaller) []optedInValidator {
	batchSize := 50
	filtered := make([]optedInValidator, 0, len(optedInValidators))

	for i := 0; i < len(optedInValidators); i += batchSize {
		end := i + batchSize
		if end > len(optedInValidators) {
			end = len(optedInValidators)
		}
		fmt.Printf("Checking batch %d to %d against router\n", i, end)

		// prepare pubkeys batch
		batch := make([][]byte, 0, end-i)
		for _, validator := range optedInValidators[i:end] {
			batch = append(batch, validator.pubKey)
		}

		statuses, err := routerCaller.AreValidatorsOptedIn(nil, batch)
		if err != nil {
			log.Fatalf("Failed to check if validators are opted in: %v", err)
		}

		for idx := 0; idx < len(statuses); idx++ {
			s := statuses[idx]
			if s.IsAvsOptedIn || s.IsMiddlewareOptedIn || s.IsVanillaOptedIn {
				row := optedInValidators[i+idx]
				row.optInType = routerTypeString(s.IsAvsOptedIn, s.IsMiddlewareOptedIn, s.IsVanillaOptedIn)
				filtered = append(filtered, row)
			}
		}
	}

	return filtered
}

func diffByPubKey(minuend, subtrahend []optedInValidator) []optedInValidator {
	has := make(map[string]struct{}, len(subtrahend))
	for _, v := range subtrahend {
		has[hex.EncodeToString(v.pubKey)] = struct{}{}
	}
	out := make([]optedInValidator, 0)
	for _, v := range minuend {
		key := hex.EncodeToString(v.pubKey)
		if _, ok := has[key]; !ok {
			out = append(out, v)
		}
	}
	return out
}

func exportWorkbookXLSX(
	filename string,
	routerPassing []optedInValidator,
	uniqueRegistrations []optedInValidator,
	failed []optedInValidator,
	allIncludingDups []optedInValidator,
	requestDelay time.Duration,
	beaconAPIKey string,
) {
	if requestDelay <= 0 {
		requestDelay = 500 * time.Millisecond
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	makeSheet := func(name string, headers []string) string {
		if len(f.GetSheetList()) == 1 && f.GetSheetName(0) == "Sheet1" {
			f.SetSheetName("Sheet1", name)
		} else {
			if _, err := f.NewSheet(name); err != nil {
				log.Fatalf("Failed to create sheet %s: %v", name, err)
			}
		}
		for c, h := range headers {
			cell, _ := excelize.CoordinatesToCellName(c+1, 1)
			_ = f.SetCellValue(name, cell, h)
		}
		_ = f.SetRowHeight(name, 1, 18)
		return name
	}

	beaconSheet := makeSheet(
		"fully_validated_list",
		[]string{
			"pubKey",
			"optInBlock",
			"optInType",
			"podOwner",
			"vault",
			"operator",
			"withdrawalAddr",
			"beacon_status",
			"beacon_validatorindex",
			"last_attestation_slot",
			"last_attestation_time_utc",
			"is_exited_like",
		},
	)
	if idx, err := f.GetSheetIndex(beaconSheet); err == nil {
		f.SetActiveSheet(idx)
	} else {
		log.Fatalf("Failed to get sheet index for %s: %v", beaconSheet, err)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	beaconBatchSize := 100

	// Build lookup of pubkey hex -> beacon data
	beaconData := make(map[string]beaconValidatorData, len(routerPassing))

	for i := 0; i < len(routerPassing); i += beaconBatchSize {
		if i > 0 {
			time.Sleep(requestDelay)
		}
		end := i + beaconBatchSize
		if end > len(routerPassing) {
			end = len(routerPassing)
		}

		pubkeys := make([]string, 0, end-i)
		for _, v := range routerPassing[i:end] {
			pubkeys = append(pubkeys, hex.EncodeToString(v.pubKey))
		}

		fmt.Printf("  beacon batch %d-%d / %d\n", i+1, end, len(routerPassing))

		results, err := fetchBeaconchaInValidatorBatch(httpClient, pubkeys, beaconAPIKey)
		if err != nil {
			log.Printf("WARN: beacon batch fetch failed (%d-%d): %v", i+1, end, err)
			continue
		}
		for k, v := range results {
			beaconData[k] = v
		}
	}

	// Write rows using fetched beacon data
	for i, v := range routerPassing {
		rowNum := i + 2
		pubKeyHex := hex.EncodeToString(v.pubKey)

		beaconStatus := ""
		validatorIndex := ""
		lastSlotStr := ""
		lastTime := ""
		isExitedLike := ""

		if bd, ok := beaconData[pubKeyHex]; ok {
			beaconStatus = bd.Status
			validatorIndex = fmt.Sprintf("%d", bd.ValidatorIndex)
			lastSlotStr = fmt.Sprintf("%d", bd.LastAttestationSlot)
			if bd.LastAttestationSlot > 0 {
				lastTime = slotToTimeUTC(bd.LastAttestationSlot).Format(time.RFC3339)
			}
			s := strings.ToLower(beaconStatus)
			if strings.HasPrefix(s, "exited") || strings.Contains(s, "withdraw") {
				isExitedLike = "true"
			} else {
				isExitedLike = "false"
			}
		} else {
			beaconStatus = "NOT_FOUND"
		}

		values := []any{
			pubKeyHex,
			v.optInBlock,
			v.optInType,
			v.podOwner.Hex(),
			v.vault.Hex(),
			v.operator.Hex(),
			v.withdrawalAddr.Hex(),
			beaconStatus,
			validatorIndex,
			lastSlotStr,
			lastTime,
			isExitedLike,
		}

		for c, val := range values {
			cell, _ := excelize.CoordinatesToCellName(c+1, rowNum)
			_ = f.SetCellValue(beaconSheet, cell, val)
		}
	}

	writeOptInSheet(f, "unique_no_router_check", uniqueRegistrations)
	writeOptInSheet(f, "failing_router_check", failed)
	writeOptInSheet(f, "all_no_router_or_dedup_check", allIncludingDups)

	if err := f.SaveAs(filename); err != nil {
		log.Fatalf("Failed to save workbook %s: %v", filename, err)
	}
	fmt.Printf("Workbook export complete: %s\n", filename)
}

func writeOptInSheet(f *excelize.File, sheetName string, validators []optedInValidator) {
	if _, err := f.NewSheet(sheetName); err != nil {
		log.Fatalf("Failed to create sheet %s: %v", sheetName, err)
	}

	headers := []string{"pubKey", "optInBlock", "optInType", "podOwner", "vault", "operator", "withdrawalAddr"}
	for c, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(c+1, 1)
		_ = f.SetCellValue(sheetName, cell, h)
	}

	sort.Slice(validators, func(i, j int) bool {
		if validators[i].optInBlock == validators[j].optInBlock {
			return bytes.Compare(validators[i].pubKey, validators[j].pubKey) < 0
		}
		return validators[i].optInBlock < validators[j].optInBlock
	})

	for i, v := range validators {
		rowNum := i + 2
		values := []any{
			hex.EncodeToString(v.pubKey),
			v.optInBlock,
			v.optInType,
			v.podOwner.Hex(),
			v.vault.Hex(),
			v.operator.Hex(),
			v.withdrawalAddr.Hex(),
		}
		for c, val := range values {
			cell, _ := excelize.CoordinatesToCellName(c+1, rowNum)
			_ = f.SetCellValue(sheetName, cell, val)
		}
	}
}

// Ethereum mainnet genesis time (Beacon chain) in UTC: 2020-12-01 12:00:23 UTC
var ethMainnetGenesis = time.Date(2020, 12, 1, 12, 0, 23, 0, time.UTC)

const secondsPerSlot = 12

func slotToTimeUTC(slot uint64) time.Time {
	return ethMainnetGenesis.Add(time.Duration(slot*secondsPerSlot) * time.Second)
}

type beaconValidatorData struct {
	Status              string      `json:"status"`
	ValidatorIndex      uint64      `json:"validatorindex"`
	LastAttestationSlot uint64      `json:"lastattestationslot"`
	ExitEpoch           json.Number `json:"exitepoch"`
	Pubkey              string      `json:"pubkey"`
}

// fetchBeaconchaInValidatorBatch fetches up to 100 validators in a single request.
// Returns a map from pubkey hex (without 0x prefix) to the validator data.
func fetchBeaconchaInValidatorBatch(client *http.Client, pubkeyHexes []string, apiKey string) (map[string]beaconValidatorData, error) {
	prefixed := make([]string, len(pubkeyHexes))
	for i, pk := range pubkeyHexes {
		if !strings.HasPrefix(pk, "0x") {
			prefixed[i] = "0x" + pk
		} else {
			prefixed[i] = pk
		}
	}
	url := "https://beaconcha.in/api/v1/validator/" + strings.Join(prefixed, ",")
	if apiKey != "" {
		url += "?apikey=" + apiKey
	}

	const maxAttempts = 6
	baseDelay := 750 * time.Millisecond

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "primev-validator-registry-audit/1.0")

		resp, err := client.Do(req)
		if err != nil {
			if attempt == maxAttempts {
				return nil, err
			}
			sleepBackoff(baseDelay, attempt)
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode == 200 {
			result := make(map[string]beaconValidatorData, len(pubkeyHexes))

			if len(pubkeyHexes) == 1 {
				// single validator: response has Data as object
				var single struct {
					Data beaconValidatorData `json:"data"`
				}
				dec := json.NewDecoder(bytes.NewReader(body))
				dec.UseNumber()
				if err := dec.Decode(&single); err != nil {
					return nil, fmt.Errorf("failed to decode beaconcha.in JSON: %w (body=%s)", err, string(body))
				}
				key := strings.TrimPrefix(strings.ToLower(single.Data.Pubkey), "0x")
				if key == "" {
					key = pubkeyHexes[0]
				}
				result[key] = single.Data
			} else {
				// multiple validators: response has Data as array
				var multi struct {
					Data []beaconValidatorData `json:"data"`
				}
				dec := json.NewDecoder(bytes.NewReader(body))
				dec.UseNumber()
				if err := dec.Decode(&multi); err != nil {
					return nil, fmt.Errorf("failed to decode beaconcha.in JSON: %w (body=%s)", err, string(body))
				}
				for _, d := range multi.Data {
					key := strings.TrimPrefix(strings.ToLower(d.Pubkey), "0x")
					result[key] = d
				}
			}
			return result, nil
		}

		if resp.StatusCode == 429 || (resp.StatusCode >= 500 && resp.StatusCode <= 599) {
			if attempt == maxAttempts {
				return nil, fmt.Errorf("beaconcha.in HTTP %d after retries: %s", resp.StatusCode, string(body))
			}
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, convErr := strconv.Atoi(strings.TrimSpace(ra)); convErr == nil && secs > 0 {
					time.Sleep(time.Duration(secs) * time.Second)
					continue
				}
			}
			sleepBackoff(baseDelay, attempt)
			continue
		}

		return nil, fmt.Errorf("beaconcha.in HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil, fmt.Errorf("unreachable")
}

func sleepBackoff(base time.Duration, attempt int) {
	mult := 1 << (attempt - 1)
	jitter := 0.7 + rand.Float64()*0.6
	d := time.Duration(float64(base*time.Duration(mult)) * jitter)
	time.Sleep(d)
}
