package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	baseURL := os.Getenv("PAYMENT_URL")
	if baseURL == "" {
		panic("PAYMENT_URL environment variable is required")
	}

	fmt.Println("=== Rate Limiter Integration Tests ===")
	fmt.Println()

	testProcessing(baseURL)
	fmt.Println()
	testValidation(baseURL)

	fmt.Println()
	fmt.Println("All tests passed")
}

func testProcessing(url string) {
	fmt.Println("Test 1: Payment Process Endpoint")
	fmt.Println("Should allow 10 reqs per session then block")

	c := &http.Client{}
	ok := 0
	blocked := 0

	for i := 1; i <= 12; i++ {
		req, _ := http.NewRequest("POST", url+"/payment/process", nil)
		req.Header.Set("X-Session-ID", "test-session-123")

		resp, err := c.Do(req)
		if err != nil {
			fmt.Printf("  Req %d: ERROR - %v\n", i, err)
			os.Exit(1)
		}

		if resp.StatusCode == http.StatusOK {
			ok++
			fmt.Printf("  Req %d: OK (status %d)\n", i, resp.StatusCode)
		} else if resp.StatusCode == http.StatusTooManyRequests {
			blocked++
			fmt.Printf("  Req %d: BLOCKED (status %d)\n", i, resp.StatusCode)
		} else {
			fmt.Printf("  Req %d: unexpected status %d\n", i, resp.StatusCode)
			os.Exit(1)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	if ok != 10 || blocked != 2 {
		fmt.Printf("  FAIL: expected 10 ok, 2 blocked. got %d ok, %d blocked\n", ok, blocked)
		os.Exit(1)
	}

	fmt.Println("  test passed")
}

func testValidation(url string) {
	fmt.Println("Test 2: Payment Validate Endpoint")
	fmt.Println("Token bucket with burst=10")

	c := &http.Client{}
	ok := 0
	blocked := 0

	for i := 1; i <= 12; i++ {
		req, _ := http.NewRequest("GET", url+"/payment/validate", nil)
		resp, err := c.Do(req)
		if err != nil {
			fmt.Printf("  Req %d: ERROR - %v\n", i, err)
			os.Exit(1)
		}

		if resp.StatusCode == http.StatusOK {
			ok++
		} else if resp.StatusCode == http.StatusTooManyRequests {
			blocked++
		} else {
			fmt.Printf("  Req %d: unexpected status %d\n", i, resp.StatusCode)
			os.Exit(1)
		}

		if i == 10 || i == 11 || i == 12 {
			st := "OK"
			if resp.StatusCode == http.StatusTooManyRequests {
				st = "BLOCKED"
			}
			fmt.Printf("  Req %d: %s (status %d)\n", i, st, resp.StatusCode)
		}

		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	if ok != 10 || blocked != 2 {
		fmt.Printf("  FAIL: expected 10 ok (burst), 2 blocked. got %d ok, %d blocked\n", ok, blocked)
		os.Exit(1)
	}

	fmt.Println("  burst limit working")

	time.Sleep(2 * time.Second)
	fmt.Println("  waiting 2s for refill...")

	req, _ := http.NewRequest("GET", url+"/payment/validate", nil)
	resp, err := c.Do(req)
	if err != nil {
		fmt.Printf("  ERROR - %v\n", err)
		os.Exit(1)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("  FAIL: should be allowed after refill (got %d)\n", resp.StatusCode)
		os.Exit(1)
	}

	fmt.Printf("  after refill: OK (status %d)\n", resp.StatusCode)
	fmt.Println("  refill working")
}


