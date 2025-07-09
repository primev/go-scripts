package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/primevprotocol/validator-registry/pkg/mevcommitavs"
	"github.com/primevprotocol/validator-registry/pkg/mevcommitmiddleware"
	"github.com/primevprotocol/validator-registry/pkg/validatoroptinrouter"
	"github.com/primevprotocol/validator-registry/pkg/vanillaregistry"
)

func main() {
	pubkeyFlag := flag.String("pubkey", "", "BLS public key to search for (hex string, with or without 0x prefix)")
	flag.Parse()
	if *pubkeyFlag == "" {
		log.Fatal("pubkey flag is required. Usage: -pubkey <hex_string>")
	}
	pubkeyHex := strings.TrimPrefix(*pubkeyFlag, "0x")
	targetPubkey := common.Hex2Bytes(pubkeyHex)
	if len(targetPubkey) == 0 {
		log.Fatal("Invalid pubkey format. Please provide a valid hex string.")
	}
	fmt.Println("targetPubkey:", common.Bytes2Hex(targetPubkey))

	client, err := ethclient.Dial("https://eth-mainnet.g.alchemy.com/v2/bz3jQOhNxXPWXUjqEfl1T4NwjR6pj72A")
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		log.Fatalf("Failed to get chain id: %v", err)
	}
	fmt.Println("Chain ID: ", chainID)

	validatorOptInRouterAddress := common.HexToAddress("0x821798d7b9d57dF7Ed7616ef9111A616aB19ed64")

	routerCaller, err := validatoroptinrouter.NewValidatoroptinrouterCaller(validatorOptInRouterAddress, client)
	if err != nil {
		log.Fatalf("Failed to create Validator Opt In Router caller: %v", err)
	}

	areOptedIn, err := routerCaller.AreValidatorsOptedIn(nil, [][]byte{targetPubkey})
	if err != nil {
		log.Fatalf("Failed to check if validator is opted in: %v", err)
	}
	isOptedIn := areOptedIn[0]
	if !isOptedIn.IsAvsOptedIn && !isOptedIn.IsVanillaOptedIn && !isOptedIn.IsMiddlewareOptedIn {
		fmt.Println("Validator is not opted in")
		return
	}

	if isOptedIn.IsMiddlewareOptedIn {
		middlewareAddress, err := routerCaller.MevCommitMiddleware(nil)
		if err != nil {
			log.Fatalf("Failed to get mev commit middleware address: %v", err)
		}
		middlewareCaller, err := mevcommitmiddleware.NewMevcommitmiddlewareCaller(middlewareAddress, client)
		if err != nil {
			log.Fatalf("Failed to create mev commit middleware caller: %v", err)
		}
		validatorRecord, err := middlewareCaller.ValidatorRecords(nil, targetPubkey)
		if err != nil {
			log.Fatalf("Failed to get validator record: %v", err)
		}
		fmt.Println("Operator: ", validatorRecord.Operator)
		fmt.Println("Vault: ", validatorRecord.Vault)
		fmt.Println("Dereg request occurrence: ", validatorRecord.DeregRequestOccurrence)
		return
	}
	if isOptedIn.IsVanillaOptedIn {
		vanillaRegistryAddress, err := routerCaller.VanillaRegistry(nil)
		if err != nil {
			log.Fatalf("Failed to get vanilla registry address: %v", err)
		}
		vanillaRegistryCaller, err := vanillaregistry.NewVanillaregistryCaller(vanillaRegistryAddress, client)
		if err != nil {
			log.Fatalf("Failed to create vanilla registry caller: %v", err)
		}
		stakedValidator, err := vanillaRegistryCaller.StakedValidators(nil, targetPubkey)
		if err != nil {
			log.Fatalf("Failed to get validator record: %v", err)
		}
		fmt.Println("Exists: ", stakedValidator.Exists)
		fmt.Println("Withdrawal address: ", stakedValidator.WithdrawalAddress)
		fmt.Println("Balance: ", stakedValidator.Balance)
		fmt.Println("Unstake occurrence: ", stakedValidator.UnstakeOccurrence)
		return
	}
	if isOptedIn.IsAvsOptedIn {
		avsRegistryAddress, err := routerCaller.MevCommitAVS(nil)
		if err != nil {
			log.Fatalf("Failed to get avs registry address: %v", err)
		}
		avsRegistryCaller, err := mevcommitavs.NewMevcommitavsCaller(avsRegistryAddress, client)
		if err != nil {
			log.Fatalf("Failed to create avs registry caller: %v", err)
		}
		validatorRecord, err := avsRegistryCaller.ValidatorRegistrations(nil, targetPubkey)
		if err != nil {
			log.Fatalf("Failed to get validator record: %v", err)
		}
		fmt.Println("Validator registration: ", validatorRecord)
		fmt.Println("Pod owner: ", validatorRecord.PodOwner)
		fmt.Println("Freeze occurrence: ", validatorRecord.FreezeOccurrence)
		fmt.Println("Dereg request occurrence: ", validatorRecord.DeregRequestOccurrence)
		return
	}
}
