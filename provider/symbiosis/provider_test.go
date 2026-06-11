package symbiosis

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
	quoteResp *QuoteResponse
	quoteErr  error
}

func (m *mockClient) Quote(ctx context.Context, req QuoteRequest) (*QuoteResponse, error) {
	return m.quoteResp, m.quoteErr
}

func TestProvider_Name(t *testing.T) {
	assert.Equal(t, "symbiosis", NewProvider().Name())
}

func TestNewProvider_WithBaseURL(t *testing.T) {
	p := NewProvider(WithBaseURL("https://custom.example.com/"))
	require.NotNil(t, p)
	c, ok := p.client.(*Client)
	require.True(t, ok)
	assert.Equal(t, "https://custom.example.com", c.baseURL)
}

func TestNewProvider_WithHTTPClient(t *testing.T) {
	p := NewProvider(WithHTTPClient(http.DefaultClient))
	require.NotNil(t, p)
	c, ok := p.client.(*Client)
	require.True(t, ok)
	assert.Equal(t, http.DefaultClient, c.client)
}

func TestProvider_Status_NotSupported(t *testing.T) {
	_, err := NewProvider().Status(context.Background(), "0xTx")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status tracking not supported")
}

func TestProvider_Quote_Success_EVM(t *testing.T) {
	mock := &mockClient{quoteResp: &QuoteResponse{
		Tx:                Tx{ChainID: 1, To: "0xPool", Data: "0xdeadbeef", Value: "0"},
		ApproveTo:         "0xPool",
		TokenAmountOut:    TokenAmount{Symbol: "USDT", Amount: "995000"},
		TokenAmountOutMin: TokenAmount{Symbol: "USDT", Amount: "990025"},
	}}
	p := &Provider{client: mock}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainBSC},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "0xSender", ToAddr: "0xRecipient",
	}
	quote, err := p.Quote(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "symbiosis", quote.Provider)
	assert.Equal(t, int64(995_000), quote.ToAmount.IntPart())
	assert.Equal(t, "0xPool", quote.To)
	expected, _ := hexutil.Decode("deadbeef")
	assert.Equal(t, expected, quote.TxData)
	assert.True(t, quote.TxValue.IsZero())
	assert.Equal(t, "0xPool", quote.ApprovalAddress)
	require.NotNil(t, quote.AllowanceNeeded)
}

func TestProvider_Quote_Success_Tron(t *testing.T) {
	mock := &mockClient{quoteResp: &QuoteResponse{
		Tx: Tx{ChainID: 728126428, To: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
			Data:  `{"contract_address":"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t","function_selector":"transfer(address,uint256)","parameter":"abcd","call_value":0,"fee_limit":150000000}`,
			Value: "0"},
		ApproveTo:         "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
		TokenAmountOut:    TokenAmount{Symbol: "USDT", Amount: "995000"},
		TokenAmountOutMin: TokenAmount{Symbol: "USDT", Amount: "990025"},
	}}
	p := &Provider{client: mock}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDT", Address: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", Decimals: 6, ChainID: domain.ChainTron},
		ToToken:   domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005,
		FromAddr:  "TUser11111111111111111111111111111111111", ToAddr: "0xRecipient",
	}
	quote, err := p.Quote(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", quote.To)
	// The provider should extract the `parameter` field (hex calldata) from the
	// Symbiosis Tron JSON envelope and decode it; the downstream Tron builder
	// expects raw calldata bytes, not the full JSON envelope.
	assert.Equal(t, []byte{0xab, 0xcd}, quote.TxData)
	assert.Equal(t, "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", quote.ApprovalAddress)
}

func TestProvider_Quote_UnsupportedChain(t *testing.T) {
	p := &Provider{client: &mockClient{}}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "X", Address: "0xA", Decimals: 6, ChainID: domain.ChainBitcoin},
		ToToken:   domain.Token{Symbol: "Y", Address: "0xB", Decimals: 6, ChainID: domain.ChainEthereum},
		Amount:    decimal.NewFromInt(1), Slippage: 0.005, FromAddr: "0xA", ToAddr: "0xB",
	}
	_, err := p.Quote(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported from chain")
}

func TestProvider_Quote_ClientError(t *testing.T) {
	p := &Provider{client: &mockClient{quoteErr: assert.AnError}}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainBSC},
		Amount:    decimal.NewFromInt(1_000_000), Slippage: 0.005, FromAddr: "0xA", ToAddr: "0xB",
	}
	_, err := p.Quote(context.Background(), req)
	require.Error(t, err)
}

func TestProvider_Quote_InvalidRequest(t *testing.T) {
	mock := &mockClient{}
	p := &Provider{client: mock}
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainBSC},
		Amount:    decimal.Zero, Slippage: 0.005, FromAddr: "0xA", ToAddr: "0xB",
	}
	_, err := p.Quote(context.Background(), req)
	require.Error(t, err)
	assert.Nil(t, mock.quoteResp)
}

// TestProvider_Quote_SlippageConversion wires Provider.Quote through a real
// client.Quote call (no mock) to verify the bps conversion: req.Slippage=0.005
// (0.5%) must produce Slippage=50 in the wire body, not 500.
func TestProvider_Quote_SlippageConversion(t *testing.T) {
	var gotReq QuoteRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotReq))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(QuoteResponse{
			Tx:                Tx{ChainID: 1, To: "0xPool", Data: "0xdeadbeef", Value: "0"},
			TokenAmountOut:    TokenAmount{Symbol: "USDT", Amount: "995000"},
			TokenAmountOutMin: TokenAmount{Symbol: "USDT", Amount: "990025"},
		})
	}))
	defer server.Close()
	p := NewProvider(WithBaseURL(server.URL))
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0xB", Decimals: 6, ChainID: domain.ChainBSC},
		Amount:    decimal.NewFromInt(1_000_000),
		Slippage:  0.005, // 0.5%
		FromAddr:  "0xSender", ToAddr: "0xRecipient",
	}
	_, err := p.Quote(context.Background(), req)
	require.NoError(t, err)
	// 0.005 * 10000 = 50 bps. The pre-fix bug would have sent 500 (test asserted
	// this stale value because the test exercised the client directly, not the
	// provider's bps conversion).
	assert.Equal(t, 50, gotReq.Slippage, "req.Slippage=0.005 (0.5%%) must convert to 50 bps on the wire")
}

// TestProvider_Quote_Tron_MalformedEnvelope verifies the re-review fix: when
// Symbiosis returns a Tron response whose Data field is not a valid JSON
// envelope with a hex parameter field, the provider must surface an error
// rather than silently store the raw envelope bytes (which would cause the
// downstream Tron builder to sign garbage).
func TestProvider_Quote_Tron_MalformedEnvelope(t *testing.T) {
	cases := []struct {
		name string
		data string
	}{
		{"empty_data", ""},
		{"raw_hex", "0x7ff73683"},
		{"json_missing_parameter", `{"contract_address":"TR7...","function_selector":"transfer(address,uint256)"}`},
		{"json_non_hex_parameter", `{"contract_address":"TR7...","function_selector":"transfer(address,uint256)","parameter":"not hex"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockClient{quoteResp: &QuoteResponse{
				Tx:                Tx{ChainID: 728126428, To: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", Data: tc.data, Value: "0"},
				TokenAmountOut:    TokenAmount{Symbol: "USDT", Amount: "995000"},
				TokenAmountOutMin: TokenAmount{Symbol: "USDT", Amount: "990025"},
			}}
			p := &Provider{client: mock}
			req := domain.QuoteRequest{
				FromToken: domain.Token{Symbol: "USDT", Address: "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", Decimals: 6, ChainID: domain.ChainTron},
				ToToken:   domain.Token{Symbol: "USDC", Address: "0xA", Decimals: 6, ChainID: domain.ChainEthereum},
				Amount:    decimal.NewFromInt(1_000_000), Slippage: 0.005,
				FromAddr: "TUser11111111111111111111111111111111111", ToAddr: "0xRecipient",
			}
			_, err := p.Quote(context.Background(), req)
			require.Error(t, err, "malformed Tron envelope must surface an error (case: %s)", tc.name)
			assert.Contains(t, err.Error(), "tron tx data is not a valid JSON envelope")
		})
	}
}
