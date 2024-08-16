package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/primevprotocol/validator-registry/pkg/events"
	vrv1 "github.com/primevprotocol/validator-registry/pkg/validatorregistryv1"
	vrv1_aug15 "github.com/primevprotocol/validator-registry/pkg/validatorregistryv1_aug15"
)

type Batch struct {
	pubKeys         [][]byte
	stakeOriginator common.Address
}

func main() {
	// TODO: can change to keystore
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
	// TODO: remove
	fmt.Println("privateKey: ", privateKey)

	client, err := ethclient.Dial("https://ethereum-holesky-rpc.publicnode.com")
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		log.Fatalf("Failed to get chain id: %v", err)
	}
	fmt.Println("Chain ID: ", chainID)

	// TODO: add this back

	// fromAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
	// balance, err := client.BalanceAt(context.Background(), fromAddress, nil)
	// if err != nil {
	// 	log.Fatalf("Failed to get account balance: %v", err)
	// }
	// if balance.Cmp(big.NewInt(1000000000000000000)) == -1 {
	// 	log.Fatalf("Insufficient balance. Please fund %v with at least 1 ETH", fromAddress.Hex())
	// }

	oldValRegAddr := common.HexToAddress("0x5d4fC7B5Aeea4CF4F0Ca6Be09A2F5AaDAd2F2803") // Holesky validator registry 6/13

	vrf, err := vrv1.NewValidatorregistryv1Filterer(oldValRegAddr, client)
	if err != nil {
		log.Fatalf("Failed to create Validator Registry filterer: %v", err)
	}

	vrta15, err := vrv1_aug15.NewValidatorregistryv1Transactor(oldValRegAddr, client)
	if err != nil {
		log.Fatalf("Failed to create Validator Registry aug15 transactor: %v", err)
	}

	fmt.Println("vrf: ", vrf)
	fmt.Println("vrta15: ", vrta15)

	// ec := utils.NewETHClient(client)

	// ec.CancelPendingTxes(context.Background(), privateKey)

	// e := make(map[string]events.Event)

	currentBlock, err := client.BlockByNumber(context.Background(), nil)
	if err != nil {
		log.Fatalf("Failed to get current block: %v", err)
	}
	fmt.Println("Current block: ", currentBlock.NumberU64())

	// // obtain events from old registry, in batches of 50000
	totEvents := []events.Event{}
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
			totEvents = append(totEvents, event)
		}
		fmt.Println("Next iteration")
	}

	numEvents := 0
	for _, event := range totEvents {
		numEvents++
		fmt.Println(event.TxOriginator)
		fmt.Println(event.ValBLSPubKey)
		fmt.Println(event.Amount)
		fmt.Println("-------------------")
	}
	fmt.Println("Number of events: ", numEvents)

	// print first 1000 events
	for i := 0; i < 1000; i++ {
		fmt.Println(totEvents[i])
		fmt.Println("-------------------")
	}

	// TODO: delete both default account events, and events from vals that are no longer staked=true
	// TODO: confirm stake originator is what we care about here.. it shouldn't be the addr you used to migrate.
	// but even if we're using the wrong account here, just get the data migrated..

	// deletedFromDefault := 0
	// for _, event := range e {
	// 	if event.TxOriginator == "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266" {
	// 		delete(e, event.ValBLSPubKey)
	// 		deletedFromDefault++
	// 	}
	// }
	// fmt.Println("Number of events deleted from default account: ", deletedFromDefault)

	// // fmt.Println("Number of validators to check on beacon chain: ", len(e))

	// batches := make(map[string]Batch)
	// // skipped := 0
	// batched := 0
	// for _, event := range e {
	// 	// registered, err := isValidatorRegisteredWithBeaconChain(event.ValBLSPubKey)
	// 	// if err != nil {
	// 	// 	log.Fatalf("Failed to check validator registration with beacon chain: %v", err)
	// 	// }
	// 	// if registered {
	// 	// fmt.Println("Validator is registered with beacon chain: ", event.ValBLSPubKey)

	// 	batched++
	// 	if batch, exists := batches[event.TxOriginator]; exists {
	// 		batch.pubKeys = append(batch.pubKeys, common.Hex2Bytes(event.ValBLSPubKey))
	// 		batches[event.TxOriginator] = batch
	// 	} else {
	// 		batches[event.TxOriginator] = Batch{
	// 			pubKeys:         [][]byte{common.Hex2Bytes(event.ValBLSPubKey)},
	// 			stakeOriginator: common.HexToAddress(event.TxOriginator),
	// 		}
	// 	}

	// 	// } else {
	// 	// 	fmt.Printf("Skipping validator who is not registered with beacon chain: %s\n", event.ValBLSPubKey)
	// 	// 	skipped++
	// 	// }
	// }
	// // fmt.Println("Number of validators skipped for not being registered with beacon chain: ", skipped)
	// fmt.Println("Number of validators batched: ", batched)

	// // print lens of batches
	// fmt.Println("Number of batches: ", len(batches))
	// for _, batch := range batches {
	// 	fmt.Println("Batch size: ", len(batch.pubKeys))
	// }

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
