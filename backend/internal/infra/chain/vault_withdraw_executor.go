package chain

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/shopspring/decimal"
	"github.com/xiaobao/rgperp/backend/internal/pkg/authx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

var (
	withdrawExecutorVaultABI = mustParseWithdrawABI(`[{"inputs":[{"internalType":"address","name":"token","type":"address"},{"internalType":"address","name":"to","type":"address"},{"internalType":"uint256","name":"amount","type":"uint256"},{"internalType":"bytes32","name":"withdrawId","type":"bytes32"}],"name":"withdraw","outputs":[],"stateMutability":"nonpayable","type":"function"}]`)
	erc20BalanceABI          = mustParseWithdrawABI(`[{"inputs":[{"internalType":"address","name":"account","type":"address"}],"name":"balanceOf","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"}]`)
)

type ReviewRequiredError struct {
	Reason string
}

func (e ReviewRequiredError) Error() string {
	if strings.TrimSpace(e.Reason) == "" {
		return "review required"
	}
	return fmt.Sprintf("review required: %s", e.Reason)
}

func IsReviewRequired(err error) bool {
	var target ReviewRequiredError
	return errors.As(err, &target)
}

type VaultWithdrawChainConfig struct {
	ChainID      int64
	RPCURL       string
	VaultAddress string
	TokenAddress string
}

type VaultWithdrawExecutor struct {
	privateKey *ecdsa.PrivateKey
	clients    map[int64]*ethclient.Client
	chains     map[int64]VaultWithdrawChainConfig
}

func NewVaultWithdrawExecutor(privateKeyHex string, chains []VaultWithdrawChainConfig) (*VaultWithdrawExecutor, error) {
	keyHex := strings.TrimSpace(strings.TrimPrefix(privateKeyHex, "0x"))
	if keyHex == "" {
		return nil, fmt.Errorf("%w: withdraw executor private key is required", errorsx.ErrInvalidArgument)
	}
	privateKey, err := crypto.HexToECDSA(keyHex)
	if err != nil {
		return nil, err
	}
	out := &VaultWithdrawExecutor{
		privateKey: privateKey,
		clients:    make(map[int64]*ethclient.Client, len(chains)),
		chains:     make(map[int64]VaultWithdrawChainConfig, len(chains)),
	}
	for _, chain := range chains {
		if chain.ChainID <= 0 || strings.TrimSpace(chain.RPCURL) == "" {
			continue
		}
		vaultAddress, err := authx.NormalizeEVMAddress(chain.VaultAddress)
		if err != nil {
			out.Close()
			return nil, err
		}
		tokenAddress, err := authx.NormalizeEVMAddress(chain.TokenAddress)
		if err != nil {
			out.Close()
			return nil, err
		}
		client, err := ethclient.Dial(chain.RPCURL)
		if err != nil {
			out.Close()
			return nil, err
		}
		chain.VaultAddress = vaultAddress
		chain.TokenAddress = tokenAddress
		out.clients[chain.ChainID] = client
		out.chains[chain.ChainID] = chain
	}
	return out, nil
}

func (e *VaultWithdrawExecutor) Close() {
	for _, client := range e.clients {
		client.Close()
	}
}

func (e *VaultWithdrawExecutor) CheckChainHealth(ctx context.Context, chainID int64) error {
	client, _, err := e.client(chainID)
	if err != nil {
		return ReviewRequiredError{Reason: err.Error()}
	}
	if _, err := client.BlockNumber(ctx); err != nil {
		return ReviewRequiredError{Reason: "chain_unavailable"}
	}
	return nil
}

func (e *VaultWithdrawExecutor) VaultBalance(ctx context.Context, chainID int64) (string, error) {
	client, chain, err := e.client(chainID)
	if err != nil {
		return "", err
	}
	input, err := erc20BalanceABI.Pack("balanceOf", common.HexToAddress(chain.VaultAddress))
	if err != nil {
		return "", err
	}
	msg := ethereum.CallMsg{
		To:   ptr(common.HexToAddress(chain.TokenAddress)),
		Data: input,
	}
	result, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		return "", err
	}
	values, err := erc20BalanceABI.Unpack("balanceOf", result)
	if err != nil {
		return "", err
	}
	balance, ok := values[0].(*big.Int)
	if !ok {
		return "", fmt.Errorf("vault balance decode failed")
	}
	return decimal.NewFromBigInt(balance, -6).String(), nil
}

func (e *VaultWithdrawExecutor) ExecuteWithdrawal(ctx context.Context, chainID int64, toAddress string, amount string, withdrawID string) (string, error) {
	client, chain, err := e.client(chainID)
	if err != nil {
		return "", err
	}
	normalizedTo, err := authx.NormalizeEVMAddress(toAddress)
	if err != nil {
		return "", err
	}
	amountBaseUnits, err := decimalStringToBaseUnits(amount, 6)
	if err != nil {
		return "", err
	}
	if amountBaseUnits.Sign() <= 0 {
		return "", fmt.Errorf("%w: withdraw amount must be positive", errorsx.ErrInvalidArgument)
	}
	balanceRaw, err := e.VaultBalance(ctx, chainID)
	if err != nil {
		return "", ReviewRequiredError{Reason: "hot_wallet_balance_check_failed"}
	}
	balance, err := decimal.NewFromString(balanceRaw)
	if err != nil {
		return "", err
	}
	required, err := decimal.NewFromString(amount)
	if err != nil {
		return "", err
	}
	if balance.LessThan(required) {
		return "", ReviewRequiredError{Reason: "hot_wallet_insufficient_balance"}
	}

	auth, err := bindKeyedTx(ctx, e.privateKey, chainID, client)
	if err != nil {
		return "", ReviewRequiredError{Reason: "chain_signer_unavailable"}
	}
	withdrawBytes32, err := encodeWithdrawID(withdrawID)
	if err != nil {
		return "", err
	}
	callData, err := withdrawExecutorVaultABI.Pack("withdraw", common.HexToAddress(chain.TokenAddress), common.HexToAddress(normalizedTo), amountBaseUnits, withdrawBytes32)
	if err != nil {
		return "", err
	}
	gasLimit, err := client.EstimateGas(ctx, ethereum.CallMsg{
		From: auth.From,
		To:   ptr(common.HexToAddress(chain.VaultAddress)),
		Data: callData,
	})
	if err != nil {
		return "", ReviewRequiredError{Reason: "withdraw_estimate_failed"}
	}
	unsignedTx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   big.NewInt(chainID),
		Nonce:     auth.Nonce,
		GasTipCap: auth.GasTipCap,
		GasFeeCap: auth.GasFeeCap,
		Gas:       gasLimit,
		To:        ptr(common.HexToAddress(chain.VaultAddress)),
		Value:     big.NewInt(0),
		Data:      callData,
	})
	signedTx, err := auth.Signer(auth.From, unsignedTx)
	if err != nil {
		return "", err
	}
	if err := client.SendTransaction(ctx, signedTx); err != nil {
		return "", err
	}
	return signedTx.Hash().Hex(), nil
}

type txAuth struct {
	From      common.Address
	Nonce     uint64
	GasTipCap *big.Int
	GasFeeCap *big.Int
	Signer    func(common.Address, *types.Transaction) (*types.Transaction, error)
}

func bindKeyedTx(ctx context.Context, privateKey *ecdsa.PrivateKey, chainID int64, client *ethclient.Client) (txAuth, error) {
	from := crypto.PubkeyToAddress(privateKey.PublicKey)
	nonce, err := client.PendingNonceAt(ctx, from)
	if err != nil {
		return txAuth{}, err
	}
	gasTipCap, err := client.SuggestGasTipCap(ctx)
	if err != nil {
		return txAuth{}, err
	}
	header, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		return txAuth{}, err
	}
	signer := types.LatestSignerForChainID(big.NewInt(chainID))
	gasFeeCap := new(big.Int).Add(gasTipCap, new(big.Int).Mul(header.BaseFee, big.NewInt(2)))
	return txAuth{
		From:      from,
		Nonce:     nonce,
		GasTipCap: gasTipCap,
		GasFeeCap: gasFeeCap,
		Signer: func(addr common.Address, tx *types.Transaction) (*types.Transaction, error) {
			if addr != from {
				return nil, fmt.Errorf("signer address mismatch")
			}
			return types.SignTx(tx, signer, privateKey)
		},
	}, nil
}

func encodeWithdrawID(withdrawID string) ([32]byte, error) {
	var out [32]byte
	trimmed := strings.TrimSpace(withdrawID)
	if trimmed == "" {
		return out, fmt.Errorf("%w: withdraw id is required", errorsx.ErrInvalidArgument)
	}
	if len(trimmed) > 32 {
		return out, fmt.Errorf("%w: withdraw id exceeds bytes32", errorsx.ErrInvalidArgument)
	}
	copy(out[:], []byte(trimmed))
	return out, nil
}

func decimalStringToBaseUnits(raw string, decimals int32) (*big.Int, error) {
	value, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	base := value.Shift(decimals)
	if !base.Equal(base.Truncate(0)) {
		return nil, fmt.Errorf("%w: amount precision exceeds token decimals", errorsx.ErrInvalidArgument)
	}
	return base.BigInt(), nil
}

func (e *VaultWithdrawExecutor) client(chainID int64) (*ethclient.Client, VaultWithdrawChainConfig, error) {
	client, ok := e.clients[chainID]
	if !ok {
		return nil, VaultWithdrawChainConfig{}, fmt.Errorf("%w: withdraw executor chain %d not configured", errorsx.ErrForbidden, chainID)
	}
	chain, ok := e.chains[chainID]
	if !ok {
		return nil, VaultWithdrawChainConfig{}, fmt.Errorf("%w: withdraw executor chain %d not configured", errorsx.ErrForbidden, chainID)
	}
	return client, chain, nil
}

func mustParseWithdrawABI(raw string) abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(raw))
	if err != nil {
		panic(err)
	}
	return parsed
}
