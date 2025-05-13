package main

import (
	"context"
	"fmt"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/primevprotocol/validator-registry/pkg/bidderregistry"
	"github.com/primevprotocol/validator-registry/pkg/preconfmanager"
)

const (
	PRECISION = 1e16
)

var (
	BigOneHundredPercent = big.NewInt(100 * PRECISION)
)

func main() {

	client, err := ethclient.Dial("https://chainrpc.mev-commit.xyz/")
	if err != nil {
		log.Fatalf("Failed to connect to the mev-commit chain client: %v", err)
	}

	preconfManagerAddr := common.HexToAddress("0x3761bF3932cD22d684A7485002E1424c3aCCD69c")
	preconfManager, err := preconfmanager.NewPreconfmanagerFilterer(preconfManagerAddr, client)
	if err != nil {
		log.Fatalf("Failed to create preconfmanager: %v", err)
	}

	bidderRegistryAddr := common.HexToAddress("0xC973D09e51A20C9Ab0214c439e4B34Dbac52AD67")
	bidderRegistry, err := bidderregistry.NewBidderregistryFilterer(bidderRegistryAddr, client)
	if err != nil {
		log.Fatalf("Failed to create bidderregistry: %v", err)
	}

	block, err := client.BlockByNumber(context.Background(), nil)
	if err != nil {
		log.Fatalf("Failed to get current block: %v", err)
	}

	endBlock := block.Number().Uint64()
	opts := &bind.FilterOpts{
		Start: 0,
		End:   &endBlock,
	}
	iter, err := preconfManager.FilterOpenedCommitmentStored(opts, nil)
	if err != nil {
		log.Fatalf("Failed to get opened commitment stored: %v", err)
	}

	providerInQuestion := common.HexToAddress("0xE3d71EF44D20917b93AA93e12Bd35b0859824A8F")

	totalBidAmt := big.NewInt(0)
	totalDecayedBidAmtFixed := big.NewInt(0)
	totalDecayedBidAmtWithBug := big.NewInt(0)
	for iter.Next() {
		commitment := iter.Event
		if commitment.Committer == providerInQuestion {
			totalBidAmt.Add(totalBidAmt, commitment.BidAmt)
			decayPercentageFixed := computeResidualAfterDecay(
				commitment.DecayStartTimeStamp,
				commitment.DecayEndTimeStamp,
				commitment.DispatchTimestamp,
				true,
			)
			decayPercentageWithBug := computeResidualAfterDecay(
				commitment.DecayStartTimeStamp,
				commitment.DecayEndTimeStamp,
				commitment.DispatchTimestamp,
				false,
			)
			decayedBidAmtFixed := new(big.Int).Mul(commitment.BidAmt, decayPercentageFixed)
			decayedBidAmtWithBug := new(big.Int).Mul(commitment.BidAmt, decayPercentageWithBug)
			decayedBidAmtFixed = new(big.Int).Div(decayedBidAmtFixed, BigOneHundredPercent)
			decayedBidAmtWithBug = new(big.Int).Div(decayedBidAmtWithBug, BigOneHundredPercent)
			totalDecayedBidAmtFixed.Add(totalDecayedBidAmtFixed, decayedBidAmtFixed)
			totalDecayedBidAmtWithBug.Add(totalDecayedBidAmtWithBug, decayedBidAmtWithBug)
		}
	}
	fmt.Println("Total bid amount: ", totalBidAmt)
	fmt.Println("Total decayed bid amount (decay logic being post PR #673): ", totalDecayedBidAmtFixed)
	fmt.Println("Total decayed bid amount (decay logic being pre PR #673): ", totalDecayedBidAmtWithBug)

	iter2, err := bidderRegistry.FilterFundsRewarded(opts, nil, nil, []common.Address{providerInQuestion})
	if err != nil {
		log.Fatalf("Failed to get funds rewarded: %v", err)
	}

	totatlFundsRewarded := big.NewInt(0)
	for iter2.Next() {
		reward := iter2.Event
		totatlFundsRewarded.Add(totatlFundsRewarded, reward.Amount)
	}
	fmt.Println("Total funds actually rewarded: ", totatlFundsRewarded)
}

// Copied from https://github.com/primev/mev-commit/blob/main/oracle/pkg/updater/updater.go
func computeResidualAfterDecay(startTimestamp, endTimestamp, commitTimestamp uint64, fixedLogic bool) *big.Int {
	if startTimestamp >= endTimestamp || endTimestamp <= commitTimestamp {
		log.Fatalf("timestamp out of range: %v, %v, %v", startTimestamp, endTimestamp, commitTimestamp)
		return big.NewInt(0)
	}
	if startTimestamp > commitTimestamp {
		if fixedLogic {
			return BigOneHundredPercent
		}
		return big.NewInt(0)
	}
	totalTime := new(big.Int).SetUint64(endTimestamp - startTimestamp)
	timePassed := new(big.Int).SetUint64(commitTimestamp - startTimestamp)
	timeRemaining := new(big.Int).Sub(totalTime, timePassed)
	scaledRemaining := new(big.Int).Mul(timeRemaining, BigOneHundredPercent)
	residualPercentage := new(big.Int).Div(scaledRemaining, totalTime)
	if residualPercentage.Cmp(BigOneHundredPercent) > 0 {
		residualPercentage = BigOneHundredPercent
	}
	return residualPercentage
}
