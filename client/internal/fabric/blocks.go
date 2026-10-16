package fabric

import (
	"context"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"google.golang.org/protobuf/proto"
)

// Commit es el resultado de una transacción enviada y confirmada.
type Commit struct {
	Payload       []byte
	TransactionID string
	BlockNumber   uint64
}

// Block resume un bloque confirmado del canal.
type Block struct {
	Number       uint64
	Transactions []BlockTransaction
}

// BlockTransaction resume una transacción de un bloque: qué función invocó,
// qué organizaciones la endosaron, si quedó válida y qué escribió en
// colecciones privadas. De esas escrituras sólo conserva los hashes, que son
// lo que el bloque contiene para cualquier organización.
type BlockTransaction struct {
	Index          int
	TransactionID  string
	Function       string
	Endorsers      []string
	Valid          bool
	ValidationCode string
	PrivateWrites  []PrivateWriteHash
}

// PrivateWriteHash es la huella de una escritura privada tal como figura en
// el bloque público.
type PrivateWriteHash struct {
	Collection string
	KeyHash    string
	ValueHash  string
	IsDelete   bool
}

// PrivateWrite es una escritura privada en claro, entregada sólo por el peer
// de una organización miembro de la colección.
type PrivateWrite struct {
	Collection string
	Key        string
	Value      []byte
}

// SubmitTransaction envía una función, espera su commit y devuelve también el
// identificador de la transacción y el bloque que la contiene.
func (c *Client) SubmitTransaction(
	ctx context.Context,
	function string,
	arguments []string,
	transient map[string][]byte,
) (Commit, error) {
	proposal, err := c.contract.NewProposal(function, proposalOptions(arguments, transient)...)
	if err != nil {
		return Commit{}, err
	}
	transaction, err := proposal.EndorseWithContext(ctx)
	if err != nil {
		return Commit{}, err
	}
	commit, err := transaction.SubmitWithContext(ctx)
	if err != nil {
		return Commit{}, err
	}
	status, err := commit.StatusWithContext(ctx)
	if err != nil {
		return Commit{}, err
	}
	if !status.Successful {
		return Commit{}, fmt.Errorf(
			"transaction %s failed to commit with status code %d (%s)",
			status.TransactionID,
			int32(status.Code),
			status.Code.String(),
		)
	}
	return Commit{
		Payload:       transaction.Result(),
		TransactionID: transaction.TransactionID(),
		BlockNumber:   status.BlockNumber,
	}, nil
}

// Blocks entrega los bloques completos del canal, resumidos.
func (c *Client) Blocks(ctx context.Context, startBlock *uint64) (<-chan Block, error) {
	blocks, err := c.network.BlockEvents(ctx, filteredBlockEventOptions(startBlock)...)
	if err != nil {
		return nil, fmt.Errorf("subscribe block events: %w", err)
	}

	result := make(chan Block)
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
				select {
				case result <- summarizeBlock(block):
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return result, nil
}

// PrivateWrites pide al peer de esta conexión el bloque indicado junto con los
// datos privados que el peer entrega a esta identidad. Devuelve el resumen
// público de la transacción indicada, con los hashes de sus escrituras
// privadas, y esas escrituras en claro. Un peer que no es miembro de la
// colección no las tiene y devuelve una lista vacía.
func (c *Client) PrivateWrites(
	ctx context.Context,
	blockNumber uint64,
	transactionID string,
) (BlockTransaction, []PrivateWrite, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	events, err := c.network.BlockAndPrivateDataEvents(ctx, client.WithStartBlock(blockNumber))
	if err != nil {
		return BlockTransaction{}, nil, fmt.Errorf("subscribe block and private data events: %w", err)
	}
	select {
	case <-ctx.Done():
		return BlockTransaction{}, nil, ctx.Err()
	case event, ok := <-events:
		if !ok {
			return BlockTransaction{}, nil, fmt.Errorf(
				"block and private data stream closed before block %d",
				blockNumber,
			)
		}
		return privateWritesOf(event, transactionID)
	}
}

func privateWritesOf(
	event *peer.BlockAndPrivateData,
	transactionID string,
) (BlockTransaction, []PrivateWrite, error) {
	var found *BlockTransaction
	for _, transaction := range summarizeBlock(event.GetBlock()).Transactions {
		if transaction.TransactionID == transactionID {
			found = &transaction
			break
		}
	}
	if found == nil {
		return BlockTransaction{}, nil, fmt.Errorf(
			"transaction %s not found in block %d",
			transactionID,
			event.GetBlock().GetHeader().GetNumber(),
		)
	}

	// Index sale del range sobre los datos del bloque: nunca es negativo.
	//nolint:gosec // conversión de un índice no negativo
	privateData, ok := event.GetPrivateDataMap()[uint64(found.Index)]
	if !ok {
		return *found, []PrivateWrite{}, nil
	}
	writes := make([]PrivateWrite, 0)
	for _, namespace := range privateData.GetNsPvtRwset() {
		for _, collection := range namespace.GetCollectionPvtRwset() {
			var kvSet kvrwset.KVRWSet
			if err := proto.Unmarshal(collection.GetRwset(), &kvSet); err != nil {
				return BlockTransaction{}, nil, fmt.Errorf(
					"decode private rwset of %s: %w",
					collection.GetCollectionName(),
					err,
				)
			}
			for _, write := range kvSet.GetWrites() {
				writes = append(writes, PrivateWrite{
					Collection: collection.GetCollectionName(),
					Key:        write.GetKey(),
					Value:      append([]byte(nil), write.GetValue()...),
				})
			}
		}
	}
	return *found, writes, nil
}

func summarizeBlock(block *common.Block) Block {
	summary := Block{Number: block.GetHeader().GetNumber()}
	var validationCodes []byte
	if metadata := block.GetMetadata().GetMetadata(); len(metadata) > int(common.BlockMetadataIndex_TRANSACTIONS_FILTER) {
		validationCodes = metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER]
	}
	for index, envelopeBytes := range block.GetData().GetData() {
		transaction, ok := summarizeTransaction(envelopeBytes)
		if !ok {
			continue
		}
		transaction.Index = index
		code := peer.TxValidationCode_VALID
		if index < len(validationCodes) {
			code = peer.TxValidationCode(validationCodes[index])
		}
		transaction.Valid = code == peer.TxValidationCode_VALID
		transaction.ValidationCode = enumName(peer.TxValidationCode_name, int32(code))
		summary.Transactions = append(summary.Transactions, transaction)
	}
	return summary
}

// summarizeTransaction decodifica una transacción de endoso. Las transacciones
// de configuración se omiten: no tienen función, endosantes ni escrituras.
func summarizeTransaction(envelopeBytes []byte) (BlockTransaction, bool) {
	var envelope common.Envelope
	if err := proto.Unmarshal(envelopeBytes, &envelope); err != nil {
		return BlockTransaction{}, false
	}
	var payload common.Payload
	if err := proto.Unmarshal(envelope.GetPayload(), &payload); err != nil {
		return BlockTransaction{}, false
	}
	var channelHeader common.ChannelHeader
	if err := proto.Unmarshal(payload.GetHeader().GetChannelHeader(), &channelHeader); err != nil {
		return BlockTransaction{}, false
	}
	if common.HeaderType(channelHeader.GetType()) != common.HeaderType_ENDORSER_TRANSACTION {
		return BlockTransaction{}, false
	}

	summary := BlockTransaction{TransactionID: channelHeader.GetTxId()}
	var transaction peer.Transaction
	if err := proto.Unmarshal(payload.GetData(), &transaction); err != nil {
		return summary, true
	}
	endorsers := map[string]struct{}{}
	for _, action := range transaction.GetActions() {
		var actionPayload peer.ChaincodeActionPayload
		if err := proto.Unmarshal(action.GetPayload(), &actionPayload); err != nil {
			continue
		}
		if summary.Function == "" {
			summary.Function = invokedFunction(actionPayload.GetChaincodeProposalPayload())
		}
		for _, endorsement := range actionPayload.GetAction().GetEndorsements() {
			var identity msp.SerializedIdentity
			if err := proto.Unmarshal(endorsement.GetEndorser(), &identity); err == nil {
				endorsers[identity.GetMspid()] = struct{}{}
			}
		}
		summary.PrivateWrites = append(
			summary.PrivateWrites,
			privateWriteHashes(actionPayload.GetAction().GetProposalResponsePayload())...,
		)
	}
	for endorser := range endorsers {
		summary.Endorsers = append(summary.Endorsers, endorser)
	}
	sort.Strings(summary.Endorsers)
	return summary, true
}

func invokedFunction(proposalPayloadBytes []byte) string {
	var proposalPayload peer.ChaincodeProposalPayload
	if err := proto.Unmarshal(proposalPayloadBytes, &proposalPayload); err != nil {
		return ""
	}
	var invocation peer.ChaincodeInvocationSpec
	if err := proto.Unmarshal(proposalPayload.GetInput(), &invocation); err != nil {
		return ""
	}
	arguments := invocation.GetChaincodeSpec().GetInput().GetArgs()
	if len(arguments) == 0 {
		return ""
	}
	// contractapi admite el nombre calificado «Contrato:Función».
	function := string(arguments[0])
	if separator := strings.LastIndex(function, ":"); separator >= 0 {
		function = function[separator+1:]
	}
	return function
}

func privateWriteHashes(responsePayloadBytes []byte) []PrivateWriteHash {
	var responsePayload peer.ProposalResponsePayload
	if err := proto.Unmarshal(responsePayloadBytes, &responsePayload); err != nil {
		return nil
	}
	var chaincodeAction peer.ChaincodeAction
	if err := proto.Unmarshal(responsePayload.GetExtension(), &chaincodeAction); err != nil {
		return nil
	}
	var readWriteSet rwset.TxReadWriteSet
	if err := proto.Unmarshal(chaincodeAction.GetResults(), &readWriteSet); err != nil {
		return nil
	}
	hashes := make([]PrivateWriteHash, 0)
	for _, namespace := range readWriteSet.GetNsRwset() {
		for _, collection := range namespace.GetCollectionHashedRwset() {
			var hashed kvrwset.HashedRWSet
			if err := proto.Unmarshal(collection.GetHashedRwset(), &hashed); err != nil {
				continue
			}
			for _, write := range hashed.GetHashedWrites() {
				hashes = append(hashes, PrivateWriteHash{
					Collection: collection.GetCollectionName(),
					KeyHash:    hex.EncodeToString(write.GetKeyHash()),
					ValueHash:  hex.EncodeToString(write.GetValueHash()),
					IsDelete:   write.GetIsDelete(),
				})
			}
		}
	}
	return hashes
}
