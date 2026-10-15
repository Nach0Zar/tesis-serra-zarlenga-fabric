package fabric

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"google.golang.org/protobuf/proto"
)

func mustMarshal(t *testing.T, message proto.Message) []byte {
	t.Helper()
	contents, err := proto.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func endorserEnvelope(
	t *testing.T,
	transactionID string,
	function string,
	endorsers []string,
	collection string,
) []byte {
	t.Helper()
	endorsements := make([]*peer.Endorsement, 0, len(endorsers))
	for _, mspID := range endorsers {
		endorsements = append(endorsements, &peer.Endorsement{
			Endorser: mustMarshal(t, &msp.SerializedIdentity{Mspid: mspID}),
		})
	}
	results := mustMarshal(t, &rwset.TxReadWriteSet{NsRwset: []*rwset.NsReadWriteSet{{
		Namespace: "snt",
		CollectionHashedRwset: []*rwset.CollectionHashedReadWriteSet{{
			CollectionName: collection,
			HashedRwset: mustMarshal(t, &kvrwset.HashedRWSet{HashedWrites: []*kvrwset.KVWriteHash{
				{KeyHash: []byte{0x01}, ValueHash: []byte{0xab, 0xcd}},
				{KeyHash: []byte{0x02}, IsDelete: true},
			}}),
		}},
	}}})
	actionPayload := mustMarshal(t, &peer.ChaincodeActionPayload{
		ChaincodeProposalPayload: mustMarshal(t, &peer.ChaincodeProposalPayload{
			Input: mustMarshal(t, &peer.ChaincodeInvocationSpec{ChaincodeSpec: &peer.ChaincodeSpec{
				Input: &peer.ChaincodeInput{Args: [][]byte{[]byte(function), []byte(`{}`)}},
			}}),
		}),
		Action: &peer.ChaincodeEndorsedAction{
			ProposalResponsePayload: mustMarshal(t, &peer.ProposalResponsePayload{
				Extension: mustMarshal(t, &peer.ChaincodeAction{Results: results}),
			}),
			Endorsements: endorsements,
		},
	})
	return mustMarshal(t, &common.Envelope{Payload: mustMarshal(t, &common.Payload{
		Header: &common.Header{ChannelHeader: mustMarshal(t, &common.ChannelHeader{
			Type: int32(common.HeaderType_ENDORSER_TRANSACTION),
			TxId: transactionID,
		})},
		Data: mustMarshal(t, &peer.Transaction{Actions: []*peer.TransactionAction{{Payload: actionPayload}}}),
	})})
}

func configEnvelope(t *testing.T) []byte {
	t.Helper()
	return mustMarshal(t, &common.Envelope{Payload: mustMarshal(t, &common.Payload{
		Header: &common.Header{ChannelHeader: mustMarshal(t, &common.ChannelHeader{
			Type: int32(common.HeaderType_CONFIG),
		})},
	})})
}

func testBlock(t *testing.T) *common.Block {
	t.Helper()
	metadata := make([][]byte, int(common.BlockMetadataIndex_TRANSACTIONS_FILTER)+1)
	metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = []byte{
		byte(peer.TxValidationCode_VALID),
		byte(peer.TxValidationCode_VALID),
		byte(peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE),
	}
	return &common.Block{
		Header: &common.BlockHeader{Number: 2145},
		Data: &common.BlockData{Data: [][]byte{
			configEnvelope(t),
			endorserEnvelope(t, "tx-receive", "SNTContract:ReceiveTransfer",
				[]string{"LabMSP", "DrogueriaMSP", "LabMSP"}, "transfer_DrogueriaMSP_LabMSP"),
			endorserEnvelope(t, "tx-invalid", "Dispense", []string{"FarmaciaMSP"}, "_implicit_org_FarmaciaMSP"),
		}},
		Metadata: &common.BlockMetadata{Metadata: metadata},
	}
}

func TestSummarizeBlockReadsFunctionEndorsersAndValidation(t *testing.T) {
	summary := summarizeBlock(testBlock(t))
	if summary.Number != 2145 || len(summary.Transactions) != 2 {
		t.Fatalf("summary = %+v", summary)
	}

	receive := summary.Transactions[0]
	if receive.Index != 1 || receive.TransactionID != "tx-receive" || receive.Function != "ReceiveTransfer" {
		t.Fatalf("receive = %+v", receive)
	}
	if len(receive.Endorsers) != 2 || receive.Endorsers[0] != "DrogueriaMSP" || receive.Endorsers[1] != "LabMSP" {
		t.Fatalf("endorsers must be unique and sorted: %v", receive.Endorsers)
	}
	if !receive.Valid || receive.ValidationCode != "VALID" {
		t.Fatalf("receive validation = %v %s", receive.Valid, receive.ValidationCode)
	}
	if len(receive.PrivateWrites) != 2 ||
		receive.PrivateWrites[0] != (PrivateWriteHash{
			Collection: "transfer_DrogueriaMSP_LabMSP",
			KeyHash:    "01",
			ValueHash:  "abcd",
		}) ||
		!receive.PrivateWrites[1].IsDelete {
		t.Fatalf("private write hashes = %+v", receive.PrivateWrites)
	}

	invalid := summary.Transactions[1]
	if invalid.Valid || invalid.ValidationCode != "ENDORSEMENT_POLICY_FAILURE" || invalid.Function != "Dispense" {
		t.Fatalf("invalid = %+v", invalid)
	}
}

func TestPrivateWritesOfReturnsMemberDataForTheTransaction(t *testing.T) {
	block := testBlock(t)
	privateSet := &rwset.TxPvtReadWriteSet{NsPvtRwset: []*rwset.NsPvtReadWriteSet{{
		Namespace: "snt",
		CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{{
			CollectionName: "transfer_DrogueriaMSP_LabMSP",
			Rwset: mustMarshal(t, &kvrwset.KVRWSet{Writes: []*kvrwset.KVWrite{
				{Key: "op", Value: []byte(`{"numeroRemito":"R-1"}`)},
			}}),
		}},
	}}}

	transaction, writes, err := privateWritesOf(&peer.BlockAndPrivateData{
		Block:          block,
		PrivateDataMap: map[uint64]*rwset.TxPvtReadWriteSet{1: privateSet},
	}, "tx-receive")
	if err != nil {
		t.Fatal(err)
	}
	if transaction.TransactionID != "tx-receive" || len(transaction.PrivateWrites) != 2 {
		t.Fatalf("transaction = %+v", transaction)
	}
	if len(writes) != 1 || writes[0].Key != "op" || string(writes[0].Value) != `{"numeroRemito":"R-1"}` {
		t.Fatalf("writes = %+v", writes)
	}

	_, writes, err = privateWritesOf(&peer.BlockAndPrivateData{Block: block}, "tx-receive")
	if err != nil || len(writes) != 0 {
		t.Fatalf("non-member writes = %+v, err %v", writes, err)
	}

	if _, _, err := privateWritesOf(&peer.BlockAndPrivateData{Block: block}, "missing"); err == nil {
		t.Fatal("a transaction outside the block must fail")
	}
}
