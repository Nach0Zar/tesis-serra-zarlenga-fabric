package fabric

import (
	"testing"

	gatewayclient "github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
)

func TestChaincodeEventFromGatewayPreservesMetadata(t *testing.T) {
	payload := []byte(`{"estado":"ROBADO"}`)
	got := chaincodeEventFromGateway(&gatewayclient.ChaincodeEvent{
		BlockNumber:   52,
		TransactionID: "tx-report-stolen",
		ChaincodeName: "snt",
		EventName:     "ReportStolen",
		Payload:       payload,
	})
	if got.BlockNumber != 52 ||
		got.TransactionID != "tx-report-stolen" ||
		got.EventName != "ReportStolen" ||
		string(got.Payload) != string(payload) {
		t.Fatalf("unexpected chaincode event: %+v", got)
	}
	payload[0] = 'X'
	if string(got.Payload) != `{"estado":"ROBADO"}` {
		t.Fatalf("payload was not copied: %q", got.Payload)
	}
}

func TestInvalidTransactionsFromBlock(t *testing.T) {
	block := &peer.FilteredBlock{
		Number: 42,
		FilteredTransactions: []*peer.FilteredTransaction{
			{
				Txid:             "valid-tx",
				Type:             common.HeaderType_ENDORSER_TRANSACTION,
				TxValidationCode: peer.TxValidationCode_VALID,
			},
			{
				Txid:             "endorsement-tx",
				Type:             common.HeaderType_ENDORSER_TRANSACTION,
				TxValidationCode: peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE,
			},
			{
				Txid:             "mvcc-tx",
				Type:             common.HeaderType_ENDORSER_TRANSACTION,
				TxValidationCode: peer.TxValidationCode_MVCC_READ_CONFLICT,
			},
		},
	}

	got := invalidTransactionsFromBlock(block)
	if len(got) != 2 {
		t.Fatalf("invalid transaction count = %d, want 2", len(got))
	}
	if got[0].BlockNumber != 42 ||
		got[0].TransactionID != "endorsement-tx" ||
		got[0].TransactionType != "ENDORSER_TRANSACTION" ||
		got[0].ValidationCode != "ENDORSEMENT_POLICY_FAILURE" ||
		got[0].ValidationCodeValue != int32(peer.TxValidationCode_ENDORSEMENT_POLICY_FAILURE) {
		t.Fatalf("unexpected endorsement transaction: %+v", got[0])
	}
	if got[1].TransactionID != "mvcc-tx" ||
		got[1].ValidationCode != "MVCC_READ_CONFLICT" ||
		got[1].ValidationCodeValue != int32(peer.TxValidationCode_MVCC_READ_CONFLICT) {
		t.Fatalf("unexpected MVCC transaction: %+v", got[1])
	}
}

func TestInvalidTransactionsFromBlockHandlesNilAndUnknownValues(t *testing.T) {
	if got := invalidTransactionsFromBlock(nil); got != nil {
		t.Fatalf("nil block = %#v, want nil", got)
	}

	block := &peer.FilteredBlock{
		Number: 7,
		FilteredTransactions: []*peer.FilteredTransaction{
			nil,
			{
				Txid:             "unknown-tx",
				Type:             common.HeaderType(99),
				TxValidationCode: peer.TxValidationCode(99),
			},
		},
	}
	got := invalidTransactionsFromBlock(block)
	if len(got) != 1 {
		t.Fatalf("invalid transaction count = %d, want 1", len(got))
	}
	if got[0].TransactionType != "UNKNOWN_99" || got[0].ValidationCode != "UNKNOWN_99" {
		t.Fatalf("unexpected unknown enum conversion: %+v", got[0])
	}
}

func TestEventOptionsPreserveOptionalStartBlockIncludingZero(t *testing.T) {
	if got := chaincodeEventOptions(nil); len(got) != 0 {
		t.Fatalf("chaincode options without start block = %d, want 0", len(got))
	}
	if got := filteredBlockEventOptions(nil); len(got) != 0 {
		t.Fatalf("filtered block options without start block = %d, want 0", len(got))
	}

	startBlock := uint64(0)
	if got := chaincodeEventOptions(&startBlock); len(got) != 1 {
		t.Fatalf("chaincode options with start block zero = %d, want 1", len(got))
	}
	if got := filteredBlockEventOptions(&startBlock); len(got) != 1 {
		t.Fatalf("filtered block options with start block zero = %d, want 1", len(got))
	}
}
