package chain

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	indexerdomain "github.com/xiaobao/rgperp/backend/internal/domain/indexer"
	"github.com/shopspring/decimal"
)

var (
	depositRouterABI  = mustParseABI(`[{"anonymous":false,"inputs":[{"indexed":true,"internalType":"uint256","name":"userId","type":"uint256"},{"indexed":true,"internalType":"address","name":"token","type":"address"},{"indexed":false,"internalType":"uint256","name":"amount","type":"uint256"},{"indexed":true,"internalType":"address","name":"from","type":"address"},{"indexed":false,"internalType":"address","name":"vault","type":"address"}],"name":"DepositForwarded","type":"event"}]`)
	depositFactoryABI = mustParseABI(`[{"anonymous":false,"inputs":[{"indexed":true,"internalType":"uint256","name":"userId","type":"uint256"},{"indexed":true,"internalType":"address","name":"router","type":"address"},{"indexed":true,"internalType":"bytes32","name":"salt","type":"bytes32"}],"name":"RouterCreated","type":"event"}]`)
	vaultABI          = mustParseABI(`[{"anonymous":false,"inputs":[{"indexed":true,"internalType":"bytes32","name":"withdrawId","type":"bytes32"},{"indexed":true,"internalType":"address","name":"token","type":"address"},{"indexed":true,"internalType":"address","name":"to","type":"address"},{"indexed":false,"internalType":"uint256","name":"amount","type":"uint256"},{"indexed":false,"internalType":"address","name":"operator","type":"address"}],"name":"WithdrawExecuted","type":"event"}]`)

	depositForwardedEvent = depositRouterABI.Events["DepositForwarded"]
	routerCreatedEvent    = depositFactoryABI.Events["RouterCreated"]
	withdrawExecutedEvent = vaultABI.Events["WithdrawExecuted"]
)

type EVMChainConfig struct {
	ChainID        int64
	RPCURL         string
	VaultAddress   string
	TokenAddress   string
	FactoryAddress string
}

type EVMEventSource struct {
	clients map[int64]*ethclient.Client
	cfgs    map[int64]EVMChainConfig
}

func NewEVMEventSource(configs []EVMChainConfig) (*EVMEventSource, error) {
	clients := make(map[int64]*ethclient.Client, len(configs))
	cfgMap := make(map[int64]EVMChainConfig, len(configs))
	for _, cfg := range configs {
		if cfg.ChainID <= 0 || strings.TrimSpace(cfg.RPCURL) == "" {
			continue
		}
		client, err := ethclient.Dial(cfg.RPCURL)
		if err != nil {
			for _, existing := range clients {
				existing.Close()
			}
			return nil, err
		}
		clients[cfg.ChainID] = client
		cfgMap[cfg.ChainID] = cfg
	}
	return &EVMEventSource{clients: clients, cfgs: cfgMap}, nil
}

func (s *EVMEventSource) Close() {
	for _, client := range s.clients {
		client.Close()
	}
}

func (s *EVMEventSource) LatestBlockNumber(ctx context.Context, chainID int64) (int64, error) {
	client, err := s.client(chainID)
	if err != nil {
		return 0, err
	}
	block, err := client.BlockNumber(ctx)
	if err != nil {
		return 0, err
	}
	return int64(block), nil
}

func (s *EVMEventSource) ListRouterCreatedEvents(ctx context.Context, chainID int64, fromBlock int64, toBlock int64) ([]indexerdomain.RouterCreated, error) {
	cfg, client, err := s.chain(chainID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.FactoryAddress) == "" {
		return nil, nil
	}
	logs, err := client.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: big.NewInt(fromBlock),
		ToBlock:   big.NewInt(toBlock),
		Addresses: []common.Address{common.HexToAddress(cfg.FactoryAddress)},
		Topics:    [][]common.Hash{{routerCreatedEvent.ID}},
	})
	if err != nil {
		return nil, err
	}
	out := make([]indexerdomain.RouterCreated, 0, len(logs))
	for _, lg := range logs {
		if len(lg.Topics) < 4 {
			continue
		}
		out = append(out, indexerdomain.RouterCreated{
			ChainID:        chainID,
			UserID:         hashToUint64(lg.Topics[1]),
			RouterAddress:  common.BytesToAddress(lg.Topics[2].Bytes()[12:]).Hex(),
			FactoryAddress: lg.Address.Hex(),
			Salt:           lg.Topics[3].Hex(),
			TxHash:         lg.TxHash.Hex(),
			LogIndex:       int64(lg.Index),
			BlockNumber:    int64(lg.BlockNumber),
		})
	}
	return out, nil
}

func (s *EVMEventSource) ListDepositEvents(ctx context.Context, chainID int64, fromBlock int64, toBlock int64) ([]indexerdomain.DepositObserved, error) {
	_, client, err := s.chain(chainID)
	if err != nil {
		return nil, err
	}
	logs, err := client.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: big.NewInt(fromBlock),
		ToBlock:   big.NewInt(toBlock),
		Topics:    [][]common.Hash{{depositForwardedEvent.ID}},
	})
	if err != nil {
		return nil, err
	}
	out := make([]indexerdomain.DepositObserved, 0, len(logs))
	for _, lg := range logs {
		event, err := decodeDepositForwarded(lg)
		if err != nil {
			return nil, err
		}
		event.ChainID = chainID
		out = append(out, event)
	}
	return out, nil
}

func (s *EVMEventSource) ListWithdrawEvents(ctx context.Context, chainID int64, fromBlock int64, toBlock int64) ([]indexerdomain.WithdrawExecuted, error) {
	cfg, client, err := s.chain(chainID)
	if err != nil {
		return nil, err
	}
	logs, err := client.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: big.NewInt(fromBlock),
		ToBlock:   big.NewInt(toBlock),
		Addresses: []common.Address{common.HexToAddress(cfg.VaultAddress)},
		Topics:    [][]common.Hash{{withdrawExecutedEvent.ID}},
	})
	if err != nil {
		return nil, err
	}
	out := make([]indexerdomain.WithdrawExecuted, 0, len(logs))
	for _, lg := range logs {
		event, err := decodeWithdrawExecuted(lg)
		if err != nil {
			return nil, err
		}
		event.ChainID = chainID
		out = append(out, event)
	}
	return out, nil
}

func (s *EVMEventSource) GetReceiptStatus(ctx context.Context, chainID int64, txHash string) (indexerdomain.ReceiptStatus, error) {
	client, err := s.client(chainID)
	if err != nil {
		return indexerdomain.ReceiptStatus{}, err
	}
	receipt, err := client.TransactionReceipt(ctx, common.HexToHash(txHash))
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			return indexerdomain.ReceiptStatus{Found: false}, nil
		}
		return indexerdomain.ReceiptStatus{}, err
	}
	return indexerdomain.ReceiptStatus{
		Found:       true,
		Success:     receipt.Status == types.ReceiptStatusSuccessful,
		BlockNumber: receipt.BlockNumber.Int64(),
	}, nil
}

func (s *EVMEventSource) chain(chainID int64) (EVMChainConfig, *ethclient.Client, error) {
	client, err := s.client(chainID)
	if err != nil {
		return EVMChainConfig{}, nil, err
	}
	cfg, ok := s.cfgs[chainID]
	if !ok {
		return EVMChainConfig{}, nil, fmt.Errorf("chain config not found: %d", chainID)
	}
	return cfg, client, nil
}

func (s *EVMEventSource) client(chainID int64) (*ethclient.Client, error) {
	client, ok := s.clients[chainID]
	if !ok {
		return nil, fmt.Errorf("chain client not found: %d", chainID)
	}
	return client, nil
}

func decodeDepositForwarded(lg types.Log) (indexerdomain.DepositObserved, error) {
	if len(lg.Topics) < 4 {
		return indexerdomain.DepositObserved{}, fmt.Errorf("deposit log topics missing")
	}
	values, err := depositForwardedEvent.Inputs.NonIndexed().Unpack(lg.Data)
	if err != nil {
		return indexerdomain.DepositObserved{}, err
	}
	amount, ok := values[0].(*big.Int)
	if !ok {
		return indexerdomain.DepositObserved{}, fmt.Errorf("deposit amount decode failed")
	}
	vault, ok := values[1].(common.Address)
	if !ok {
		return indexerdomain.DepositObserved{}, fmt.Errorf("deposit vault decode failed")
	}
	return indexerdomain.DepositObserved{
		UserID:        hashToUint64(lg.Topics[1]),
		TxHash:        lg.TxHash.Hex(),
		LogIndex:      int64(lg.Index),
		BlockNumber:   int64(lg.BlockNumber),
		RouterAddress: lg.Address.Hex(),
		VaultAddress:  vault.Hex(),
		TokenAddress:  common.BytesToAddress(lg.Topics[2].Bytes()[12:]).Hex(),
		FromAddress:   common.BytesToAddress(lg.Topics[3].Bytes()[12:]).Hex(),
		Amount:        baseUnitsToDecimalString(amount, 6),
		Removed:       lg.Removed,
	}, nil
}

func decodeWithdrawExecuted(lg types.Log) (indexerdomain.WithdrawExecuted, error) {
	if len(lg.Topics) < 4 {
		return indexerdomain.WithdrawExecuted{}, fmt.Errorf("withdraw log topics missing")
	}
	values, err := withdrawExecutedEvent.Inputs.NonIndexed().Unpack(lg.Data)
	if err != nil {
		return indexerdomain.WithdrawExecuted{}, err
	}
	amount, ok := values[0].(*big.Int)
	if !ok {
		return indexerdomain.WithdrawExecuted{}, fmt.Errorf("withdraw amount decode failed")
	}
	operator, ok := values[1].(common.Address)
	if !ok {
		return indexerdomain.WithdrawExecuted{}, fmt.Errorf("withdraw operator decode failed")
	}
	return indexerdomain.WithdrawExecuted{
		WithdrawID:   decodeBytes32Identifier(lg.Topics[1]),
		TxHash:       lg.TxHash.Hex(),
		LogIndex:     int64(lg.Index),
		BlockNumber:  int64(lg.BlockNumber),
		VaultAddress: lg.Address.Hex(),
		TokenAddress: common.BytesToAddress(lg.Topics[2].Bytes()[12:]).Hex(),
		ToAddress:    common.BytesToAddress(lg.Topics[3].Bytes()[12:]).Hex(),
		Amount:       baseUnitsToDecimalString(amount, 6),
		Operator:     operator.Hex(),
	}, nil
}

func hashToUint64(hash common.Hash) uint64 {
	return hash.Big().Uint64()
}

func decodeBytes32Identifier(hash common.Hash) string {
	data := hash.Bytes()
	end := len(data)
	for end > 0 && data[end-1] == 0 {
		end--
	}
	if end > 0 {
		printable := true
		for _, b := range data[:end] {
			if b < 32 || b > 126 {
				printable = false
				break
			}
		}
		if printable {
			return string(data[:end])
		}
	}
	return hash.Hex()
}

func baseUnitsToDecimalString(value *big.Int, decimals int32) string {
	return decimal.NewFromBigInt(value, -decimals).String()
}

func mustParseABI(raw string) abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(raw))
	if err != nil {
		panic(err)
	}
	return parsed
}
