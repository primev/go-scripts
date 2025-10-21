// query provider registry v2 events at 0xeb6d22309062a86fa194520344530874221ef48c

package main

import (
	"fmt"
	"log"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/primevprotocol/validator-registry/pkg/providerregistry"
)

func main() {

	client, err := ethclient.Dial("https://chainrpc.mev-commit.xyz/")
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	providerRegistryAddr := common.HexToAddress("0xeb6d22309062a86fa194520344530874221ef48c")
	providerRegistry, err := providerregistry.NewProviderregistryFilterer(providerRegistryAddr, client)
	if err != nil {
		log.Fatalf("Failed to create provider registry filterer: %v", err)
	}

	blsKeyAddedEvents, err := providerRegistry.FilterProviderRegistered(nil, nil)
	if err != nil {
		log.Fatalf("Failed to get bls key added events: %v", err)
	}

	count := 0

	for blsKeyAddedEvents.Next() {
		providerAddress := blsKeyAddedEvents.Event.Provider
		stakedAmount := blsKeyAddedEvents.Event.StakedAmount
		fmt.Println("Provider Registered")
		fmt.Println("provider address:", providerAddress)
		fmt.Println("staked amount:", stakedAmount)
		count++
	}

	fmt.Println("Total Provider Registered events:", count)

	// check bls key added events for this addr 0x570e531fB805B5eEbD5F29Eaa2766fBeB4977ddE
	providerAddr := common.HexToAddress("0x570e531fB805B5eEbD5F29Eaa2766fBeB4977ddE")
	blsKeyAddedEvents2, err2 := providerRegistry.FilterBLSKeyAdded(nil, []common.Address{providerAddr})
	if err2 != nil {
		log.Fatalf("Failed to get bls key added events: %v", err2)
	}

	count = 0
	for blsKeyAddedEvents2.Next() {
		fmt.Println("BLS Key Added")
		fmt.Println("provider address:", blsKeyAddedEvents2.Event.Provider)
		fmt.Println("bls public key:", common.Bytes2Hex(blsKeyAddedEvents2.Event.BlsPublicKey))
		count++
	}

	fmt.Println("Total BLS Key Added events:", count)

	// is provider valid
	caller, err := providerregistry.NewProviderregistryCaller(providerRegistryAddr, client)
	if err != nil {
		log.Fatalf("Failed to create provider registry caller: %v", err)
	}
	stakedAmount, err := caller.ProviderStakes(nil, providerAddr)
	if err != nil {
		log.Fatalf("Failed to check if provider is valid: %v", err)
	}
	fmt.Println("Staked Amount:", stakedAmount)

	// min stake
	minStake, err := caller.MinStake(nil)
	if err != nil {
		log.Fatalf("Failed to get min stake: %v", err)
	}
	fmt.Println("Min Stake:", minStake)
}
