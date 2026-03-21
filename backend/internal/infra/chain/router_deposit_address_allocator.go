package chain

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

var depositRouterFactoryABI = mustParseContractABI(`[
	{"inputs":[{"internalType":"uint256","name":"userId","type":"uint256"}],"name":"routerOfUser","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
	{"inputs":[{"internalType":"uint256","name":"userId","type":"uint256"},{"internalType":"bytes32","name":"salt","type":"bytes32"}],"name":"createRouter","outputs":[{"internalType":"address","name":"router","type":"address"}],"stateMutability":"nonpayable","type":"function"}
]`)

type RouterAllocatorChainConfig struct {
	ChainID        int64
	RPCURL         string
	FactoryAddress string
}

type RouterDepositAddressAllocator struct {
	privateKey *ecdsa.PrivateKey
	chains     map[int64]RouterAllocatorChainConfig
	clients    map[int64]*ethclient.Client
	fallback   *DeterministicDepositAddressAllocator
}

func NewRouterDepositAddressAllocator(privateKeyHex string, chains []RouterAllocatorChainConfig) (*RouterDepositAddressAllocator, error) {
	keyHex := strings.TrimSpace(strings.TrimPrefix(privateKeyHex, "0x"))
	if keyHex == "" {
		return &RouterDepositAddressAllocator{fallback: NewDeterministicDepositAddressAllocator()}, nil
	}
	privateKey, err := crypto.HexToECDSA(keyHex)
	if err != nil {
		return nil, err
	}

	out := &RouterDepositAddressAllocator{
		privateKey: privateKey,
		chains:     make(map[int64]RouterAllocatorChainConfig, len(chains)),
		clients:    make(map[int64]*ethclient.Client, len(chains)),
		fallback:   NewDeterministicDepositAddressAllocator(),
	}
	for _, chain := range chains {
		if chain.ChainID <= 0 || strings.TrimSpace(chain.RPCURL) == "" || strings.TrimSpace(chain.FactoryAddress) == "" {
			continue
		}
		client, err := ethclient.Dial(chain.RPCURL)
		if err != nil {
			return nil, err
		}
		out.chains[chain.ChainID] = chain
		out.clients[chain.ChainID] = client
	}
	return out, nil
}

func (a *RouterDepositAddressAllocator) Close() {
	for _, client := range a.clients {
		client.Close()
	}
}

func (a *RouterDepositAddressAllocator) Allocate(ctx context.Context, userID uint64, chainID int64, asset string) (string, error) {
	chainCfg, ok := a.chains[chainID]
	if !ok || a.privateKey == nil {
		return a.fallback.Allocate(ctx, userID, chainID, asset)
	}
	client := a.clients[chainID]
	factoryAddress := common.HexToAddress(chainCfg.FactoryAddress)
	contract := bind.NewBoundContract(factoryAddress, depositRouterFactoryABI, client, client, client)

	router, err := readRouterOfUser(ctx, contract, userID)
	if err != nil {
		return "", err
	}
	if router != (common.Address{}) {
		return router.Hex(), nil
	}

	auth, err := bind.NewKeyedTransactorWithChainID(a.privateKey, big.NewInt(chainID))
	if err != nil {
		return "", err
	}
	auth.Context = ctx

	salt := crypto.Keccak256Hash([]byte(fmt.Sprintf("rgperp:user:%d", userID)))
	tx, err := contract.Transact(auth, "createRouter", big.NewInt(int64(userID)), salt)
	if err != nil {
		return "", err
	}
	receipt, err := bind.WaitMined(ctx, client, tx)
	if err != nil {
		return "", err
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return "", fmt.Errorf("create router reverted: %s", tx.Hash().Hex())
	}
	router, err = readRouterOfUser(ctx, contract, userID)
	if err != nil {
		return "", err
	}
	if router == (common.Address{}) {
		return "", fmt.Errorf("router not created for user %d", userID)
	}
	return router.Hex(), nil
}

func readRouterOfUser(ctx context.Context, contract *bind.BoundContract, userID uint64) (common.Address, error) {
	var out []interface{}
	if err := contract.Call(&bind.CallOpts{Context: ctx}, &out, "routerOfUser", big.NewInt(int64(userID))); err != nil {
		return common.Address{}, err
	}
	if len(out) == 0 {
		return common.Address{}, nil
	}
	switch value := out[0].(type) {
	case common.Address:
		return value, nil
	case [20]byte:
		return common.BytesToAddress(value[:]), nil
	default:
		return common.Address{}, fmt.Errorf("unexpected routerOfUser return type %T", out[0])
	}
}

func mustParseContractABI(raw string) abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(raw))
	if err != nil {
		panic(err)
	}
	return parsed
}
