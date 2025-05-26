package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"golang.org/x/sync/errgroup"
)

var (
	// USDC/SOL Orca pool
	AccInfoToRequest = solana.MPK("Czfq3xZZDmsdGdUyrNLtRhGc47cXcZtLG4crryfu44zE")
)

type RPCConfig struct {
	ID        string `json:"id"`
	Endpoint  string `json:"endpoint"`
	RateLimit int    `json:"rateLimit"`
}

type Report struct {
	RPCID        string
	MethodsStats []*MethodStats
}

type MethodStats struct {
	Name                  string
	StatsSamples          []*Stats
	AvgFirstResponseTime  float64
	AvgTotalResponseTime  float64
	TotalDataRetrieved    float64
	PositiveResponseCount int
	NegativeResponseCount int
}

// Stats in milliseconds and bytes for data size.
type Stats struct {
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

func main() {
	f, err := os.Open("rpc.json")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		panic(err)
	}

	var cfgs []RPCConfig
	err = json.Unmarshal(data, &cfgs)
	if err != nil {
		panic(err)
	}

	n := 1
	ctx := context.Background()
	reports := make([]*Report, 0, len(cfgs))
	for _, cfg := range cfgs {
		report, err := collectResults(ctx, cfg, n)
		if err != nil {
			panic(err)
		}

		reports = append(reports, report)
	}

	// csv file
	records, err := os.Create("records.csv")
	if err != nil {
		panic(err)
	}
	defer records.Close()

	records.WriteString("rpc|method|status_code|frt|trt|total_data_retrieved(KB)\n")

	// save reports to later generate graphs
	for _, r := range reports {
		fmt.Printf("RPC_NAME: %s\n", r.RPCID)
		for _, m := range r.MethodsStats {
			for _, s := range m.StatsSamples {
				line := fmt.Sprintf("%s|%s|%d|%d|%d|%f\n", r.RPCID, m.Name, s.StatusCode, s.FirstResponseTime, s.TotalResponseTime, float64(s.TotalDataRetrieved)/1024)
				records.WriteString(line)
			}

			fmt.Printf("Method: %s\nAvg FRT: %f\nAvg TRT: %f\nPositive Response Count: %d\nNegative Response Count: %d\nTotal Data Retrieved(MB): %f\n", m.Name, m.AvgFirstResponseTime, m.AvgTotalResponseTime, m.PositiveResponseCount, m.NegativeResponseCount, m.TotalDataRetrieved/1048576)
			fmt.Println("---------------------------------------------------------------------------------------")
		}
		fmt.Println("---------------------------------------------------------------------------------------")
	}
}

// collectResults collects the results for a given rpc configuration for each method.
// n specifies the amount of calls to perform for each method.
// The methods to be tested are getAccountInfo, getMultipleAccounts, and getProgramAccounts.
func collectResults(ctx context.Context, cfg RPCConfig, n int) (*Report, error) {
	result := &Report{
		RPCID:        cfg.ID,
		MethodsStats: make([]*MethodStats, 0),
	}

	// getAccountInfo
	report, err := getAccountInfo(ctx, cfg, AccInfoToRequest, n)
	if err != nil {
		return nil, err
	}
	result.MethodsStats = append(result.MethodsStats, report)

	// getMultipleAccounts
	// getProgramAccounts

	return result, nil
}

func getAccountInfo(
	ctx context.Context,
	cfg RPCConfig,
	account solana.PublicKey,
	n int,
) (*MethodStats, error) {
	call := RPCCall{
		JsonRPC: "2.0",
		ID:      1,
		Method:  "getAccountInfo",
		Params: []any{
			account.String(),
			struct {
				Encoding string `json:"encoding"`
			}{
				Encoding: "base64",
			},
		},
	}

	return collectStats(ctx, cfg, &call, n, "getAccountInfo")
}

// collectStats collect stats about a given call, performing the specified amount of calls. All the calls
// are performed sequentially no goroutine are dispatched here, and respecting
// the rate limit of the RPC provider.
func collectStats(
	ctx context.Context,
	cfg RPCConfig,
	call *RPCCall,
	amount int,
	name string,
) (*MethodStats, error) {
	result := &MethodStats{
		Name: name,
	}

	bucket := make(chan struct{}, cfg.RateLimit)

	for range cfg.RateLimit {
		bucket <- struct{}{}
	}

	limiter := time.NewTicker(1 * time.Second)
	go func() {
		for range limiter.C {
			for range cfg.RateLimit - len(bucket) {
				bucket <- struct{}{}
			}
		}
	}()

	var (
		g  errgroup.Group
		mu sync.Mutex
	)
	counter := make(map[int64]int64)
	for i := 0; i < amount; i++ {
		g.Go(func() error {
			<-bucket

			stats, err := post(ctx, cfg, call)
			if err != nil {
				return err
			}

			mu.Lock()
			result.StatsSamples = append(result.StatsSamples, stats)
			counter[time.Now().Unix()]++
			mu.Unlock()

			if stats.StatusCode != http.StatusOK {
				fmt.Println(stats.StatusCode)
				result.NegativeResponseCount++
				return nil
			}

			result.PositiveResponseCount++
			result.AvgFirstResponseTime += float64(stats.FirstResponseTime)
			result.AvgTotalResponseTime += float64(stats.TotalResponseTime)
			result.TotalDataRetrieved += float64(stats.TotalDataRetrieved)
			return nil
		})
	}

	err := g.Wait()
	if err != nil {
		return nil, err
	}
	limiter.Stop()

	result.AvgFirstResponseTime /= float64(amount)
	result.AvgTotalResponseTime /= float64(amount)
	// fmt.Println(counter)

	return result, nil
}

func post(
	ctx context.Context,
	cfg RPCConfig,
	call *RPCCall,
) (*Stats, error) {
	b, err := json.Marshal(call)
	if err != nil {
		return nil, err
	}

	body := bytes.NewBuffer(b)
	clt := http.Client{}

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")

	stats := new(Stats)
	start := time.Now()

	resp, err := clt.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	stats.StatusCode = resp.StatusCode

	if resp.StatusCode != http.StatusOK {
		// we just return the status stats with the status code
		// and caller can count this a negative response
		return stats, nil
	}

	// read response by chunks in order to get the First reponse time
	data := make([]byte, 0)
	buff := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buff)
		stats.TotalDataRetrieved += int64(n)
		if stats.FirstResponseTime == 0 {
			stats.FirstResponseTime = time.Now().Sub(start).Milliseconds()
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				data = append(data, buff[:n]...)
				break
			}

			return nil, err
		}

		data = append(data, buff[:n]...)
	}

	stats.TotalResponseTime = time.Now().Sub(start).Milliseconds()
	// fmt.Println(string(data))

	return stats, nil
}
