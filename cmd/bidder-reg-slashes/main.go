package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	oracle "github.com/primev/mev-commit/contracts-abi/clients/Oracle"
	providerregistry "github.com/primev/mev-commit/contracts-abi/clients/ProviderRegistry"
)

func main() {

	client, err := ethclient.Dial("https://chainrpc.mev-commit.xyz/")
	if err != nil {
		log.Fatalf("Failed to connect to the mev-commit chain client: %v", err)
	}

	providerRegistryAddr := common.HexToAddress("0xb772Add4718E5BD6Fe57Fb486A6f7f008E52167E")
	providerRegistry, err := providerregistry.NewProviderregistryFilterer(providerRegistryAddr, client)
	if err != nil {
		log.Fatalf("Failed to create bidderregistry: %v", err)
	}

	oracleContractAddr := common.HexToAddress("0xa1aaCA1e4583dB498D47f3D5901f2B2EB49Bd8f6")
	oracleContract, err := oracle.NewOracleFilterer(oracleContractAddr, client)
	if err != nil {
		log.Fatalf("Failed to create bidderregistry: %v", err)
	}

	startBlock := uint64(0)

	block, err := client.BlockByNumber(context.Background(), nil)
	if err != nil {
		log.Fatalf("Failed to get current block: %v", err)
	}

	fmt.Println("Current block: ", block.Number().Uint64())
	endBlock := block.Number().Uint64()

	providerInQuestion := common.HexToAddress("0xE3d71EF44D20917b93AA93e12Bd35b0859824A8F") // BTCS

	opts := &bind.FilterOpts{
		Start: startBlock,
		End:   &endBlock,
	}
	iter, err := providerRegistry.FilterFundsSlashed(opts, []common.Address{providerInQuestion})
	if err != nil {
		log.Fatalf("Failed to get funds rewarded: %v", err)
	}

	events := []providerregistry.ProviderregistryFundsSlashed{}
	for iter.Next() {
		events = append(events, *iter.Event)
	}

	var mevCommitChainBlock uint64

	for _, event := range events {
		fmt.Println("Slash for provider: ", event.Provider)
		fmt.Println("Amount: ", event.Amount)
		fmt.Println("Mev-commit chain block: ", event.Raw.BlockNumber)
		mevCommitChainBlock = event.Raw.BlockNumber
	}

	eventsOracle := []oracle.OracleCommitmentProcessed{}
	optsOracle := &bind.FilterOpts{
		Start: mevCommitChainBlock,
		End:   &endBlock,
	}
	iterOracle, err := oracleContract.FilterCommitmentProcessed(optsOracle, nil)
	if err != nil {
		log.Fatalf("Failed to get commitment processed: %v", err)
	}

	for iterOracle.Next() {
		eventsOracle = append(eventsOracle, *iterOracle.Event)
	}

	for _, event := range eventsOracle {
		if event.IsSlash && event.Raw.BlockNumber == mevCommitChainBlock {
			fmt.Println("Commitment index processed for provider: ", common.Bytes2Hex(event.CommitmentIndex[:]))
			fmt.Println("Is slash: ", event.IsSlash)
		}
	}
}
