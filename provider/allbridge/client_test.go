package allbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Calculate_RequestParams(t *testing.T) {
	var gotPath, gotAmount, gotSrc, gotDst, gotMsgr string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAmount = r.URL.Query().Get("amount")
		gotSrc = r.URL.Query().Get("sourceToken")
		gotDst = r.URL.Query().Get("destinationToken")
		gotMsgr = r.URL.Query().Get("messenger")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"amountInFloat":"1.0","amountReceivedInFloat":"0.995"}`))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	ba, err := c.Calculate(context.Background(), CalcParams{
		Amount: "1.0", SourceToken: "0xA", DestinationToken: "0xB", Messenger: "ALLBRIDGE",
	})
	require.NoError(t, err)
	assert.Equal(t, "/bridge/receive/calculate", gotPath)
	assert.Equal(t, "1.0", gotAmount)
	assert.Equal(t, "0xA", gotSrc)
	assert.Equal(t, "0xB", gotDst)
	assert.Equal(t, "ALLBRIDGE", gotMsgr)
	assert.Equal(t, "0.995", ba.AmountReceivedFloat)
}

func TestClient_Calculate_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	c := NewClient()
	c.baseURL = server.URL
	_, err := c.Calculate(context.Background(), CalcParams{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 400")
}

func TestClient_RawBridge_RequestParams_EVM(t *testing.T) {
	var gotPath, gotMsgr, gotFee, gotFormat string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMsgr = r.URL.Query().Get("messenger")
		gotFee = r.URL.Query().Get("feePaymentMethod")
		gotFormat = r.URL.Query().Get("outputFormat")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"from":"0xSender","to":"0xBridge","value":"0","data":"0xdeadbeef"}`))
	}))
	defer server.Close()

	c := NewClient()
	c.baseURL = server.URL
	raw, err := c.RawBridge(context.Background(), BridgeParams{
		Amount: "1.0", Sender: "0xSender", Recipient: "0xRecipient",
		SourceToken: "0xA", DestinationToken: "0xB",
		Messenger: "ALLBRIDGE", FeePaymentMethod: "WITH_NATIVE_CURRENCY", OutputFormat: "json",
	})
	require.NoError(t, err)
	assert.Equal(t, "/raw/bridge", gotPath)
	assert.Equal(t, "ALLBRIDGE", gotMsgr)
	assert.Equal(t, "WITH_NATIVE_CURRENCY", gotFee)
	assert.Equal(t, "json", gotFormat)
	var tx EVMRawTransaction
	require.NoError(t, json.Unmarshal(raw, &tx))
	assert.Equal(t, "0xBridge", tx.To)
	assert.Equal(t, "0xdeadbeef", tx.Data)
}

func TestClient_Calculate_RealAPI(t *testing.T) {
	// Targets a self-hosted Allbridge Core REST API; skips when none is running.
	c := NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := c.Calculate(ctx, CalcParams{
		Amount: "1", SourceToken: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		DestinationToken: "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", Messenger: "ALLBRIDGE",
	})
	if err != nil {
		t.Skipf("self-hosted Allbridge Core REST API unavailable: %v", err)
	}
}
