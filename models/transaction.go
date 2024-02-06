package models

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	gethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/onflow/cadence"
	cdcCommon "github.com/onflow/cadence/runtime/common"
	"github.com/onflow/flow-go/fvm/evm/types"
	"math/big"
)

type txEventPayload struct {
	BlockHeight             uint64 `cadence:"blockHeight"`
	TransactionHash         string `cadence:"transactionHash"`
	Transaction             string `cadence:"transaction"`
	TransactionType         int    `cadence:"transactionType"`
	Failed                  bool   `cadence:"failed"`
	GasConsumed             uint64 `cadence:"gasConsumed"`
	DeployedContractAddress string `cadence:"deployedContractAddress"`
	ReturnedValue           string `cadence:"returnedValue"`
	Logs                    string `cadence:"logs"`
}

// DecodeReceipt takes a cadence event for transaction executed and decodes it into the receipt.
func DecodeReceipt(event cadence.Event) (*gethTypes.Receipt, error) {
	if !IsTransactionExecutedEvent(event) {
		return nil, fmt.Errorf(
			"invalid event type for decoding into receipt, received %s expected %s",
			event.Type().ID(),
			types.EventTypeTransactionExecuted,
		)
	}

	var tx txEventPayload
	err := cadence.DecodeFields(event, &tx)
	if err != nil {
		return nil, fmt.Errorf("failed to cadence decode receipt: %w", err)
	}

	encLogs, err := hex.DecodeString(tx.Logs)
	if err != nil {
		return nil, fmt.Errorf("failed to hex decode receipt: %w", err)
	}

	var logs []*gethTypes.Log
	if len(encLogs) > 0 {
		err = rlp.Decode(bytes.NewReader(encLogs), &logs)
		if err != nil {
			return nil, fmt.Errorf("failed to rlp decode receipt: %w", err)
		}
	}

	receipt := &gethTypes.Receipt{
		Type:              0,              // todo check
		Status:            0,              // todo check
		CumulativeGasUsed: tx.GasConsumed, // todo check
		Logs:              logs,
		TxHash:            common.HexToHash(tx.TransactionHash),
		ContractAddress:   common.HexToAddress(tx.DeployedContractAddress),
		GasUsed:           tx.GasConsumed,
		EffectiveGasPrice: nil,           // todo check
		BlobGasUsed:       0,             // todo check
		BlobGasPrice:      nil,           // todo check
		BlockHash:         common.Hash{}, // todo check
		BlockNumber:       big.NewInt(int64(tx.BlockHeight)),
		TransactionIndex:  0, // todo check
	}

	receipt.Bloom = gethTypes.CreateBloom([]*gethTypes.Receipt{receipt})

	return receipt, nil
}

// DecodeTransaction takes a cadence event for transaction executed and decodes it into the transaction.
func DecodeTransaction(event cadence.Event) (*gethTypes.Transaction, error) {
	if !IsTransactionExecutedEvent(event) {
		return nil, fmt.Errorf(
			"invalid event type for decoding into receipt, received %s expected %s",
			event.Type().ID(),
			types.EventTypeTransactionExecuted,
		)
	}

	var t txEventPayload
	err := cadence.DecodeFields(event, &t)
	if err != nil {
		return nil, fmt.Errorf("failed to cadence decode transaction: %w", err)
	}

	encTx, err := hex.DecodeString(t.Transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to decode transaction hex: %w", err)
	}

	// check if the transaction data is actually from a direct call, which is a special flow/evm state transition
	if encTx[0] == types.DirectCallTxType {
		var direct types.DirectCall
		err = rlp.DecodeBytes(encTx[1:], &direct) // strip type from data before rlp decoding
		if err != nil {
			return nil, fmt.Errorf("failed to rlp decode transaction direct call: %w", err)
		}

		// convert message to transaction
		msg := direct.Message()
		return gethTypes.NewTransaction(msg.Nonce, *msg.To, msg.Value, msg.GasLimit, msg.GasPrice, msg.Data), nil
	}

	tx := gethTypes.Transaction{}
	err = tx.DecodeRLP(rlp.NewStream(bytes.NewReader(encTx), uint64(len(encTx))))
	if err != nil {
		return nil, fmt.Errorf("failed to rlp decode transaction: %w", err)
	}

	return &tx, nil
}

func IsTransactionExecutedEvent(event cadence.Event) bool {
	txExecutedType := (types.EVMLocation{}).TypeID(nil, string(types.EventTypeTransactionExecuted))
	return cdcCommon.TypeID(event.EventType.ID()) == txExecutedType
}
