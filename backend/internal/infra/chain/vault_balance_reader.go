package chain

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/shopspring/decimal"
	"github.com/xiaobao/rgperp/backend/internal/pkg/authx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type VaultBalanceReaderChainConfig struct {
	ChainID      int64
	ChainKey     string
	ChainName    string
	Asset        string
	RPCURL       string
	VaultAddress string
	TokenAddress string
}

type VaultBalanceSnapshot struct {
	ChainID      int64
	ChainKey     string
	ChainName    string
	Asset        string
	VaultAddress string
	Balance      string
}

type VaultBalanceReader struct {
	clients map[int64]*ethclient.Client
	chains  map[int64]VaultBalanceReaderChainConfig
}

func NewVaultBalanceReader(chains []VaultBalanceReaderChainConfig) (*VaultBalanceReader, error) {
	out := &VaultBalanceReader{
		clients: make(map[int64]*ethclient.Client, len(chains)),
		chains:  make(map[int64]VaultBalanceReaderChainConfig, len(chains)),
	}
	for _, chain := range chains {
		if chain.ChainID <= 0 || strings.TrimSpace(chain.RPCURL) == "" || strings.TrimSpace(chain.Asset) == "" {
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
		chain.Asset = strings.ToUpper(strings.TrimSpace(chain.Asset))
		chain.VaultAddress = vaultAddress
		chain.TokenAddress = tokenAddress
		out.clients[chain.ChainID] = client
		out.chains[chain.ChainID] = chain
	}
	return out, nil
}

func (r *VaultBalanceReader) Close() {
	if r == nil {
		return
	}
	for _, client := range r.clients {
		client.Close()
	}
}

func (r *VaultBalanceReader) ListVaultBalances(ctx context.Context, scopeAsset string) ([]VaultBalanceSnapshot, error) {
	if r == nil {
		return nil, nil
	}
	scopeAsset = strings.ToUpper(strings.TrimSpace(scopeAsset))
	out := make([]VaultBalanceSnapshot, 0, len(r.chains))
	for chainID, chain := range r.chains {
		if scopeAsset != "" && scopeAsset != "ALL" && chain.Asset != scopeAsset {
			continue
		}
		client := r.clients[chainID]
		if client == nil {
			return nil, fmt.Errorf("%w: missing vault client for chain %d", errorsx.ErrInvalidArgument, chainID)
		}
		input, err := erc20BalanceABI.Pack("balanceOf", common.HexToAddress(chain.VaultAddress))
		if err != nil {
			return nil, err
		}
		result, err := client.CallContract(ctx, ethereum.CallMsg{
			To:   ptr(common.HexToAddress(chain.TokenAddress)),
			Data: input,
		}, nil)
		if err != nil {
			return nil, err
		}
		values, err := erc20BalanceABI.Unpack("balanceOf", result)
		if err != nil {
			return nil, err
		}
		balance, ok := values[0].(*big.Int)
		if !ok {
			return nil, fmt.Errorf("%w: vault balance decode failed", errorsx.ErrInvalidArgument)
		}
		out = append(out, VaultBalanceSnapshot{
			ChainID:      chain.ChainID,
			ChainKey:     chain.ChainKey,
			ChainName:    chain.ChainName,
			Asset:        chain.Asset,
			VaultAddress: chain.VaultAddress,
			Balance:      decimal.NewFromBigInt(balance, -6).String(),
		})
	}
	return out, nil
}
