package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	utils "github.com/primevprotocol/validator-registry/pkg/utils"
	vr "github.com/primevprotocol/validator-registry/pkg/validatorregistry"
)

func main() {

	privateKeyString := os.Getenv("PRIVATE_KEY")
	if privateKeyString == "" {
		fmt.Println("PRIVATE_KEY env var not supplied")
		os.Exit(1)
	}

	if privateKeyString[:2] == "0x" {
		privateKeyString = privateKeyString[2:]
	}
	privateKey, err := crypto.HexToECDSA(privateKeyString)
	if err != nil {
		fmt.Println("Failed to parse private key")
		os.Exit(1)
	}

	client, err := ethclient.Dial("https://chainrpc.testnet.mev-commit.xyz")
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
	if balance.Cmp(big.NewInt(3100000000000000000)) == -1 {
		log.Fatalf("Insufficient balance. Please fund %v with at least 3.1 ETH", fromAddress.Hex())
	}

	contractAddress := common.HexToAddress("0xF263483500e849Bd8d452c9A0F075B606ee64087") // Accurate as of 4/24/2024

	vrt, err := vr.NewValidatorregistryTransactor(contractAddress, client)
	if err != nil {
		log.Fatalf("Failed to create Validator Registry transactor: %v", err)
	}

	ec := utils.NewETHClient(client)

	publicKeyFilePath := "../../keys_example.txt"
	pksAsBytes, err := readBLSPublicKeysFromFile(publicKeyFilePath)
	if err != nil {
		log.Fatalf("Failed to read public keys from file: %v", err)
	}

	batchSize := 20
	type Batch struct {
		pubKeys [][]byte
	}
	batches := make([]Batch, 0)
	for i := 0; i < len(pksAsBytes); i += batchSize {
		end := i + batchSize
		if end > len(pksAsBytes) {
			end = len(pksAsBytes)
		}
		batches = append(batches, Batch{pubKeys: pksAsBytes[i:end]})
	}

	for idx, batch := range batches {

		opts, err := ec.CreateTransactOpts(context.Background(), privateKey, chainID)
		if err != nil {
			log.Fatalf("Failed to create transact opts: %v", err)
		}

		amountPerValidator := new(big.Int)
		amountPerValidator.SetString("3100000000000000000", 10)
		totalAmount := new(big.Int).Mul(amountPerValidator, big.NewInt(int64(batchSize)))
		opts.Value = totalAmount

		submitTx := func(
			ctx context.Context,
			opts *bind.TransactOpts,
		) (*types.Transaction, error) {

			tx, err := vrt.Stake(opts, batch.pubKeys)
			if err != nil {
				return nil, fmt.Errorf("failed to stake: %w", err)
			}
			fmt.Println("Stake tx sent. Transaction hash: ", tx.Hash().Hex())
			return tx, nil
		}

		receipt, err := ec.WaitMinedWithRetry(context.Background(), opts, submitTx)
		if err != nil {
			log.Fatalf("Failed to wait for stake tx to be mined: %v", err)
		}
		fmt.Println("Stake tx included in block: ", receipt.BlockNumber)

		if receipt.Status == 0 {
			fmt.Println("Stake tx included, but failed. Exiting...")
			os.Exit(1)
		}

		fmt.Println("-------------------")
		fmt.Printf("Batch %d completed\n", idx+1)
		fmt.Println("-------------------")
	}
	fmt.Println("All staking batches completed!")
}

func readBLSPublicKeysFromFile(filePath string) ([][]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var keys [][]byte
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key := scanner.Text()
		if len(key) > 2 && key[:2] == "0x" {
			key = key[2:]
		}
		keyBytes := common.Hex2Bytes(key)
		keys = append(keys, keyBytes)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return keys, nil
}
