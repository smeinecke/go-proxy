package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type result struct {
	requestCount int64
	successCount int64
	errorCount   int64
	durations    []time.Duration
}

type workerArgs struct {
	mode          string
	proxyURL      *url.URL
	targetURL     string
	duration      time.Duration
	warmup        time.Duration
	workerID      int
	stickySession string
	bodySize      int
}

func main() {
	mode := flag.String("mode", "http-small", "Benchmark mode: http-small, http-large, connect-small, connect-large, sticky-session, random-ip")
	proxy := flag.String("proxy", "http://user:pass@127.0.0.1:8080", "Proxy URL")
	target := flag.String("target", "http://[::1]:8081", "Target URL")
	concurrency := flag.Int("concurrency", 100, "Number of concurrent workers")
	duration := flag.Duration("duration", 10*time.Second, "Benchmark duration")
	warmup := flag.Duration("warmup", 2*time.Second, "Warmup duration")
	flag.Parse()

	proxyURL, err := url.Parse(*proxy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid proxy URL: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Mode:       %s\n", *mode)
	fmt.Printf("Proxy:      %s\n", proxyURL.String())
	fmt.Printf("Target:     %s\n", *target)
	fmt.Printf("Workers:    %d\n", *concurrency)
	fmt.Printf("Warmup:     %s\n", *warmup)
	fmt.Printf("Duration:   %s\n", *duration)
	fmt.Println()

	// Warmup
	if *warmup > 0 {
		fmt.Println("Warming up...")
		warmupArgs := workerArgs{
			mode:      *mode,
			proxyURL:  proxyURL,
			targetURL: *target,
			duration:  *warmup,
			workerID:  0,
		}
		if *mode == "http-large" || *mode == "connect-large" {
			warmupArgs.bodySize = 64 * 1024
		}
		if *mode == "http-small" || *mode == "http-large" {
			runHTTPWorkers(1, warmupArgs)
		} else {
			runConnectWorkers(1, warmupArgs)
		}
		fmt.Println("Warmup done.")
		fmt.Println()
	}

	// Benchmark
	fmt.Println("Running benchmark...")
	var res result
	args := workerArgs{
		mode:      *mode,
		proxyURL:  proxyURL,
		targetURL: *target,
		duration:  *duration,
		workerID:  0,
	}
	if *mode == "http-large" || *mode == "connect-large" {
		args.bodySize = 64 * 1024
	}

	switch *mode {
	case "http-small", "http-large", "sticky-session", "random-ip":
		res = runHTTPWorkers(*concurrency, args)
	case "connect-small", "connect-large":
		res = runConnectWorkers(*concurrency, args)
	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", *mode)
		os.Exit(1)
	}

	printResults(&res, *duration)
}

func runHTTPWorkers(concurrency int, args workerArgs) result {
	var totalReqs int64
	var totalSuccess int64
	var totalErrors int64

	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        concurrency,
			MaxIdleConnsPerHost: concurrency,
			Proxy: func(req *http.Request) (*url.URL, error) {
				return args.proxyURL, nil
			},
		},
		Timeout: 30 * time.Second,
	}

	var body []byte
	if args.bodySize > 0 {
		body = make([]byte, args.bodySize)
	}

	workers := make([][]time.Duration, concurrency)
	for i := range workers {
		workers[i] = make([]time.Duration, 0, 1024)
	}

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			localDurations := workers[id][:0]
			end := time.Now().Add(args.duration)
			for time.Now().Before(end) {
				reqURL := args.targetURL
				if args.mode == "random-ip" {
					// Each request uses a different session to force IP rotation
					reqURL = fmt.Sprintf("%s?_=%d", args.targetURL, id+int(time.Now().UnixNano()))
				}

				reqStart := time.Now()
				var req *http.Request
				var err error
				if body != nil {
					req, err = http.NewRequest("POST", reqURL, nil)
				} else {
					req, err = http.NewRequest("GET", reqURL, nil)
				}
				if err != nil {
					atomic.AddInt64(&totalReqs, 1)
					atomic.AddInt64(&totalErrors, 1)
					continue
				}

				if args.mode == "sticky-session" {
					req.Header.Set("Proxy-Authorization", "Basic dXNlcjpwYXNzLXNlc3Npb24tc3RpY2t5MTIz")
				}

				resp, err := client.Do(req)
				dur := time.Since(reqStart)
				atomic.AddInt64(&totalReqs, 1)
				if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
					atomic.AddInt64(&totalErrors, 1)
					if resp != nil {
						resp.Body.Close()
					}
					continue
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				atomic.AddInt64(&totalSuccess, 1)
				localDurations = append(localDurations, dur)
			}
			workers[id] = localDurations
		}(i)
	}
	wg.Wait()

	// Merge per-worker slices
	var allDurations []time.Duration
	for _, w := range workers {
		allDurations = append(allDurations, w...)
	}

	return result{
		requestCount: totalReqs,
		successCount: totalSuccess,
		errorCount:   totalErrors,
		durations:    allDurations,
	}
}

func runConnectWorkers(concurrency int, args workerArgs) result {
	var totalReqs int64
	var totalSuccess int64
	var totalErrors int64

	workers := make([][]time.Duration, concurrency)
	for i := range workers {
		workers[i] = make([]time.Duration, 0, 1024)
	}

	proxyHost := args.proxyURL.Host

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			localDurations := workers[id][:0]
			end := time.Now().Add(args.duration)
			for time.Now().Before(end) {
				reqStart := time.Now()
				conn, err := net.Dial("tcp", proxyHost)
				if err != nil {
					atomic.AddInt64(&totalReqs, 1)
					atomic.AddInt64(&totalErrors, 1)
					continue
				}

				targetHost := args.targetURL
				// Strip scheme if present
				if u, err := url.Parse(targetHost); err == nil {
					targetHost = u.Host
					if targetHost == "" {
						targetHost = u.Path
					}
				}
				if targetHost == "" {
					targetHost = "[::1]:8081"
				}

				fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", targetHost, targetHost)
				br := bufio.NewReader(conn)
				resp, err := http.ReadResponse(br, nil)
				dur := time.Since(reqStart)
				if err != nil || resp.StatusCode != 200 {
					conn.Close()
					atomic.AddInt64(&totalReqs, 1)
					atomic.AddInt64(&totalErrors, 1)
					if resp != nil {
						resp.Body.Close()
					}
					continue
				}
				resp.Body.Close()
				conn.Close()
				atomic.AddInt64(&totalReqs, 1)
				atomic.AddInt64(&totalSuccess, 1)
				localDurations = append(localDurations, dur)
			}
			workers[id] = localDurations
		}(i)
	}
	wg.Wait()

	var allDurations []time.Duration
	for _, w := range workers {
		allDurations = append(allDurations, w...)
	}

	return result{
		requestCount: totalReqs,
		successCount: totalSuccess,
		errorCount:   totalErrors,
		durations:    allDurations,
	}
}

func printResults(r *result, duration time.Duration) {
	fmt.Println()
	fmt.Println("--- Results ---")
	fmt.Printf("Total requests:   %d\n", r.requestCount)
	fmt.Printf("Success count:    %d\n", r.successCount)
	fmt.Printf("Error count:      %d\n", r.errorCount)
	if duration.Seconds() > 0 {
		fmt.Printf("Throughput:       %.2f req/s\n", float64(r.successCount)/duration.Seconds())
	}

	if len(r.durations) == 0 {
		fmt.Println("No successful requests to report latencies.")
		return
	}

	sort.Slice(r.durations, func(i, j int) bool {
		return r.durations[i] < r.durations[j]
	})

	fmt.Printf("Total duration:   %s\n", duration)
	fmt.Printf("P50 latency:      %s\n", percentile(r.durations, 0.50))
	fmt.Printf("P95 latency:      %s\n", percentile(r.durations, 0.95))
	fmt.Printf("P99 latency:      %s\n", percentile(r.durations, 0.99))
	fmt.Printf("Max latency:      %s\n", r.durations[len(r.durations)-1])
}

func percentile(durations []time.Duration, p float64) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	idx := int(float64(len(durations)-1) * p)
	return durations[idx]
}
