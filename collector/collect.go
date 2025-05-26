package collector

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
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

const (
	defaultSampleSize = 1
)

type Collector struct {
	notifier *Notifier
	interval time.Duration
	cfg      *Config
}

func New(
	filename string,
	interval time.Duration,
) (*Collector, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	var cfg Config
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}

	// check sample sizes
	for _, rpc := range cfg.RPCs {
		if rpc.SampleSize == 0 {
			rpc.SampleSize = defaultSampleSize
		}
	}

	if len(cfg.Accounts) == 0 {
		cfg.Accounts = append(cfg.Accounts, DefaultAccountsToRequest...)
	}

	notifier := NewNotifier()

	return &Collector{
		notifier: notifier,
		cfg:      &cfg,
		interval: interval,
	}, nil
}

func (c *Collector) Run() error {
	ctx := context.Background()
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// to triger ticker right away
	for ; true; <-ticker.C {
		for _, rpc := range c.cfg.RPCs {
			r, err := collectResults(ctx, rpc, c.cfg.Accounts)
			if err != nil {
				log.Err(err).Str("rpc_name", rpc.ID).Msg("collectResults")
				c.notify(rpc.ID + " " + err.Error())
				continue
			}

			for _, m := range r.MethodsStats {
				// in case any error was encountered just print the errors
				if len(m.ErrMsgs) > 0 {
					for _, errMsg := range m.ErrMsgs {
						log.Debug().
							Str("rpc_name", r.RPCID).
							Int("sample_size", rpc.SampleSize).
							Str("err_msg", errMsg).
							Msg("errors encountered on rpc")
					}

					// notify only the first error
					c.notify(r.RPCID + " " + m.ErrMsgs[0])
					continue
				}

				log.Debug().
					Str("rpc_name", r.RPCID).
					Int("sample_size", rpc.SampleSize).
					Str("method_name", m.Name).
					Float64("avg_frt", m.AvgFirstResponseTime).
					Float64("avg_trt", m.AvgTotalResponseTime).
					Int("positive_count", m.PositiveResponseCount).
					Int("negative_count", m.NegativeResponseCount).
					Float64("total_data_retrieved_kb", m.TotalDataRetrieved/1048576).
					Msg("rpc results")
			}
			fmt.Println("---------------------------------------------------------------------------------------")
		}
	}

	return nil
}

func (c *Collector) notify(text string) {
	if c.notifier != nil {
		err := c.notifier.Notify(text)
		if err != nil {
			log.Debug().Err(err).Msg("Run")
		}
	}
}

// collectResults collects the results for a given rpc configuration for each method.
// n specifies the amount of calls to perform for each method.
// The methods to be tested are getAccountInfo, getMultipleAccounts, and getProgramAccounts.
func collectResults(ctx context.Context, cfg *RPCConfig, accounts []solana.PublicKey) (*Report, error) {
	result := &Report{
		RPCID:        cfg.ID,
		MethodsStats: make([]*MethodStats, 0),
	}

	// getAccountInfo
	report, err := getAccountInfo(ctx, cfg, accounts[0])
	if err != nil {
		return nil, err
	}
	result.MethodsStats = append(result.MethodsStats, report)

	// getMultipleAccounts
	report, err = getMultipleAccounts(ctx, cfg, accounts)
	if err != nil {
		return nil, err
	}
	result.MethodsStats = append(result.MethodsStats, report)

	// getProgramAccounts

	return result, nil
}

func getAccountInfo(
	ctx context.Context,
	rpc *RPCConfig,
	account solana.PublicKey,
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

	return collectStats(ctx, rpc, &call, "getAccountInfo")
}

func getMultipleAccounts(
	ctx context.Context,
	rpc *RPCConfig,
	accounts []solana.PublicKey,
) (*MethodStats, error) {
	accs := make([]string, len(accounts))
	for i := range accounts {
		accs[i] = accounts[i].String()
	}

	call := RPCCall{
		JsonRPC: "2.0",
		ID:      1,
		Method:  "getMultipleAccounts",
		Params: []any{
			accs,
			struct {
				Encoding string `json:"encoding"`
			}{
				Encoding: "base64",
			},
		},
	}

	return collectStats(ctx, rpc, &call, "getMultipleAccounts")
}

// collectStats collect stats about a given call, performing the specified amount of calls. All the calls
// are performed sequentially no goroutine are dispatched here, and respecting
// the rate limit of the RPC provider.
func collectStats(
	ctx context.Context,
	rpc *RPCConfig,
	call *RPCCall,
	name string,
) (*MethodStats, error) {
	result := &MethodStats{
		Name: name,
	}

	bucket := make(chan struct{}, rpc.RateLimit)

	for range rpc.RateLimit {
		bucket <- struct{}{}
	}

	limiter := time.NewTicker(1 * time.Second)
	go func() {
		for range limiter.C {
			for range rpc.RateLimit - len(bucket) {
				bucket <- struct{}{}
			}
		}
	}()

	var (
		g  errgroup.Group
		mu sync.Mutex
	)

	counter := make(map[int64]int64)
	for i := 0; i < rpc.SampleSize; i++ {
		g.Go(func() error {
			<-bucket

			stats, err := post(ctx, rpc, call)
			if err != nil {
				return err
			}

			mu.Lock()
			result.StatsSamples = append(result.StatsSamples, stats)
			if stats.Err != "" {
				result.ErrMsgs = append(result.ErrMsgs, stats.Err)
			}
			counter[time.Now().Unix()]++
			mu.Unlock()

			if stats.StatusCode != http.StatusOK {
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

	result.AvgFirstResponseTime /= float64(rpc.SampleSize)
	result.AvgTotalResponseTime /= float64(rpc.SampleSize)

	return result, nil
}

func post(
	ctx context.Context,
	rpc *RPCConfig,
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rpc.Endpoint, body)
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
		// in case status code is not 200
		// add this as error message
		stats.Err = fmt.Sprintf("received unexpected status code: %d", resp.StatusCode)
		return stats, nil
	}

	// read response by chunks in order to get the First Reponse Time
	// this metric is important in FluxRPC given that we stream gPA response...basically everything I think.
	data := make([]byte, 0, 4096)
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
	var r RPCResponse
	err = json.Unmarshal(data, &r)
	if err != nil {
		return nil, err
	}

	if r.Error != nil {
		stats.Err, _ = r.Error["message"].(string)
	}

	return stats, nil
}
