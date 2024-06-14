package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	utils "github.com/primevprotocol/validator-registry/pkg/utils"
	vrv1 "github.com/primevprotocol/validator-registry/pkg/validatorregistryV1"
)

type Event struct {
	TxOriginator string   `json:"tx_originator"`
	ValBLSPubKey string   `json:"val_bls_pub_key"`
	Amount       *big.Int `json:"amount"`
	Block        uint64   `json:"block"`
}

type Batch struct {
	pubKeys         [][]byte
	stakeOriginator common.Address
}

func isValidatorRegisteredWithBeaconChain(pubKey string) (bool, error) {
	url := fmt.Sprintf("https://holesky.beaconcha.in/api/v1/validator/%s", pubKey)
	resp, err := http.Get(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("failed to get validator status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return false, err
	}

	status, ok := result["status"].(string)
	if !ok || status != "OK" {
		return false, nil
	}

	return true, nil
}

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
	if balance.Cmp(big.NewInt(1000000000000000000)) == -1 {
		log.Fatalf("Insufficient balance. Please fund %v with at least 1 ETH", fromAddress.Hex())
	}

	contractAddress := common.HexToAddress("0x5d4fC7B5Aeea4CF4F0Ca6Be09A2F5AaDAd2F2803") // Holesky validator registry 6/13

	vrt, err := vrv1.NewValidatorregistryv1Transactor(contractAddress, client)
	if err != nil {
		log.Fatalf("Failed to create Validator Registry transactor: %v", err)
	}

	ec := utils.NewETHClient(nil, client)

	artifactDir := "../../artifacts"
	eventTypes := []string{"staked", "unstaked", "withdraw"}

	for _, eventType := range eventTypes {
		files, err := filepath.Glob(fmt.Sprintf("%s/%s_events_*.json", artifactDir, eventType))
		if err != nil {
			log.Fatalf("Failed to list %s event files: %v", eventType, err)
		}

		if len(files) == 0 {
			log.Fatalf("No %s event files found", eventType)
		}

		sort.Slice(files, func(i, j int) bool {
			infoI, err := os.Stat(files[i])
			if err != nil {
				log.Fatalf("Failed to stat file %s: %v", files[i], err)
			}
			infoJ, err := os.Stat(files[j])
			if err != nil {
				log.Fatalf("Failed to stat file %s: %v", files[j], err)
			}
			return infoI.ModTime().After(infoJ.ModTime())
		})

		recentFile := files[0]
		fmt.Printf("Using artifact file: %s\n", recentFile)

		f, err := os.Open(recentFile)
		if err != nil {
			log.Fatalf("Failed to open file %s: %v", recentFile, err)
		}
		defer f.Close()

		var events []Event
		if err := json.NewDecoder(f).Decode(&events); err != nil {
			log.Fatalf("Failed to decode events from file %s: %v", recentFile, err)
		}

		batches := make(map[string]Batch)
		for _, event := range events {
			registered, err := isValidatorRegisteredWithBeaconChain(event.ValBLSPubKey)
			if err != nil {
				log.Fatalf("Failed to check validator registration with beacon chain: %v", err)
			}
			if registered {
				if batch, exists := batches[event.TxOriginator]; exists {
					batch.pubKeys = append(batch.pubKeys, common.Hex2Bytes(event.ValBLSPubKey))
					batches[event.TxOriginator] = batch
				} else {
					batches[event.TxOriginator] = Batch{
						pubKeys:         [][]byte{common.Hex2Bytes(event.ValBLSPubKey)},
						stakeOriginator: common.HexToAddress(event.TxOriginator),
					}
				}
			} else {
				fmt.Printf("Skipping validator who is not registered with beacon chain: %s\n", event.ValBLSPubKey)
			}
		}

		for idx, batch := range batches {
			opts, err := ec.CreateTransactOpts(context.Background(), privateKey, chainID)
			if err != nil {
				log.Fatalf("Failed to create transact opts: %v", err)
			}
			amountPerValidator := new(big.Int)
			amountPerValidator.SetString("100000000000000", 10) // 0.0001 ether in wei
			opts.Value = amountPerValidator

			submitTx := func(
				ctx context.Context,
				opts *bind.TransactOpts,
			) (*types.Transaction, error) {
				tx, err := vrt.DelegateStake(opts, batch.pubKeys, batch.stakeOriginator)
				if err != nil {
					return nil, fmt.Errorf("failed to send transaction for batch %s: %w", idx, err)
				}
				fmt.Printf("Transaction sent for batch %s: %s\n", idx, tx.Hash().Hex())
				return tx, nil
			}

			receipt, err := ec.WaitMinedWithRetry(context.Background(), opts, submitTx)
			if err != nil {
				log.Fatalf("Failed to wait for transaction receipt: %v", err)
			}

			fmt.Println("Delegate stake tx included in block: ", receipt.BlockNumber)

			if receipt.Status == 0 {
				fmt.Println("Delegate stake tx included, but failed. Exiting...")
				os.Exit(1)
			}

			fmt.Println("-------------------")
			fmt.Printf("Batch %s completed\n", idx)
			fmt.Println("-------------------")
		}
		fmt.Println("All staking batches completed!")
	}
}
