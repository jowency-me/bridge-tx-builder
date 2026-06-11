package allbridge

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jowency-me/bridge-tx-builder/domain"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// TestLiveDocker exercises the real *Provider against a locally-running Allbridge
// Core REST API container. Enable with ALLBRIDGE_REST_URL=http://localhost:3100.
func TestLiveDocker(t *testing.T) {
	base := os.Getenv("ALLBRIDGE_REST_URL")
	if base == "" {
		t.Skip("set ALLBRIDGE_REST_URL to run against a live container")
	}
	p := NewProvider(WithBaseURL(base))
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	req := domain.QuoteRequest{
		FromToken: domain.Token{Symbol: "USDT", Address: "0xdAC17F958D2ee523a2206206994597C13D831ec7", Decimals: 6, ChainID: domain.ChainEthereum},
		ToToken:   domain.Token{Symbol: "USDT", Address: "0x55d398326f99059fF775485246999027B3197955", Decimals: 18, ChainID: domain.ChainBSC},
		Amount:    decimal.NewFromInt(10_000_000), // 10 USDT
		Slippage:  0.005,
		FromAddr:  "0x1234567890123456789012345678901234567890",
		ToAddr:    "0x1234567890123456789012345678901234567890",
	}
	q, err := p.Quote(ctx, req)
	require.NoError(t, err)
	t.Logf("LIVE allbridge quote: toAmount=%s to=%s txdata=%dB approval=%s",
		q.ToAmount, q.To, len(q.TxData), q.ApprovalAddress)
	require.True(t, q.ToAmount.IsPositive(), "ToAmount must be positive")
	require.NotEmpty(t, q.To)
	require.NotEmpty(t, q.TxData)
}
