package utils

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type ETHClient struct {
	logger *slog.Logger
	client *ethclient.Client
}

func NewETHClient(logger *slog.Logger, client *ethclient.Client) *ETHClient {
	return &ETHClient{logger: logger, client: client}
}

func (c *ETHClient) CreateTransactOpts(
	ctx context.Context,
	privateKey *ecdsa.PrivateKey,
	srcChainID *big.Int,
) (*bind.TransactOpts, error) {
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, srcChainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create transactor: %w", err)
	}

	fromAddress := auth.From
	nonce, err := c.client.PendingNonceAt(ctx, fromAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending nonce: %w", err)
	}
	auth.Nonce = big.NewInt(int64(nonce))

	gasTip, gasPrice, err := c.SuggestGasTipCapAndPrice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to suggest gas tip cap and price: %w", err)
	}

	auth.GasFeeCap = gasPrice
	auth.GasTipCap = gasTip
	auth.GasLimit = uint64(3000000)
	return auth, nil
}

func (c *ETHClient) SuggestGasTipCapAndPrice(ctx context.Context) (*big.Int, *big.Int, error) {
	// Returns priority fee per gas
	gasTip, err := c.client.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get gas tip cap: %w", err)
	}
	// Returns priority fee per gas + base fee per gas
	gasPrice, err := c.client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get gas price: %w", err)
	}
	return gasTip, gasPrice, nil
}

func (c *ETHClient) BoostTipForTransactOpts(
	ctx context.Context,
	opts *bind.TransactOpts,
) error {
	c.logger.Debug(
		"gas params for tx that were not included",
		"gas_tip", opts.GasTipCap.String(),
		"gas_fee_cap", opts.GasFeeCap.String(),
		"base_fee", new(big.Int).Sub(opts.GasFeeCap, opts.GasTipCap).String(),
	)

	newGasTip, newFeeCap, err := c.SuggestGasTipCapAndPrice(ctx)
	if err != nil {
		return fmt.Errorf("failed to suggest gas tip cap and price: %w", err)
	}

	newBaseFee := new(big.Int).Sub(newFeeCap, newGasTip)
	if newBaseFee.Cmp(big.NewInt(0)) == -1 {
		return fmt.Errorf("new base fee cannot be negative: %s", newBaseFee.String())
	}

	prevBaseFee := new(big.Int).Sub(opts.GasFeeCap, opts.GasTipCap)
	if prevBaseFee.Cmp(big.NewInt(0)) == -1 {
		return fmt.Errorf("base fee cannot be negative: %s", prevBaseFee.String())
	}

	var maxBaseFee *big.Int
	if newBaseFee.Cmp(prevBaseFee) == 1 {
		maxBaseFee = newBaseFee
	} else {
		maxBaseFee = prevBaseFee
	}

	var maxGasTip *big.Int
	if newGasTip.Cmp(opts.GasTipCap) == 1 {
		maxGasTip = newGasTip
	} else {
		maxGasTip = opts.GasTipCap
	}

	boostedTip := new(big.Int).Add(maxGasTip, new(big.Int).Div(maxGasTip, big.NewInt(10)))
	boostedTip = boostedTip.Add(boostedTip, big.NewInt(1))

	boostedBaseFee := new(big.Int).Add(maxBaseFee, new(big.Int).Div(maxBaseFee, big.NewInt(10)))
	boostedBaseFee = boostedBaseFee.Add(boostedBaseFee, big.NewInt(1))

	opts.GasTipCap = boostedTip
	opts.GasFeeCap = new(big.Int).Add(boostedBaseFee, boostedTip)

	c.logger.Debug("tip and base fee will be boosted by 10%")
	c.logger.Debug(
		"boosted gas",
		"get_tip_cap", opts.GasTipCap.String(),
		"gas_fee_cap", opts.GasFeeCap.String(),
		"base_fee", boostedBaseFee.String(),
	)

	return nil
}

type TxSubmitFunc func(
	ctx context.Context,
	opts *bind.TransactOpts,
) (
	tx *types.Transaction,
	err error,
)

func (c *ETHClient) WaitMinedWithRetry(
	ctx context.Context,
	opts *bind.TransactOpts,
	submitTx TxSubmitFunc,
) (*types.Receipt, error) {

	const maxRetries = 10
	var err error
	var tx *types.Transaction

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			c.logger.Info("transaction not included within 60 seconds, boosting gas tip by 10%", "attempt", attempt)
			if err := c.BoostTipForTransactOpts(ctx, opts); err != nil {
				return nil, fmt.Errorf("failed to boost gas tip for attempt %d: %w", attempt, err)
			}
		}

		tx, err = submitTx(ctx, opts)
		if err != nil {
			if strings.Contains(err.Error(), "replacement transaction underpriced") || strings.Contains(err.Error(), "already known") {
				c.logger.Error("tx submission failed", "attempt", attempt, "error", err)
				continue
			}
			return nil, fmt.Errorf("tx submission failed on attempt %d: %w", attempt, err)
		}

		timeoutCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		receiptChan := make(chan *types.Receipt)
		errChan := make(chan error)

		go func() {
			receipt, err := bind.WaitMined(timeoutCtx, c.client, tx)
			if err != nil {
				errChan <- err
				return
			}
			receiptChan <- receipt
		}()

		select {
		case receipt := <-receiptChan:
			cancel()
			return receipt, nil
		case err := <-errChan:
			cancel()
			return nil, err
		case <-timeoutCtx.Done():
			cancel()
			if attempt == maxRetries-1 {
				return nil, fmt.Errorf("tx not included after %d attempts", maxRetries)
			}
			// Continue with boosted tip
		}
	}
	return nil, fmt.Errorf("unexpected error: control flow should not reach end of WaitMinedWithRetry")
}
