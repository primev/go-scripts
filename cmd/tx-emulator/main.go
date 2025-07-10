package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"math/rand"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	rpcURL              = "http://localhost:8545"
	dummyAddress        = "0x9999999999999999999999999999999999999999"
	transferAmount      = 1000000000000000 // 0.001 ETH
	txDelay             = 200 * time.Millisecond
	gasLimit            = 21000
	confirmationTimeout = 5 * time.Second
)

type Wallet struct {
	privateKey *ecdsa.PrivateKey
	address    common.Address
	client     *ethclient.Client
	nonce      uint64
	mu         sync.Mutex
}

func NewWallet(privateKeyBytes []byte, client *ethclient.Client) (*Wallet, error) {
	privateKey, err := crypto.ToECDSA(privateKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to convert private key: %v", err)
	}

	address := crypto.PubkeyToAddress(privateKey.PublicKey)

	nonce, err := client.PendingNonceAt(context.Background(), address)
	if err != nil {
		return nil, fmt.Errorf("failed to get nonce: %v", err)
	}

	return &Wallet{
		privateKey: privateKey,
		address:    address,
		client:     client,
		nonce:      nonce,
	}, nil
}

func (w *Wallet) sendAndWaitForTransaction(ctx context.Context, to common.Address, amount *big.Int) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	gasPrice, err := w.client.SuggestGasPrice(ctx)
	if err != nil {
		return fmt.Errorf("failed to get gas price: %v", err)
	}

	chainID, err := w.client.ChainID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get chain ID: %v", err)
	}

	tx := types.NewTransaction(
		w.nonce,
		to,
		amount,
		gasLimit,
		gasPrice,
		nil,
	)

	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), w.privateKey)
	if err != nil {
		return fmt.Errorf("failed to sign transaction: %v", err)
	}

	err = w.client.SendTransaction(ctx, signedTx)
	if err != nil {
		return fmt.Errorf("failed to send transaction: %v", err)
	}

	log.Printf("Sent tx %s from %s to %s (nonce: %d) - waiting for confirmation...",
		signedTx.Hash().Hex(),
		w.address.Hex(),
		to.Hex(),
		w.nonce)

	// Wait for transaction to be mined with timeout
	confirmCtx, cancel := context.WithTimeout(ctx, confirmationTimeout)
	defer cancel()

	receipt, err := bind.WaitMined(confirmCtx, w.client, signedTx)
	if err != nil {
		return fmt.Errorf("failed to wait for transaction confirmation: %v", err)
	}

	if receipt.Status == types.ReceiptStatusFailed {
		log.Printf("Transaction %s FAILED (block: %d, gas used: %d)",
			signedTx.Hash().Hex(),
			receipt.BlockNumber.Uint64(),
			receipt.GasUsed)
		w.nonce++
		return fmt.Errorf("transaction failed with status: %d", receipt.Status)
	}

	log.Printf("Transaction %s CONFIRMED (block: %d, gas used: %d)",
		signedTx.Hash().Hex(),
		receipt.BlockNumber.Uint64(),
		receipt.GasUsed)

	w.nonce++
	return nil
}

func (w *Wallet) spamTransactions(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	toAddress := common.HexToAddress(dummyAddress)
	amount := big.NewInt(transferAmount)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Stopping spam from address %s", w.address.Hex())
			return
		default:
			// Optional random delay between transactions
			randomDelay := time.Duration(rand.Intn(5)) * txDelay
			time.Sleep(randomDelay)

			err := w.sendAndWaitForTransaction(ctx, toAddress, amount)
			if err != nil {
				log.Printf("Error with transaction from %s: %v", w.address.Hex(), err)
				time.Sleep(time.Second * 5) // Wait longer on error
			}
		}
	}
}

func main() {
	privateKeyBytes := [][]byte{
		common.FromHex("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"),
		common.FromHex("0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"),
		common.FromHex("0x5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a"),
		common.FromHex("0x7c852118294e51e653712a81e05800f419141751be58f605c371e15141b007a6"),
		common.FromHex("0x47e179ec197488593b187f80a00eb0da91f1b9d0b13f8733639f19c30a34926a"),
		common.FromHex("0x8b3a350cf5c34c9194ca85829a2df0ec3153be0318b5e2d3348e872092edffba"),
		common.FromHex("0x92db14e403b83dfe3df233f83dfa3a0d7096f21ca9b0d6d6b8d88b2b4ec1564e"),
		common.FromHex("0x4bbbf85ce3377467afe5d46f804f221813b2bb87f24d81f60f1fcdbf7cbf4356"),
		common.FromHex("0xdbda1821b80551c9d65939329250298aa3472ba22feea921c0cf5d620ea67b97"),
		common.FromHex("0x2a871d0798f97d79848a013d4936a73bf4cc922c825d33c1cf7073dff6d409c6"),
	}

	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Fatalf("Failed to connect to Ethereum client: %v", err)
	}
	defer client.Close()

	log.Printf("Connected to Ethereum client at %s", rpcURL)

	var wallets []*Wallet
	for i, pk := range privateKeyBytes {
		wallet, err := NewWallet(pk, client)
		if err != nil {
			log.Fatalf("Failed to create wallet %d: %v", i, err)
		}
		wallets = append(wallets, wallet)
		log.Printf("Created wallet %d: %s", i, wallet.address.Hex())
	}

	for i, wallet := range wallets {
		balance, err := client.BalanceAt(context.Background(), wallet.address, nil)
		if err != nil {
			log.Printf("Failed to get balance for wallet %d: %v", i, err)
		} else {
			log.Printf("Wallet %d (%s) balance: %f ETH", i, wallet.address.Hex(), float64(balance.Int64())/1e18)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	for i, wallet := range wallets {
		wg.Add(1)
		go wallet.spamTransactions(ctx, &wg)
		log.Printf("Started transaction spam from wallet %d (%s)", i, wallet.address.Hex())
	}

	log.Printf("Started transaction spam from %d wallets to %s", len(wallets), dummyAddress)
	log.Printf("Each transaction sends %f ETH", float64(transferAmount)/1e18)
	log.Printf("Press Ctrl+C to stop...")

	wg.Wait()
	log.Println("All transaction spammers stopped")
}
