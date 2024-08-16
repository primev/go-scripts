package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/primevprotocol/validator-registry/pkg/events"
	"github.com/primevprotocol/validator-registry/pkg/query"
	vrv1 "github.com/primevprotocol/validator-registry/pkg/validatorregistryv1"
	vrv1_aug15 "github.com/primevprotocol/validator-registry/pkg/validatorregistryv1_aug15"
)

type Batch struct {
	pubKeys         [][]byte
	stakeOriginator common.Address
}

func main() {

	keystorePath := os.Getenv("PRIVATE_KEYSTORE_PATH")
	if keystorePath == "" {
		log.Fatalf("PRIVATE_KEYSTORE_PATH is not set")
	}

	_, err := os.Stat(keystorePath)
	if err != nil {
		log.Fatalf("Failed to stat keystore path: %v", err)
	}

	keystorePassword := os.Getenv("PRIVATE_KEYSTORE_PASSWORD")
	if keystorePassword == "" {
		log.Fatalf("PRIVATE_KEYSTORE_PASSWORD is not set")
	}

	dir := filepath.Dir(keystorePath)

	keystore := keystore.NewKeyStore(dir, keystore.LightScryptN, keystore.LightScryptP)
	ksAccounts := keystore.Accounts()

	var account accounts.Account
	if len(ksAccounts) == 0 {
		log.Fatalf("no accounts in dir: %s", dir)
	} else {
		found := false
		for _, acc := range ksAccounts {
			if acc.Address == common.HexToAddress("0x4535bd6fF24860b5fd2889857651a85fb3d3C6b1") {
				account = acc
				found = true
				break
			}
		}
		if !found {
			log.Fatalf("account %s not found in keystore dir: %s", "0x4535bd6fF24860b5fd2889857651a85fb3d3C6b1", dir)
		}
	}

	if err := keystore.Unlock(account, keystorePassword); err != nil {
		log.Fatalf("failed to unlock account: %v", err)
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

	tOpts, err := bind.NewKeyStoreTransactorWithChainID(keystore, account, chainID)
	if err != nil {
		log.Fatalf("failed to get auth: %v", err)
	}
	tOpts.From = account.Address
	nonce, err := client.PendingNonceAt(context.Background(), account.Address)
	if err != nil {
		log.Fatalf("failed to get pending nonce: %v", err)
	}
	tOpts.Nonce = big.NewInt(int64(nonce))

	gasTip, gasPrice, err := SuggestGasTipCapAndPrice(context.Background(), client)
	if err != nil {
		log.Fatalf("failed to suggest gas tip cap and price: %v", err)
	}
	tOpts.GasFeeCap = gasPrice
	tOpts.GasTipCap = gasTip
	tOpts.GasLimit = 300000

	balance, err := client.BalanceAt(context.Background(), account.Address, nil)
	if err != nil {
		log.Fatalf("Failed to get account balance: %v", err)
	}
	if balance.Cmp(big.NewInt(1000000000000000000)) == -1 {
		log.Fatalf("Insufficient balance. Please fund %v with at least 1 ETH", account.Address.Hex())
	}

	oldValRegAddr := common.HexToAddress("0x5d4fC7B5Aeea4CF4F0Ca6Be09A2F5AaDAd2F2803") // Holesky validator registry 6/13
	vrf, err := vrv1.NewValidatorregistryv1Filterer(oldValRegAddr, client)
	if err != nil {
		log.Fatalf("Failed to create Validator Registry filterer: %v", err)
	}

	newValRegAddr := common.HexToAddress("0x87D5F694fAD0b6C8aaBCa96277DE09451E277Bcf")
	vrta15, err := vrv1_aug15.NewValidatorregistryv1Transactor(newValRegAddr, client)
	if err != nil {
		log.Fatalf("Failed to create Validator Registry aug15 transactor: %v", err)
	}

	fmt.Println("vrta15: ", vrta15)

	// ec := utils.NewETHClient(client)
	// ec.CancelPendingTxes(context.Background(), privateKey)

	currentBlock, err := client.BlockByNumber(context.Background(), nil)
	if err != nil {
		log.Fatalf("Failed to get current block: %v", err)
	}
	fmt.Println("Current block: ", currentBlock.NumberU64())

	// // obtain events from old registry, in batches of 50000
	totEvents := make(map[string]events.Event)
	for i := 0; i < int(currentBlock.NumberU64()); i += 50000 {
		start := uint64(i)
		end := uint64(i + 50000)
		if end > currentBlock.NumberU64() {
			end = currentBlock.NumberU64()
		}
		opts := &bind.FilterOpts{
			Start:   start,
			End:     &end,
			Context: context.Background(),
		}
		stakedEvents, err := vrf.FilterStaked(opts, nil)
		if err != nil {
			log.Fatalf("Failed to get staked events: %v", err)
		}
		for stakedEvents.Next() {
			event := events.Event{
				ValBLSPubKey: hex.EncodeToString(stakedEvents.Event.ValBLSPubKey),
				TxOriginator: stakedEvents.Event.TxOriginator.Hex(),
				Amount:       stakedEvents.Event.Amount,
			}
			totEvents[event.ValBLSPubKey] = event
		}
		fmt.Println("Next iteration")
	}

	deletedFromDefault := 0
	for _, event := range totEvents {
		if event.TxOriginator == "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266" {
			delete(totEvents, event.ValBLSPubKey)
			deletedFromDefault++
		}
	}
	fmt.Println("Number of events deleted from default account: ", deletedFromDefault)

	stakedValidators, err := query.GetAllStakedValsFromRegistry()
	if err != nil {
		log.Fatalf("Failed to get staked validators: %v", err)
	}

	stakedValidatorsMap := make(map[string]bool)
	for _, validator := range stakedValidators {
		stakedValidatorsMap[validator] = true
	}

	// delete events from vals that are not in stakedValidators
	deletedFromStaked := 0
	for _, event := range totEvents {
		if !stakedValidatorsMap[event.ValBLSPubKey] {
			delete(totEvents, event.ValBLSPubKey)
			deletedFromStaked++
		}
	}
	fmt.Println("Number of events deleted from staked validators: ", deletedFromStaked)

	numEvents := 0
	for _, event := range totEvents {
		numEvents++
		fmt.Println(event.TxOriginator)
		fmt.Println(event.ValBLSPubKey)
		fmt.Println(event.Amount)
		fmt.Println("-------------------")
	}
	fmt.Println("Number of events to act upon: ", numEvents)

	// organize into map of txOriginator to slice of pubKeys
	batches := make(map[string]Batch)
	for _, event := range totEvents {
		if batch, exists := batches[event.TxOriginator]; exists {
			batch.pubKeys = append(batch.pubKeys, common.Hex2Bytes(event.ValBLSPubKey))
			batches[event.TxOriginator] = batch
		} else {
			batches[event.TxOriginator] = Batch{
				pubKeys:         [][]byte{common.Hex2Bytes(event.ValBLSPubKey)},
				stakeOriginator: common.HexToAddress(event.TxOriginator),
			}
		}
	}

	// print lens of batches
	fmt.Println("Number of batches: ", len(batches))
	for _, batch := range batches {
		fmt.Println("Batch size: ", len(batch.pubKeys))
	}

	// biggestBatchSize := 20
	// for idx, batch := range batches {
	// 	// split into sub batches of 20 or less
	// 	for i := 0; i < len(batch.pubKeys); i += biggestBatchSize {
	// 		end := i + biggestBatchSize
	// 		if end > len(batch.pubKeys) {
	// 			end = len(batch.pubKeys)
	// 		}
	// 		subBatch := batch.pubKeys[i:end]

	// 		opts, err := ec.CreateTransactOpts(context.Background(), privateKey, chainID)
	// 		if err != nil {
	// 			log.Fatalf("Failed to create transact opts: %v", err)
	// 		}

	// 		amountPerValidator := new(big.Int)
	// 		// 0.0001 ether
	// 		amountPerValidator.SetString("100000000000000", 10)
	// 		totalAmount := new(big.Int).Mul(amountPerValidator, big.NewInt(int64(len(subBatch))))
	// 		opts.Value = totalAmount

	// 		submitTx := func(
	// 			ctx context.Context,
	// 			opts *bind.TransactOpts,
	// 		) (*types.Transaction, error) {

	// 			tx, err := vrt.DelegateStake(opts, subBatch, batch.stakeOriginator)
	// 			if err != nil {
	// 				return nil, fmt.Errorf("failed to stake: %w", err)
	// 			}
	// 			fmt.Println("DelegateStake tx sent. Transaction hash: ", tx.Hash().Hex())
	// 			return tx, nil
	// 		}

	// 		receipt, err := ec.WaitMinedWithRetry(context.Background(), opts, submitTx)
	// 		if err != nil {
	// 			if strings.Contains(err.Error(), "nonce too low") {
	// 				fmt.Println("Nonce too low. This likely means the tx was included while constructing a retry...")
	// 				receipt = &types.Receipt{Status: 1, BlockNumber: big.NewInt(0)}
	// 			} else {
	// 				log.Fatalf("Failed to wait for stake tx to be mined: %v", err)
	// 			}
	// 		}
	// 		fmt.Println("DelegateStake tx included in block: ", receipt.BlockNumber)

	// 		if receipt.Status == 0 {
	// 			fmt.Println("DelegateStake tx included, but failed. Exiting...")
	// 			os.Exit(1)
	// 		}

	// 		fmt.Println("-------------------")
	// 		fmt.Printf("Batch %s completed\n", idx)
	// 		fmt.Println("-------------------")
	// 	}
	// }
	// fmt.Println("All batches completed!")
}

func SuggestGasTipCapAndPrice(ctx context.Context, client *ethclient.Client) (
	gasTip *big.Int, gasPrice *big.Int, err error) {

	// Returns priority fee per gas
	gasTip, err = client.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get gas tip cap: %w", err)
	}
	// Returns priority fee per gas + base fee per gas
	gasPrice, err = client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get gas price: %w", err)
	}
	return gasTip, gasPrice, nil
}
