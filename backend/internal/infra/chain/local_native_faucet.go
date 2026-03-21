package chain

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/xiaobao/rgperp/backend/internal/pkg/authx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type LocalNativeFaucetChainConfig struct {
	ChainID int64
	RPCURL  string
}

type LocalNativeFaucet struct {
	privateKey *ecdsa.PrivateKey
	chains     map[int64]LocalNativeFaucetChainConfig
	clients    map[int64]*ethclient.Client
	amountWei  *big.Int
}

func NewLocalNativeFaucet(privateKeyHex string, chains []LocalNativeFaucetChainConfig, amountWei *big.Int) (*LocalNativeFaucet, error) {
	keyHex := strings.TrimSpace(strings.TrimPrefix(privateKeyHex, "0x"))
	if keyHex == "" {
		return nil, fmt.Errorf("%w: local native faucet private key is required", errorsx.ErrInvalidArgument)
	}
	privateKey, err := crypto.HexToECDSA(keyHex)
	if err != nil {
		return nil, err
	}
	if amountWei == nil || amountWei.Sign() <= 0 {
		return nil, fmt.Errorf("%w: local native faucet amount is required", errorsx.ErrInvalidArgument)
	}
	out := &LocalNativeFaucet{
		privateKey: privateKey,
		chains:     make(map[int64]LocalNativeFaucetChainConfig, len(chains)),
		clients:    make(map[int64]*ethclient.Client, len(chains)),
		amountWei:  new(big.Int).Set(amountWei),
	}
	for _, chain := range chains {
		if chain.ChainID <= 0 || strings.TrimSpace(chain.RPCURL) == "" {
			continue
		}
		client, err := ethclient.Dial(chain.RPCURL)
		if err != nil {
			out.Close()
			return nil, err
		}
		out.chains[chain.ChainID] = chain
		out.clients[chain.ChainID] = client
	}
	return out, nil
}

func (f *LocalNativeFaucet) Close() {
	for _, client := range f.clients {
		client.Close()
	}
}

func (f *LocalNativeFaucet) GrantNativeToken(ctx context.Context, address string, chainID int64) (string, error) {
	normalized, err := authx.NormalizeEVMAddress(address)
	if err != nil {
		return "", err
	}
	client, ok := f.clients[chainID]
	if !ok {
		return "", fmt.Errorf("%w: chain %d faucet not configured", errorsx.ErrForbidden, chainID)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(f.privateKey, big.NewInt(chainID))
	if err != nil {
		return "", err
	}
	auth.Context = ctx

	nonce, err := client.PendingNonceAt(ctx, auth.From)
	if err != nil {
		return "", err
	}
	gasTipCap, err := client.SuggestGasTipCap(ctx)
	if err != nil {
		return "", err
	}
	header, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		return "", err
	}
	gasFeeCap := new(big.Int).Add(gasTipCap, new(big.Int).Mul(header.BaseFee, big.NewInt(2)))
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   big.NewInt(chainID),
		Nonce:     nonce,
		GasTipCap: gasTipCap,
		GasFeeCap: gasFeeCap,
		Gas:       21_000,
		To:        ptr(common.HexToAddress(normalized)),
		Value:     new(big.Int).Set(f.amountWei),
	})
	signedTx, err := auth.Signer(auth.From, tx)
	if err != nil {
		return "", err
	}
	if err := client.SendTransaction(ctx, signedTx); err != nil {
		return "", err
	}
	return signedTx.Hash().Hex(), nil
}

func ptr[T any](value T) *T {
	return &value
}
