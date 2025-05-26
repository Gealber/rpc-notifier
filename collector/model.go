package collector

import "github.com/gagliardetto/solana-go"

var (
	// using DEX pools given that they need to constantly update the values
	// to keep up to date the liquidity, sqrtPrice, etc...
	DefaultAccountsToRequest = []solana.PublicKey{
		// USDC/SOL pool Orca
		solana.MPK("Czfq3xZZDmsdGdUyrNLtRhGc47cXcZtLG4crryfu44zE"),
		// USDC/SOL pool Raydium
		solana.MPK("3ucNos4NbumPLZNWztqGHNFFgkHeRMBQAVemeeomsUxv"),
		// USDC/SOL pool Meteora
		solana.MPK("5rCf1DM8LjKTw4YqhnoLcngyZYeNnQqztScTogYHAS6"),
	}
)

type Config struct {
	RPCs     []*RPCConfig       `json:"rpcs"`
	Accounts []solana.PublicKey `json:"accounts"`
}

type RPCConfig struct {
	ID         string `json:"id"`
	Endpoint   string `json:"endpoint"`
	RateLimit  int    `json:"rateLimit"`
	SampleSize int    `json:"sampleSize"`
}

type Report struct {
	RPCID        string
	MethodsStats []*MethodStats
}

type MethodStats struct {
	Name                  string
	StatsSamples          []*Stats
	ErrMsgs               []string
	AvgFirstResponseTime  float64
	AvgTotalResponseTime  float64
	TotalDataRetrieved    float64
	PositiveResponseCount int
	NegativeResponseCount int
}

// Stats in milliseconds and bytes for data size.
type Stats struct {
	Err                string
	StatusCode         int
	FirstResponseTime  int64
	TotalResponseTime  int64
	TotalDataRetrieved int64
}

type RPCCall struct {
	JsonRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type RPCResponse struct {
	JsonRPC string         `json:"jsonrpc"`
	Error   map[string]any `json:"error,omitempty"`
	// we don't care about the results itself,
	// add in case we will need it
}
