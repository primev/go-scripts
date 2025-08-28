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
	flag.Parse()
	if *fileFlag == "" || *rpcFlag == "" {
		log.Fatal("both -file and -rpc are required. Usage: -file /absolute/path/to/keys.txt -rpc https://your.rpc")
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

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		log.Fatalf("Failed to get chain id: %v", err)
	}
	fmt.Println("Chain ID:", chainID)

	validatorOptInRouterAddress := common.HexToAddress("0x821798d7b9d57dF7Ed7616ef9111A616aB19ed64")
	routerCaller, err := validatoroptinrouter.NewValidatoroptinrouterCaller(validatorOptInRouterAddress, client)
	if err != nil {
		log.Fatalf("Failed to create Validator Opt In Router caller: %v", err)
	}

	for _, in := range inputs {
		pubkeyHex := strings.TrimPrefix(in, "0x")
		targetPubkey := common.Hex2Bytes(pubkeyHex)
		if len(targetPubkey) == 0 {
			fmt.Println("Skipping invalid pubkey:", in)
			continue
		}
		fmt.Println("targetPubkey:", common.Bytes2Hex(targetPubkey))

		areOptedIn, err := routerCaller.AreValidatorsOptedIn(nil, [][]byte{targetPubkey})
		if err != nil {
			fmt.Printf("Failed to check opted-in for %s: %v\n", in, err)
			continue
		}
		isOptedIn := areOptedIn[0]
		if !isOptedIn.IsAvsOptedIn && !isOptedIn.IsVanillaOptedIn && !isOptedIn.IsMiddlewareOptedIn {
			fmt.Println("Validator is not opted in")
			continue
		}

		if isOptedIn.IsMiddlewareOptedIn {
			middlewareAddress, err := routerCaller.MevCommitMiddleware(nil)
			if err != nil {
				fmt.Printf("Failed to get mev commit middleware address for %s: %v\n", in, err)
				continue
			}
			middlewareCaller, err := mevcommitmiddleware.NewMevcommitmiddlewareCaller(middlewareAddress, client)
			if err != nil {
				fmt.Printf("Failed to create mev commit middleware caller for %s: %v\n", in, err)
				continue
			}
			validatorRecord, err := middlewareCaller.ValidatorRecords(nil, targetPubkey)
			if err != nil {
				fmt.Printf("Failed to get validator record (middleware) for %s: %v\n", in, err)
				continue
			}
			fmt.Println("Operator:", validatorRecord.Operator)
			fmt.Println("Vault:", validatorRecord.Vault)
			fmt.Println("Dereg request occurrence:", validatorRecord.DeregRequestOccurrence)
			continue
		}

		if isOptedIn.IsVanillaOptedIn {
			vanillaRegistryAddress, err := routerCaller.VanillaRegistry(nil)
			if err != nil {
				fmt.Printf("Failed to get vanilla registry address for %s: %v\n", in, err)
				continue
			}
			vanillaRegistryCaller, err := vanillaregistry.NewVanillaregistryCaller(vanillaRegistryAddress, client)
			if err != nil {
				fmt.Printf("Failed to create vanilla registry caller for %s: %v\n", in, err)
				continue
			}
			stakedValidator, err := vanillaRegistryCaller.StakedValidators(nil, targetPubkey)
			if err != nil {
				fmt.Printf("Failed to get validator record (vanilla) for %s: %v\n", in, err)
				continue
			}
			fmt.Println("Exists:", stakedValidator.Exists)
			fmt.Println("Withdrawal address:", stakedValidator.WithdrawalAddress)
			fmt.Println("Balance:", stakedValidator.Balance)
			fmt.Println("Unstake occurrence:", stakedValidator.UnstakeOccurrence)
			continue
		}

		if isOptedIn.IsAvsOptedIn {
			avsRegistryAddress, err := routerCaller.MevCommitAVS(nil)
			if err != nil {
				fmt.Printf("Failed to get avs registry address for %s: %v\n", in, err)
				continue
			}
			avsRegistryCaller, err := mevcommitavs.NewMevcommitavsCaller(avsRegistryAddress, client)
			if err != nil {
				fmt.Printf("Failed to create avs registry caller for %s: %v\n", in, err)
				continue
			}
			validatorRecord, err := avsRegistryCaller.ValidatorRegistrations(nil, targetPubkey)
			if err != nil {
				fmt.Printf("Failed to get validator record (avs) for %s: %v\n", in, err)
				continue
			}
			fmt.Println("Validator registration:", validatorRecord)
			fmt.Println("Pod owner:", validatorRecord.PodOwner)
			fmt.Println("Freeze occurrence:", validatorRecord.FreezeOccurrence)
			fmt.Println("Dereg request occurrence:", validatorRecord.DeregRequestOccurrence)
			continue
		}
	}
}
