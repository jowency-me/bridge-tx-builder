package allbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jowency-me/bridge-tx-builder/domain"
	hexutil "github.com/jowency-me/bridge-tx-builder/utils/hex"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockClient struct {
	calc     *BridgeAmounts
	calcErr  error
	raw      json.RawMessage
	rawErr   error
	lastCalc CalcParams
	lastRaw  BridgeParams
}

func (m *mockClient) Calculate(ctx context.Context, params CalcParams) (*BridgeAmounts, error) {
	m.lastCalc = params
	return m.calc, m.calcErr
}

func (m *mockClient) RawBridge(ctx context.Context, params BridgeParams) (json.RawMessage, error) {
	m.lastRaw = params
	return m.raw, m.rawErr
}

func evmReq() domain.QuoteRequest {
	return domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA0b86991", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDC", Address: "0x83358", Decimals: 6, ChainID: domain.ChainBase},
		Amount:    decimal.NewFromInt(1_000_000), // 1.0 USDC
		Slippage:  0.005,
		FromAddr:  "0xSender", ToAddr: "0xRecipient",
	}
}

func TestProvider_Name(t *testing.T) {
	assert.Equal(t, "allbridge", NewProvider().Name())
}

func TestNewProvider_Options(t *testing.T) {
	p := NewProvider(WithBaseURL("http://host:3000/"), WithHTTPClient(http.DefaultClient), WithMessenger("WORMHOLE"))
	c, ok := p.client.(*Client)
	require.True(t, ok)
	assert.Equal(t, "http://host:3000", c.baseURL)
	assert.Equal(t, http.DefaultClient, c.client)
	assert.Equal(t, "WORMHOLE", p.messenger)
}

func TestProvider_Status_NotSupported(t *testing.T) {
	_, err := NewProvider().Status(context.Background(), "0xTx")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status tracking not supported")
}

func TestProvider_Quote_Success_EVM(t *testing.T) {
	mock := &mockClient{
		calc: &BridgeAmounts{AmountInFloat: "1.0", AmountReceivedFloat: "0.995"},
		raw:  json.RawMessage(`{"from":"0xSender","to":"0xBridge","value":"0","data":"0xdeadbeef"}`),
	}
	p := &Provider{client: mock, messenger: defaultMessenger}
	quote, err := p.Quote(context.Background(), evmReq())
	require.NoError(t, err)
	assert.Equal(t, "allbridge", quote.Provider)
	// 0.995 received, dest decimals 6 -> 995000 base units.
	assert.Equal(t, int64(995_000), quote.ToAmount.IntPart())
	assert.Equal(t, "0xBridge", quote.To)
	expected, _ := hexutil.Decode("deadbeef")
	assert.Equal(t, expected, quote.TxData)
	assert.True(t, quote.TxValue.IsZero())
	assert.Equal(t, "0xBridge", quote.ApprovalAddress)
	require.NotNil(t, quote.AllowanceNeeded)
	assert.Equal(t, "bridge", quote.Route[0].Action)
	// The API must receive integer base units (1000000, NOT float 1.0) and the
	// ALLBRIDGE messenger — the REST API converts to float internally.
	assert.Equal(t, "1000000", mock.lastCalc.Amount)
	assert.Equal(t, "1000000", mock.lastRaw.Amount)
	assert.Equal(t, "ALLBRIDGE", mock.lastRaw.Messenger)
	assert.Equal(t, "json", mock.lastRaw.OutputFormat)
}

func TestProvider_Quote_NativeFee_StillApproves(t *testing.T) {
	// A non-zero tx.value is the WITH_NATIVE_CURRENCY messenger fee, not the
	// bridged amount. The ERC-20 source token still needs approval to the bridge.
	mock := &mockClient{
		calc: &BridgeAmounts{AmountReceivedFloat: "0.99"},
		raw:  json.RawMessage(`{"to":"0xBridge","value":"251332194862189","data":"0x4cd480bd"}`),
	}
	p := &Provider{client: mock, messenger: defaultMessenger}
	quote, err := p.Quote(context.Background(), evmReq())
	require.NoError(t, err)
	assert.True(t, decimal.NewFromInt(251332194862189).Equal(quote.TxValue), "tx value carries the native messenger fee")
	assert.Equal(t, "0xBridge", quote.ApprovalAddress, "ERC-20 source always needs approval to the bridge")
	require.NotNil(t, quote.AllowanceNeeded)
}

func TestProvider_Quote_Success_Solana(t *testing.T) {
	mock := &mockClient{
		calc: &BridgeAmounts{AmountReceivedFloat: "0.995"},
		raw:  json.RawMessage(`"AQIDBA=="`), // base64 string envelope
	}
	p := &Provider{client: mock, messenger: defaultMessenger}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "EPjF", Decimals: 6, ChainID: domain.ChainSolana},
		ToToken:   domain.Token{Symbol: "USDC", Address: "0xB", Decimals: 6, ChainID: domain.ChainBase},
		Amount:    decimal.NewFromInt(1_000_000), Slippage: 0.005,
		FromAddr: "SolPubkey", ToAddr: "0xRecipient",
	}
	quote, err := p.Quote(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, quote.TxData)
	assert.True(t, quote.TxValue.IsZero())
	assert.Empty(t, quote.ApprovalAddress)
	assert.Equal(t, "base64", mock.lastRaw.OutputFormat)
}

func TestProvider_Quote_Success_Tron(t *testing.T) {
	mock := &mockClient{
		calc: &BridgeAmounts{AmountReceivedFloat: "0.995"},
		raw:  json.RawMessage(`{"contract_address":"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t","raw_data":{"contract":[]}}`),
	}
	p := &Provider{client: mock, messenger: defaultMessenger}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDT", Address: "TR7N", Decimals: 6, ChainID: domain.ChainTron},
		ToToken:   domain.Token{Symbol: "USDC", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000), Slippage: 0.005,
		FromAddr: "TUser", ToAddr: "0xRecipient",
	}
	quote, err := p.Quote(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", quote.To)
	assert.NotEmpty(t, quote.TxData)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(quote.TxData, &parsed))
	assert.Equal(t, "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", parsed["contract_address"])
	assert.Equal(t, "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", quote.ApprovalAddress)
}

func TestProvider_Quote_UnsupportedChain(t *testing.T) {
	p := &Provider{client: &mockClient{}, messenger: defaultMessenger}
	req := evmReq()
	req.FromToken.ChainID = domain.ChainBitcoin
	_, err := p.Quote(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported from chain")
}

func TestProvider_Quote_CalculateError(t *testing.T) {
	p := &Provider{client: &mockClient{calcErr: assert.AnError}, messenger: defaultMessenger}
	_, err := p.Quote(context.Background(), evmReq())
	require.Error(t, err)
}

func TestProvider_Quote_RawBridgeError(t *testing.T) {
	mock := &mockClient{calc: &BridgeAmounts{AmountReceivedFloat: "0.99"}, rawErr: assert.AnError}
	p := &Provider{client: mock, messenger: defaultMessenger}
	_, err := p.Quote(context.Background(), evmReq())
	require.Error(t, err)
}

func TestProvider_Quote_InvalidRequest(t *testing.T) {
	mock := &mockClient{}
	p := &Provider{client: mock, messenger: defaultMessenger}
	req := evmReq()
	req.Amount = decimal.Zero
	_, err := p.Quote(context.Background(), req)
	require.Error(t, err)
	assert.Nil(t, mock.calc, "mock must not be called on invalid request")
}

func TestProvider_Quote_InvalidEVMHexData(t *testing.T) {
	mock := &mockClient{
		calc: &BridgeAmounts{AmountReceivedFloat: "0.99"},
		raw:  json.RawMessage(`{"to":"0xBridge","value":"0","data":"0xZZnothex"}`),
	}
	p := &Provider{client: mock, messenger: defaultMessenger}
	_, err := p.Quote(context.Background(), evmReq())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tx data")
}

func TestProvider_Quote_EmptyEVMTo(t *testing.T) {
	mock := &mockClient{
		calc: &BridgeAmounts{AmountReceivedFloat: "0.99"},
		raw:  json.RawMessage(`{"to":"","value":"0","data":"0xab"}`),
	}
	p := &Provider{client: mock, messenger: defaultMessenger}
	_, err := p.Quote(context.Background(), evmReq())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing to address")
}

// TestProvider_EndToEnd_Golden runs the full Provider.Quote flow through the real
// *Client against an httptest server serving the documented Allbridge Core REST
// API shapes (BridgeAmounts + EVM RawTransaction), proving the adapter wires the
// official endpoints correctly without a running Docker instance.
func TestProvider_EndToEnd_Golden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/bridge/receive/calculate":
			_, _ = w.Write([]byte(`{"amountInFloat":"1.0","amountReceivedInFloat":"0.994"}`))
		case "/raw/bridge":
			_, _ = w.Write([]byte(`{"from":"0xSender","to":"0xBridgeContract","value":"0","data":"0xa9059cbbdeadbeef"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	p := NewProvider(WithBaseURL(server.URL))
	quote, err := p.Quote(context.Background(), evmReq())
	require.NoError(t, err)
	assert.Equal(t, int64(994_000), quote.ToAmount.IntPart())
	assert.Equal(t, "0xBridgeContract", quote.To)
	assert.NotEmpty(t, quote.TxData)
	assert.Equal(t, "0xBridgeContract", quote.ApprovalAddress)
}
