package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ethereum/go-ethereum/common"
	utils "github.com/primevprotocol/validator-registry/pkg/utils"
	vr "github.com/primevprotocol/validator-registry/pkg/validatorregistry"
)

func main() {

	client := utils.InitClient()

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		log.Fatalf("Failed to get chain id: %v", err)
	}
	fmt.Println("Chain ID: ", chainID)

	contractAddress := common.HexToAddress("0xF263483500e849Bd8d452c9A0F075B606ee64087") // Accurate as of 4/24/2024

	vrc, err := vr.NewValidatorregistryCaller(contractAddress, client)
	if err != nil {
		log.Fatalf("Failed to create Validator Registry caller: %v", err)
	}

	fmt.Println("-------------------")
	fmt.Println("Querying full set of validators BLS pubkeys staked with the registry contract...")
	fmt.Println("-------------------")

	startBeforeLen := time.Now()

	numStakedVals, valsetVersion, err := vrc.GetNumberOfStakedValidators(nil)
	if err != nil {
		log.Fatalf("Failed to get number of staked validators: %v", err)
	}
	fmt.Println("Number of staked validators: ", numStakedVals)

	elapsedToObtainLen := time.Since(startBeforeLen)

	start := time.Now()

	aggregatedValset := utils.GetStakedValidators(vrc, numStakedVals, valsetVersion)
	fmt.Println("Aggregated validator set length: ", len(aggregatedValset))

	startIndex := len(aggregatedValset) - 10
	if startIndex < 0 {
		startIndex = 0
	}
	lastVals := aggregatedValset[startIndex:]
	fmt.Print("Up to last 10 of staked validator BLS pubkeys: \n[\n")
	for _, v := range lastVals {
		fmt.Print(common.Bytes2Hex(v))
		fmt.Print(",\n")
	}
	fmt.Print("]\n")

	elapsed := time.Since(start)
	fmt.Printf("Time to query number of staked validators: %s\n", elapsedToObtainLen)
	fmt.Printf("Time to query all staked validator BLS pubkeys: %s\n", elapsed)
	fmt.Println("The above performance can be improved utilizing https://geth.ethereum.org/docs/interacting-with-geth/rpc/batch")
}
