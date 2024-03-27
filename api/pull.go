package api

import (
	"context"
	"errors"
	"fmt"
	gethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/filters"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/onflow/flow-evm-gateway/config"
	"github.com/onflow/flow-evm-gateway/models"
	"github.com/onflow/flow-evm-gateway/services/logs"
	"github.com/onflow/flow-evm-gateway/storage"
	errs "github.com/onflow/flow-evm-gateway/storage/errors"
	"github.com/onflow/flow-go/fvm/evm/types"
	"github.com/rs/zerolog"
	"math/big"
	"sync"
	"time"
)

// filterExpiry defines the max number of block heights a filter
// can be idle for before being removed from existing filters.
const filterExpiry = time.Minute * 5

// filter defines a general resource filter that is used when pulling new data.
//
// Filters work by remembering the last height at which the data was pulled
// and allow on next request to fetch all data that is missing.
// Each filter is identified by a unique ID.
type filter interface {
	id() rpc.ID
	lastUsedHeight() uint64
	updateUsed(height uint64)
	expired() bool
}

// baseFilter is a base implementation that keeps track of
// last height and filter unique ID.
type baseFilter struct {
	last  uint64
	rpcID rpc.ID
	used  time.Time
}

func (h *baseFilter) id() rpc.ID {
	return h.rpcID
}

func (h *baseFilter) lastUsedHeight() uint64 {
	return h.last
}

// updateUsed updates the latest height the filter was used with as well as
// resets the time expiry for the filter.
func (h *baseFilter) updateUsed(height uint64) {
	h.last = height
	h.used = time.Now()
}

func (h *baseFilter) expired() bool {
	return time.Since(h.used) > filterExpiry
}

var _ filter = &blocksFilter{}
var _ filter = &transactionsFilter{}

// blocksFilter is used to get all new blocks since the last request
type blocksFilter struct {
	*baseFilter
}

func newBlocksFilter(lastHeight uint64) *blocksFilter {
	return &blocksFilter{
		&baseFilter{
			last:  lastHeight,
			rpcID: rpc.NewID(),
		},
	}
}

// transactionFilter is used to get all new transactions since last request.
//
// FullTx parameters determines if the result will include only
// hashes or full transaction body.
type transactionsFilter struct {
	*baseFilter
	fullTx bool
}

func newTransactionFilter(lastHeight uint64, fullTx bool) *transactionsFilter {
	return &transactionsFilter{
		&baseFilter{
			last:  lastHeight,
			rpcID: rpc.NewID(),
		},
		fullTx,
	}
}

// logsFilter is used to get all new logs since the last request.
//
// Criteria parameter filters the logs according to the criteria values.
type logsFilter struct {
	*baseFilter
	criteria *filters.FilterCriteria
}

func newLogsFilter(lastHeight uint64, criteria *filters.FilterCriteria) *logsFilter {
	return &logsFilter{
		&baseFilter{
			last:  lastHeight,
			rpcID: rpc.NewID(),
		},
		criteria,
	}
}

type PullAPI struct {
	logger       zerolog.Logger
	config       *config.Config
	blocks       storage.BlockIndexer
	transactions storage.TransactionIndexer
	receipts     storage.ReceiptIndexer
	filters      map[rpc.ID]filter
	mux          sync.RWMutex
}

func NewPullAPI(
	logger zerolog.Logger,
	config *config.Config,
	blocks storage.BlockIndexer,
	transactions storage.TransactionIndexer,
	receipts storage.ReceiptIndexer,
) *PullAPI {
	api := &PullAPI{
		logger:       logger,
		config:       config,
		blocks:       blocks,
		transactions: transactions,
		receipts:     receipts,
		filters:      make(map[rpc.ID]filter),
	}

	go api.idleFilterChecker()

	return api
}

// NewPendingTransactionFilter creates a filter that fetches pending transactions
// as transactions enter the pending state.
//
// It is part of the filter package because this filter can be used through the
// `eth_getFilterChanges` polling method that is also used for log filters.
func (api *PullAPI) NewPendingTransactionFilter(fullTx *bool) (rpc.ID, error) {
	last, err := api.blocks.LatestEVMHeight()
	if err != nil {
		return "", err
	}

	full := false
	if fullTx != nil && *fullTx {
		full = true
	}

	return api.addFilter(newTransactionFilter(last, full)), nil
}

// NewBlockFilter creates a filter that fetches blocks that are imported into the chain.
// It is part of the filter package since polling goes with eth_getFilterChanges.
func (api *PullAPI) NewBlockFilter() (rpc.ID, error) {
	last, err := api.blocks.LatestEVMHeight()
	if err != nil {
		return "", err
	}

	return api.addFilter(newBlocksFilter(last)), nil
}

func (api *PullAPI) UninstallFilter(id rpc.ID) bool {
	api.mux.Lock()
	defer api.mux.Unlock()

	_, ok := api.filters[id]
	delete(api.filters, id)
	return ok
}

// NewFilter creates a new filter and returns the filter id. It can be
// used to retrieve logs when the state changes. This method cannot be
// used to fetch logs that are already stored in the state.
//
// Default criteria for the from and to block are "latest".
// Using "latest" as block number will return logs for mined blocks.
// Using "pending" as block number returns logs for not yet mined (pending) blocks.
// In case logs are removed (chain reorg) previously returned logs are returned
// again but with the removed property set to true.
//
// In case "fromBlock" > "toBlock" an error is returned.
func (api *PullAPI) NewFilter(criteria filters.FilterCriteria) (rpc.ID, error) {
	from, err := api.blocks.LatestEVMHeight()
	if err != nil {
		return "", err
	}

	// if from block is actually set we use it as from value otherwise use latest
	if criteria.FromBlock != nil && criteria.FromBlock.Int64() >= 0 {
		from = criteria.FromBlock.Uint64()
		// if to block is set make sure it's not less than from block
		if criteria.ToBlock != nil && criteria.FromBlock.Cmp(criteria.ToBlock) > 0 {
			return "", fmt.Errorf("from block must be lower than to block")
		}
		// todo we should check for max range of from-to heights
	}

	return api.addFilter(newLogsFilter(from, &criteria)), nil
}

// GetFilterLogs returns the logs for the filter with the given id.
// If the filter could not be found an empty array of logs is returned.
func (api *PullAPI) GetFilterLogs(ctx context.Context, id rpc.ID) ([]*gethTypes.Log, error) {
	panic("not implemented")
}

// GetFilterChanges returns the logs for the filter with the given id since
// last time it was called. This can be used for polling.
//
// For pending transaction and block filters the result is []common.Hash.
// (pending)Log filters return []Log.
func (api *PullAPI) GetFilterChanges(id rpc.ID) (interface{}, error) {
	api.mux.RLock()
	defer api.mux.RUnlock()

	f, ok := api.filters[id]
	if !ok {
		return nil, errors.Join(errs.ErrNotFound, fmt.Errorf("filted by id %s does not exist", id))
	}

	current, err := api.blocks.LatestEVMHeight()
	if err != nil {
		return nil, err
	}

	if f.expired() {
		api.UninstallFilter(id)
		return nil, errors.Join(errs.ErrNotFound, fmt.Errorf("filted by id %s expired", id))
	}
	defer f.updateUsed(current)

	switch f.(type) {
	case *blocksFilter:
		return api.getBlocks(current, f.(*blocksFilter))
	case *transactionsFilter:
		return api.getTransactions(current, f.(*transactionsFilter))
	case *logsFilter:
		return api.getLogs(current, f.(*logsFilter))
	}

	return nil, nil
}

// idleFilterChecker continuously monitors all the registered filters and
// if any of them has been idle for too long i.e. no requests have been made
// for a period defined as filterExpiry using the filter ID it will be removed.
func (api *PullAPI) idleFilterChecker() {
	for _ = range time.Tick(time.Minute) {
		for id, f := range api.filters {
			if f.expired() {
				api.UninstallFilter(id)
			}
		}
	}
}

func (api *PullAPI) addFilter(f filter) rpc.ID {
	api.mux.Lock()
	defer api.mux.Unlock()

	// we limit max active filters at any time to prevent abuse and OOM
	const maxFilters = 10_000
	if len(api.filters) > maxFilters {
		return ""
	}

	api.filters[f.id()] = f
	return f.id()
}

func (api *PullAPI) getBlocks(latestHeight uint64, filter *blocksFilter) ([]*types.Block, error) {
	blocks := make([]*types.Block, latestHeight-filter.lastUsedHeight())

	// todo we can optimize if needed by adding a getter method for range of blocks
	for i := filter.last; i <= latestHeight; i++ {
		b, err := api.blocks.GetByHeight(i)
		if err != nil {
			return nil, err
		}
		blocks[i] = b
	}

	return blocks, nil
}

func (api *PullAPI) getTransactions(latestHeight uint64, filter *transactionsFilter) ([]models.Transaction, error) {
	txs := make([]models.Transaction, 0)

	// todo we can optimize if needed by adding a getter method for range of txs by heights
	for i := filter.last; i <= latestHeight; i++ {
		b, err := api.blocks.GetByHeight(i)
		if err != nil {
			return nil, err
		}

		// for now there will only be one tx per block
		for _, h := range b.TransactionHashes {
			tx, err := api.transactions.Get(h)
			if err != nil {
				return nil, err
			}
			txs = append(txs, tx)
		}
	}

	return txs, nil
}

func (api *PullAPI) getLogs(latestHeight uint64, filter *logsFilter) (any, error) {
	last := big.NewInt(int64(filter.last))
	latest := big.NewInt(int64(latestHeight))
	criteria := logs.FilterCriteria{
		Addresses: filter.criteria.Addresses,
		Topics:    filter.criteria.Topics,
	}

	// if filter criteria "to" block is lower height than current height there can not be any new
	// logs available, so we return nil, if "to" is negative it means latest, so we always return new data
	to := filter.criteria.ToBlock
	if to != nil && latest.Cmp(to) > 0 && to.Cmp(big.NewInt(0)) >= 0 {
		return nil, nil
	}

	f, err := logs.NewRangeFilter(*last, *latest, criteria, api.receipts)
	if err != nil {
		return nil, err
	}

	return f.Match()
}
