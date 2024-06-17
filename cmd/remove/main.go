package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/primevprotocol/validator-registry/pkg/events"
	"github.com/primevprotocol/validator-registry/pkg/query"
	utils "github.com/primevprotocol/validator-registry/pkg/utils"
	vrv1 "github.com/primevprotocol/validator-registry/pkg/validatorregistryv1"
)

func extractPrivateKey(keystoreFile string, passphrase string) *ecdsa.PrivateKey {
	keyjson, err := os.ReadFile(keystoreFile)
	if err != nil {
		panic("failed to read keystore file")
	}

	key, err := keystore.DecryptKey(keyjson, passphrase)
	if err != nil {
		panic("failed to decrypt key")
	}

	return key.PrivateKey
}

func main() {
	// Now using owner keystore
	keystoreFile := os.Getenv("KEYSTORE_FILE")
	if keystoreFile == "" {
		fmt.Println("KEYSTORE_FILE env var not supplied")
		os.Exit(1)
	}
	passphrase := os.Getenv("PASSPHRASE")
	if passphrase == "" {
		fmt.Println("PASSPHRASE env var not supplied")
		os.Exit(1)
	}
	privateKey := extractPrivateKey(keystoreFile, passphrase)

	client, err := ethclient.Dial("https://ethereum-holesky-rpc.publicnode.com")
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		log.Fatalf("Failed to get chain id: %v", err)
	}
	fmt.Println("Chain ID: ", chainID)

	fromAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
	balance, err := client.BalanceAt(context.Background(), fromAddress, nil)
	if err != nil {
		log.Fatalf("Failed to get account balance: %v", err)
	}
	zeroPointTwoEth := big.NewInt(200000000000000000)
	if balance.Cmp(zeroPointTwoEth) == -1 {
		log.Fatalf("Insufficient balance. Please fund %v with at least 0.2 ETH", fromAddress.Hex())
	}

	contractAddress := common.HexToAddress("0x5d4fC7B5Aeea4CF4F0Ca6Be09A2F5AaDAd2F2803") // Holesky validator registry 6/13

	vrt, err := vrv1.NewValidatorregistryv1Transactor(contractAddress, client)
	if err != nil {
		log.Fatalf("Failed to create Validator Registry transactor: %v", err)
	}

	ec := utils.NewETHClient(client)

	ec.CancelPendingTxes(context.Background(), privateKey)

	opts, err := ec.CreateTransactOpts(context.Background(), privateKey, chainID)
	if err != nil {
		log.Fatalf("Failed to create transact opts: %v", err)
	}

	// obtain all validators staked under 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266 and remove them
	e := make(map[string]events.Event)
	stakedEvents, err := events.ReadEvents("staked")
	if err != nil {
		log.Fatalf("Failed to read staked events: %v", err)
	}
	unstakedEvents, err := events.ReadEvents("unstaked")
	if err != nil {
		log.Fatalf("Failed to read unstaked events: %v", err)
	}
	withdrawnEvents, err := events.ReadEvents("withdraw")
	if err != nil {
		log.Fatalf("Failed to read withdrawn events: %v", err)
	}

	for _, event := range stakedEvents {
		e[event.ValBLSPubKey] = event
	}
	for _, event := range unstakedEvents {
		delete(e, event.ValBLSPubKey)
	}
	for _, event := range withdrawnEvents {
		delete(e, event.ValBLSPubKey)
	}

	stakedVals, err := query.GetAllStakedValsFromNewRegistry()
	if err != nil {
		log.Fatalf("Failed to get all staked validators: %v", err)
	}

	toRemove := make([][]byte, 0)
	for _, stakedVal := range stakedVals {
		if e[stakedVal].TxOriginator == "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266" {
			toRemove = append(toRemove, common.Hex2Bytes(stakedVal))
		}
	}

	fmt.Println("Number of validators to unstake: ", len(toRemove))

	submitTx := func(
		ctx context.Context,
		opts *bind.TransactOpts,
	) (*types.Transaction, error) {

		tx, err := vrt.Unstake(opts, toRemove)
		if err != nil {
			return nil, fmt.Errorf("failed to unstake: %w", err)
		}
		fmt.Println("Unstake tx sent. Transaction hash: ", tx.Hash().Hex())
		return tx, nil
	}

	receipt, err := ec.WaitMinedWithRetry(context.Background(), opts, submitTx)
	if err != nil {
		if strings.Contains(err.Error(), "nonce too low") {
			fmt.Println("Nonce too low. This likely means the tx was included while constructing a retry...")
			receipt = &types.Receipt{Status: 1, BlockNumber: big.NewInt(0)}
		} else {
			log.Fatalf("Failed to wait for stake tx to be mined: %v", err)
		}
	}
	fmt.Println("Unstake tx included in block: ", receipt.BlockNumber)

	if receipt.Status == 0 {
		fmt.Println("Unstake tx included, but failed. Exiting...")
		os.Exit(1)
	}
}
