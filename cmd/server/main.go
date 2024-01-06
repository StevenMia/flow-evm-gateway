package main

import (
	"context"
	"flag"
	"fmt"
	"runtime"

	goGrpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/onflow/flow-evm-gateway/api"
	"github.com/onflow/flow-evm-gateway/storage"
	"github.com/onflow/flow-go-sdk"
	"github.com/onflow/flow-go-sdk/access/grpc"
	"github.com/rs/zerolog"
)

const (
	accessURL    = "access-001.devnet49.nodes.onflow.org:9000"
	coinbaseAddr = "0xf02c1c8e6114b1dbe8937a39260b5b0a374432bb"
)

func main() {
	var network, coinbase string

	flag.StringVar(&network, "network", "testnet", "network to connect the gateway to")
	flag.StringVar(&coinbase, "coinbase", coinbaseAddr, "coinbase address to use for fee collection")
	flag.Parse()

	config := &api.Config{}
	config.Coinbase = common.HexToAddress(coinbase)
	if network == "testnet" {
		config.ChainID = api.FlowEVMTestnetChainID
	} else if network == "mainnet" {
		config.ChainID = api.FlowEVMMainnetChainID
	} else {
		panic(fmt.Errorf("unknown network: %s", network))
	}

	store := storage.NewStore()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Logger()
	runServer(config, store, logger)
	runIndexer(ctx, store, logger)

	runtime.Goexit()
}

func runIndexer(ctx context.Context, store *storage.Store, logger zerolog.Logger) {
	flowClient, err := grpc.NewBaseClient(
		accessURL,
		goGrpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		panic(err)
	}

	// TODO(m-Peter) The starting height from which the indexer should
	// begins, should either be retrieved from storage (latest height + 1),
	// or should be specified through a command-line flag (when starting
	// from scratch).
	latestBlockHeader, err := flowClient.GetLatestBlockHeader(ctx, true)
	if err != nil {
		panic(err)
	}
	logger.Info().Msgf("Latest Block Height: %d", latestBlockHeader.Height)
	logger.Info().Msgf("Latest Block ID: %s", latestBlockHeader.ID)

	data, errChan, initErr := flowClient.SubscribeEventsByBlockHeight(
		ctx,
		latestBlockHeader.Height,
		flow.EventFilter{
			Contracts: []string{"A.7e60df042a9c0868.FlowToken"},
		},
		grpc.WithHeartbeatInterval(1),
	)
	if initErr != nil {
		logger.Error().Msgf("could not subscribe to events: %v", initErr)
	}

	reconnect := func(height uint64) {
		logger.Warn().Msgf("Reconnecting at block height: %d", height)

		var err error
		flowClient, err := grpc.NewBaseClient(
			accessURL,
			goGrpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			logger.Error().Msgf("could not create flow client: %v", err)
		}

		data, errChan, initErr = flowClient.SubscribeEventsByBlockHeight(
			ctx,
			latestBlockHeader.Height,
			flow.EventFilter{
				Contracts: []string{"A.7e60df042a9c0868.FlowToken"},
			},
			grpc.WithHeartbeatInterval(1),
		)
		if initErr != nil {
			logger.Error().Msgf("could not subscribe to events: %v", initErr)
		}
	}

	// track the most recently seen block height. we will use this when reconnecting
	// the first response should be for latestBlockHeader.Height
	lastHeight := latestBlockHeader.Height - 1
	for {
		select {
		case <-ctx.Done():
			return

		case response, ok := <-data:
			if !ok {
				if ctx.Err() != nil {
					return // graceful shutdown
				}
				logger.Error().Msg("subscription closed - reconnecting")
				reconnect(lastHeight + 1)
				continue
			}

			if response.Height != lastHeight+1 {
				logger.Error().Msgf("missed events response for block %d", lastHeight+1)
				reconnect(lastHeight)
				continue
			}

			logger.Info().Msgf("block %d %s:", response.Height, response.BlockID)
			if len(response.Events) > 0 {
				store.StoreBlockHeight(ctx, response.Height)
			}
			for _, event := range response.Events {
				logger.Info().Msgf("  %s", event.Type)
			}

			lastHeight = response.Height

		case err, ok := <-errChan:
			if !ok {
				if ctx.Err() != nil {
					return // graceful shutdown
				}
				// unexpected close
				reconnect(lastHeight + 1)
				continue
			}

			logger.Error().Msgf("ERROR: %v", err)
			reconnect(lastHeight + 1)
			continue
		}
	}
}

func runServer(config *api.Config, store *storage.Store, logger zerolog.Logger) {
	srv := api.NewHTTPServer(logger, rpc.DefaultHTTPTimeouts)
	supportedAPIs := api.SupportedAPIs(config, store)

	srv.EnableRPC(supportedAPIs)
	srv.EnableWS(supportedAPIs)

	srv.SetListenAddr("localhost", 8545)

	err := srv.Start()
	if err != nil {
		panic(err)
	}
	logger.Info().Msgf("Server Started: %s", srv.ListenAddr())
}
