package proposer

import (
	"context"
	"math/big"

	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/sources/caching"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/stateless"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
)

type L1Client interface {
	BlockNumber(ctx context.Context) (uint64, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error)
	BlockReceipts(ctx context.Context, hash common.Hash) (types.Receipts, error)
	CodeAt(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error)
	CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
	Close()
}

type L2Client interface {
	ChainConfig(ctx context.Context) (*params.ChainConfig, error)
	GetProof(ctx context.Context, address common.Address, hash common.Hash) (*eth.AccountResult, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error)
	BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error)
	ExecutionWitness(ctx context.Context, hash common.Hash) (*stateless.ExecutionWitness, error)
	Close()
}

type Client interface {
	L1Client
	L2Client
}

type RollupClient interface {
	RollupConfig(ctx context.Context) (*rollup.Config, error)
	SyncStatus(ctx context.Context) (*eth.SyncStatus, error)
}

type ethClient struct {
	client        *ethclient.Client
	blocksCache   *caching.LRUCache[common.Hash, *types.Block]
	headersCache  *caching.LRUCache[common.Hash, *types.Header]
	receiptsCache *caching.LRUCache[common.Hash, types.Receipts]
	proofsCache   *caching.LRUCache[[common.AddressLength + common.HashLength]byte, *eth.AccountResult]
}

func NewClient(client *ethclient.Client, metrics caching.Metrics) Client {
	return &ethClient{
		client:        client,
		blocksCache:   caching.NewLRUCache[common.Hash, *types.Block](metrics, "blocks", 1000),
		headersCache:  caching.NewLRUCache[common.Hash, *types.Header](metrics, "headers", 1000),
		receiptsCache: caching.NewLRUCache[common.Hash, types.Receipts](metrics, "receipts", 1000),
		proofsCache:   caching.NewLRUCache[[common.AddressLength + common.HashLength]byte, *eth.AccountResult](metrics, "proofs", 1000),
	}
}

func (e *ethClient) ChainConfig(ctx context.Context) (*params.ChainConfig, error) {
	var config params.ChainConfig
	if err := e.client.Client().CallContext(ctx, &config, "debug_chainConfig"); err != nil {
		return nil, err
	}
	return &config, nil
}

func (e *ethClient) BlockNumber(ctx context.Context) (uint64, error) {
	return e.client.BlockNumber(ctx)
}

func (e *ethClient) getHeader(ctx context.Context, fetchFunc func() (*types.Header, error), key common.Hash) (*types.Header, error) {
	if header, ok := e.headersCache.Get(key); ok {
		return header, nil
	}
	header, err := fetchFunc()
	if err != nil {
		return nil, err
	}
	e.headersCache.Add(key, header)
	return header, nil
}

func (e *ethClient) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	return e.getHeader(ctx, func() (*types.Header, error) {
		return e.client.HeaderByHash(ctx, hash)
	}, hash)
}

func (e *ethClient) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	header, err := e.client.HeaderByNumber(ctx, number)
	if err != nil {
		return nil, err
	}
	e.headersCache.Add(header.Hash(), header)
	return header, nil
}

func (e *ethClient) getBlock(ctx context.Context, fetchFunc func() (*types.Block, error), key common.Hash) (*types.Block, error) {
	if block, ok := e.blocksCache.Get(key); ok {
		return block, nil
	}
	block, err := fetchFunc()
	if err != nil {
		return nil, err
	}
	e.blocksCache.Add(block.Hash(), block)
	e.headersCache.Add(block.Hash(), block.Header())
	return block, nil
}

func (e *ethClient) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	return e.getBlock(ctx, func() (*types.Block, error) {
		return e.client.BlockByNumber(ctx, number)
	}, number.Bytes32())
}

func (e *ethClient) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	return e.getBlock(ctx, func() (*types.Block, error) {
		return e.client.BlockByHash(ctx, hash)
	}, hash)
}

func (e *ethClient) BlockReceipts(ctx context.Context, hash common.Hash) (types.Receipts, error) {
	if receipts, ok := e.receiptsCache.Get(hash); ok {
		return receipts, nil
	}
	receipts, err := e.client.BlockReceipts(ctx, rpc.BlockNumberOrHash{BlockHash: &hash})
	if err != nil {
		return nil, err
	}
	e.receiptsCache.Add(hash, receipts)
	return receipts, nil
}

func (e *ethClient) Close() { e.client.Close() }

type rollupClient struct {
	client       *rpc.Client
	witnessCache *caching.LRUCache[common.Hash, []byte]
}

func NewRollupClient(client *rpc.Client, metrics caching.Metrics) RollupClient {
	return &rollupClient{
		client:       client,
		witnessCache: caching.NewLRUCache[common.Hash, []byte](metrics, "witnesses", 1000),
	}
}

func (w *rollupClient) RollupConfig(ctx context.Context) (*rollup.Config, error) {
	var config rollup.Config
	if err := w.client.CallContext(ctx, &config, "optimism_rollupConfig"); err != nil {
		return nil, err
	}
	return &config, nil
}

func (w *rollupClient) SyncStatus(ctx context.Context) (*eth.SyncStatus, error) {
	var status eth.SyncStatus
	if err := w.client.CallContext(ctx, &status, "optimism_syncStatus"); err != nil {
		return nil, err
	}
	return &status, nil
}
