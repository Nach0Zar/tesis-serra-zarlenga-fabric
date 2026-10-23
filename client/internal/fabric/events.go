package fabric

import (
	"context"
	"fmt"

	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
)

// ChaincodeEvent contiene los metadatos públicos de un evento confirmado.
type ChaincodeEvent struct {
	BlockNumber   uint64
	TransactionID string
	EventName     string
	Payload       []byte
}

// InvalidTransaction representa una transacción incluida en un bloque pero
// marcada como inválida por la validación de los peers.
type InvalidTransaction struct {
	BlockNumber         uint64
	TransactionID       string
	TransactionType     string
	ValidationCode      string
	ValidationCodeValue int32
}

// ChaincodeEvents entrega eventos confirmados del chaincode configurado.
func (c *Client) ChaincodeEvents(
	ctx context.Context,
	startBlock *uint64,
) (<-chan ChaincodeEvent, error) {
	events, err := c.network.ChaincodeEvents(
		ctx,
		c.chaincode,
		chaincodeEventOptions(startBlock)...,
	)
	if err != nil {
		return nil, fmt.Errorf("subscribe chaincode events: %w", err)
	}

	result := make(chan ChaincodeEvent)
	go func() {
		defer close(result)
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-events:
				if !ok {
					return
				}
				converted := chaincodeEventFromGateway(event)
				select {
				case result <- converted:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return result, nil
}

// InvalidTransactions entrega todas las transacciones no válidas observadas en
// los bloques filtrados del canal configurado.
func (c *Client) InvalidTransactions(
	ctx context.Context,
	startBlock *uint64,
) (<-chan InvalidTransaction, error) {
	blocks, err := c.network.FilteredBlockEvents(
		ctx,
		filteredBlockEventOptions(startBlock)...,
	)
	if err != nil {
		return nil, fmt.Errorf("subscribe filtered block events: %w", err)
	}

	result := make(chan InvalidTransaction)
	go func() {
		defer close(result)
		for {
			select {
			case <-ctx.Done():
				return
			case block, ok := <-blocks:
				if !ok {
					return
				}
				for _, transaction := range invalidTransactionsFromBlock(block) {
					select {
					case result <- transaction:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()
	return result, nil
}

func chaincodeEventFromGateway(event *client.ChaincodeEvent) ChaincodeEvent {
	return ChaincodeEvent{
		BlockNumber:   event.BlockNumber,
		TransactionID: event.TransactionID,
		EventName:     event.EventName,
		Payload:       append([]byte(nil), event.Payload...),
	}
}

func chaincodeEventOptions(startBlock *uint64) []client.ChaincodeEventsOption {
	if startBlock == nil {
		return nil
	}
	return []client.ChaincodeEventsOption{client.WithStartBlock(*startBlock)}
}

func filteredBlockEventOptions(startBlock *uint64) []client.BlockEventsOption {
	if startBlock == nil {
		return nil
	}
	return []client.BlockEventsOption{client.WithStartBlock(*startBlock)}
}

func invalidTransactionsFromBlock(block *peer.FilteredBlock) []InvalidTransaction {
	if block == nil {
		return nil
	}

	transactions := make([]InvalidTransaction, 0)
	for _, transaction := range block.GetFilteredTransactions() {
		if transaction == nil || transaction.GetTxValidationCode() == peer.TxValidationCode_VALID {
			continue
		}
		validationCode := transaction.GetTxValidationCode()
		transactions = append(transactions, InvalidTransaction{
			BlockNumber:         block.GetNumber(),
			TransactionID:       transaction.GetTxid(),
			TransactionType:     enumName(common.HeaderType_name, int32(transaction.GetType())),
			ValidationCode:      enumName(peer.TxValidationCode_name, int32(validationCode)),
			ValidationCodeValue: int32(validationCode),
		})
	}
	return transactions
}

func enumName(names map[int32]string, value int32) string {
	if name, ok := names[value]; ok {
		return name
	}
	return fmt.Sprintf("UNKNOWN_%d", value)
}
