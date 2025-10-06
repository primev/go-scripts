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

	blsKeyAddedEvents, err := providerRegistry.FilterBLSKeyAdded(nil, nil)
	if err != nil {
		log.Fatalf("Failed to get bls key added events: %v", err)
	}

	count := 0

	for blsKeyAddedEvents.Next() {
		providerAddress := blsKeyAddedEvents.Event.Provider
		blsPublicKey := blsKeyAddedEvents.Event.BlsPublicKey
		fmt.Println("BLS Key Added")
		fmt.Println("provider address:", providerAddress)
		fmt.Println("bls public key:", common.Bytes2Hex(blsPublicKey))
		count++
	}

	fmt.Println("Total BLS Key Added events:", count)

	blsKeyRemovedEvents, err := providerRegistry.FilterBLSKeyRemoved(nil, nil)
	if err != nil {
		log.Fatalf("Failed to get bls key removed events: %v", err)
	}

	count = 0
	for blsKeyRemovedEvents.Next() {
		providerAddress := blsKeyRemovedEvents.Event.Provider
		blsPublicKey := blsKeyRemovedEvents.Event.BlsPublicKey
		fmt.Println("BLS Key Removed")
		fmt.Println("provider address:", providerAddress)
		fmt.Println("bls public key:", common.Bytes2Hex(blsPublicKey))
		count++
	}

	fmt.Println("Total BLS Key Removed events:", count)

	providerRegistryCaller, err := providerregistry.NewProviderregistryCaller(providerRegistryAddr, client)
	if err != nil {
		log.Fatalf("Failed to create provider registry caller: %v", err)
	}

	providerAddress := common.HexToAddress("0xB3998135372F1eE16Cb510af70ed212b5155Af62")
	blsKeys, err := providerRegistryCaller.GetBLSKeys(nil, providerAddress)
	if err != nil {
		log.Fatalf("Failed to get bls keys for provider: %v", err)
	}

	for _, blsKey := range blsKeys {
		fmt.Println("BLS Key:", common.Bytes2Hex(blsKey))
	}
}
