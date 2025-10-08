package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/primevprotocol/validator-registry/pkg/mevcommitavs"
	"github.com/primevprotocol/validator-registry/pkg/mevcommitmiddleware"
	"github.com/primevprotocol/validator-registry/pkg/validatoroptinrouter"
	"github.com/primevprotocol/validator-registry/pkg/vanillaregistry"
)

var hexRe = regexp.MustCompile(`(?i)0x[0-9a-f]+|[0-9a-f]{64,}`)

func main() {
	fileFlag := flag.String("file", "", "Path to a txt file containing BLS pubkeys (one per line or embedded like \"hex\"), 0x prefix optional")
	rpcFlag := flag.String("rpc", "", "Ethereum RPC URL (e.g., https://..., http://..., or ws://...)")
	delayMs := flag.Int("delay", 0, "Delay in milliseconds between RPC calls (optional)")
	onlyOperator := flag.Bool("operator", false, "If set, print 'pubkey -> [middleware:OP, vanilla:WithdrawalAddress, avs:PodOwner]' for any opted-in registry")
	flag.Parse()

	if *fileFlag == "" || *rpcFlag == "" {
		log.Fatal("both -file and -rpc are required. Usage: -file /absolute/path/to/keys.txt -rpc https://your.rpc [-delay 250] [-operator]")
	}

	f, err := os.Open(*fileFlag)
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer f.Close()

	var inputs []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		hex := hexRe.FindString(line)
		if hex == "" {
			continue
		}
		inputs = append(inputs, hex)
	}
	if err := sc.Err(); err != nil {
		log.Fatalf("Failed to read file: %v", err)
	}
	if len(inputs) == 0 {
		log.Fatal("No valid hex pubkeys found in file")
	}

	client, err := ethclient.Dial(*rpcFlag)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	// ---- RPC spacing ----
	var lastRPC time.Time
	var delay = time.Duration(*delayMs) * time.Millisecond
	waitRPC := func() {
		if delay <= 0 {
			return
		}
		if !lastRPC.IsZero() {
			if elapsed := time.Since(lastRPC); elapsed < delay {
				time.Sleep(delay - elapsed)
			}
		}
	}
	markRPC := func() { lastRPC = time.Now() }
	// ---------------------

	// Chain ID
	waitRPC()
	chainID, err := client.ChainID(context.Background())
	markRPC()
	if err != nil {
		log.Fatalf("Failed to get chain id: %v", err)
	}
	if !*onlyOperator {
		fmt.Println("Chain ID:", chainID)
	}

	// Router
	validatorOptInRouterAddress := common.HexToAddress("0x821798d7b9d57dF7Ed7616ef9111A616aB19ed64")
	routerCaller, err := validatoroptinrouter.NewValidatoroptinrouterCaller(validatorOptInRouterAddress, client)
	if err != nil {
		log.Fatalf("Failed to create Validator Opt In Router caller: %v", err)
	}

	// Prefetch sub-registries once
	waitRPC()
	middlewareAddress, mwErr := routerCaller.MevCommitMiddleware(nil)
	markRPC()
	var middlewareCaller *mevcommitmiddleware.MevcommitmiddlewareCaller
	if mwErr == nil {
		if c, err := mevcommitmiddleware.NewMevcommitmiddlewareCaller(middlewareAddress, client); err == nil {
			middlewareCaller = c
		}
	}

	waitRPC()
	vanillaAddress, vErr := routerCaller.VanillaRegistry(nil)
	markRPC()
	var vanillaCaller *vanillaregistry.VanillaregistryCaller
	if vErr == nil {
		if c, err := vanillaregistry.NewVanillaregistryCaller(vanillaAddress, client); err == nil {
			vanillaCaller = c
		}
	}

	waitRPC()
	avsAddress, aErr := routerCaller.MevCommitAVS(nil)
	markRPC()
	var avsCaller *mevcommitavs.MevcommitavsCaller
	if aErr == nil {
		if c, err := mevcommitavs.NewMevcommitavsCaller(avsAddress, client); err == nil {
			avsCaller = c
		}
	}

	for _, in := range inputs {
		pubkeyHex := strings.TrimPrefix(in, "0x")
		targetPubkey := common.Hex2Bytes(pubkeyHex)
		if len(targetPubkey) == 0 {
			if *onlyOperator {
				fmt.Printf("pubkey: %s -> N/A (invalid pubkey)\n", in)
			} else {
				fmt.Println("Skipping invalid pubkey:", in)
			}
			continue
		}
		pubkeyHexNo0x := common.Bytes2Hex(targetPubkey)

		// --- operator-only mode: check all opted-in registries and print their operator-equivalents ---
		if *onlyOperator {
			waitRPC()
			areOptedIn, err := routerCaller.AreValidatorsOptedIn(nil, [][]byte{targetPubkey})
			markRPC()
			if err != nil || len(areOptedIn) == 0 {
				fmt.Printf("pubkey: 0x%s -> N/A (router check error)\n", pubkeyHexNo0x)
				continue
			}
			opt := areOptedIn[0]

			var parts []string

			// Middleware -> Operator
			if opt.IsMiddlewareOptedIn && middlewareCaller != nil {
				waitRPC()
				rec, err := middlewareCaller.ValidatorRecords(nil, targetPubkey)
				markRPC()
				if err == nil && rec.Operator != (common.Address{}) {
					parts = append(parts, fmt.Sprintf("middleware:%s", rec.Operator.String()))
				}
			}
			// Vanilla -> WithdrawalAddress
			if opt.IsVanillaOptedIn && vanillaCaller != nil {
				waitRPC()
				vrec, err := vanillaCaller.StakedValidators(nil, targetPubkey)
				markRPC()
				// if Exists is true and WithdrawalAddress is nonzero, report it
				if err == nil && vrec.Exists && vrec.WithdrawalAddress != (common.Address{}) {
					parts = append(parts, fmt.Sprintf("vanilla:%s", vrec.WithdrawalAddress.String()))
				}
			}
			// AVS -> PodOwner
			if opt.IsAvsOptedIn && avsCaller != nil {
				waitRPC()
				areg, err := avsCaller.ValidatorRegistrations(nil, targetPubkey)
				markRPC()
				if err == nil && areg.PodOwner != (common.Address{}) {
					parts = append(parts, fmt.Sprintf("avs:%s", areg.PodOwner.String()))
				}
			}
			if len(parts) == 0 {
				fmt.Printf("pubkey: 0x%s -> N/A\n", pubkeyHexNo0x)
			} else {
				// Single line, clearly labeled per source
				fmt.Printf("pubkey: 0x%s -> %s\n", pubkeyHexNo0x, strings.Join(parts, ", "))
			}
			continue
		}

		// --- verbose mode (unchanged except prefetching callers) ---
		fmt.Println("targetPubkey:", pubkeyHexNo0x)

		waitRPC()
		areOptedIn, err := routerCaller.AreValidatorsOptedIn(nil, [][]byte{targetPubkey})
		markRPC()
		if err != nil {
			fmt.Printf("Failed to check opted-in for 0x%s: %v\n", pubkeyHexNo0x, err)
			continue
		}
		opt := areOptedIn[0]
		if !opt.IsAvsOptedIn && !opt.IsVanillaOptedIn && !opt.IsMiddlewareOptedIn {
			fmt.Println("Validator is not opted in")
			continue
		}

		if opt.IsMiddlewareOptedIn && middlewareCaller != nil {
			waitRPC()
			mrec, err := middlewareCaller.ValidatorRecords(nil, targetPubkey)
			markRPC()
			if err != nil {
				fmt.Printf("Failed to get validator record (middleware) for 0x%s: %v\n", pubkeyHexNo0x, err)
			} else {
				fmt.Println("Operator:", mrec.Operator)
				fmt.Println("Vault:", mrec.Vault)
				fmt.Println("Dereg request occurrence:", mrec.DeregRequestOccurrence)
			}
			// continue: you had continues per-branch in your original
			continue
		}

		if opt.IsVanillaOptedIn && vanillaCaller != nil {
			waitRPC()
			vrec, err := vanillaCaller.StakedValidators(nil, targetPubkey)
			markRPC()
			if err != nil {
				fmt.Printf("Failed to get validator record (vanilla) for 0x%s: %v\n", pubkeyHexNo0x, err)
				continue
			}
			fmt.Println("Exists:", vrec.Exists)
			fmt.Println("Withdrawal address:", vrec.WithdrawalAddress)
			fmt.Println("Balance:", vrec.Balance)
			fmt.Println("Unstake occurrence:", vrec.UnstakeOccurrence)
			continue
		}

		if opt.IsAvsOptedIn && avsCaller != nil {
			waitRPC()
			areg, err := avsCaller.ValidatorRegistrations(nil, targetPubkey)
			markRPC()
			if err != nil {
				fmt.Printf("Failed to get validator record (avs) for 0x%s: %v\n", pubkeyHexNo0x, err)
				continue
			}
			fmt.Println("Validator registration:", areg)
			fmt.Println("Pod owner:", areg.PodOwner)
			fmt.Println("Freeze occurrence:", areg.FreezeOccurrence)
			fmt.Println("Dereg request occurrence:", areg.DeregRequestOccurrence)
			continue
		}
	}
}
