package main

import (
	"fmt"
	"os"
	"time"

	"github.com/larrasket/hlimiter/pkg/client"
)

func main() {
	limiterURL := os.Getenv("LIMITER_URL")
	if limiterURL == "" {
		panic("LIMITER_URL environment variable is required")
	}

	c := client.New(limiterURL)

	fmt.Println("=== Rate Limiter Validation Tests ===")
	fmt.Println()

	testSlidingWindow(c)
	fmt.Println()
	testTokenBucket(c)
	fmt.Println()
	testHeaderStrategy(c)

	fmt.Println()
	fmt.Println("All validation tests passed")
}

func testSlidingWindow(c *client.Client) {
	fmt.Println("Test 1: Sliding Window Algorithm")
	fmt.Println("Config: /api/login - limit 5 requests per 60s")

	passed := 0
	blocked := 0

	for i := 1; i <= 7; i++ {
		resp, err := c.Check(client.CheckRequest{
			Service: "api-gateway",
			API:     "/api/login",
			IP:      "192.168.1.100",
		})
		if err != nil {
			fmt.Printf("  Request %d: ERROR - %v\n", i, err)
			os.Exit(1)
		}

		if resp.Allowed {
			passed++
			fmt.Printf("  Request %d: ALLOWED (remaining: %d)\n", i, resp.Remaining)
		} else {
			blocked++
			fmt.Printf("  Request %d: BLOCKED (remaining: %d)\n", i, resp.Remaining)
		}
	}

	if passed != 5 || blocked != 2 {
		fmt.Printf("  FAILED: Expected 5 allowed, 2 blocked. Got %d allowed, %d blocked\n", passed, blocked)
		os.Exit(1)
	}

	fmt.Println("  Sliding window works correctly")
}

func testTokenBucket(c *client.Client) {
	fmt.Println("Test 2: Token Bucket Algorithm")
	fmt.Println("Config: /api/data - limit 100/60s, burst 20")

	passed := 0
	blocked := 0

	for i := 1; i <= 25; i++ {
		resp, err := c.Check(client.CheckRequest{
			Service: "api-gateway",
			API:     "/api/data",
			IP:      "10.0.0.5",
			Headers: map[string]string{"X-User-ID": "user123"},
		})
		if err != nil {
			fmt.Printf("  Request %d: ERROR - %v\n", i, err)
			os.Exit(1)
		}

		if resp.Allowed {
			passed++
		} else {
			blocked++
		}

		if i == 20 || i == 21 || i == 25 {
			status := "ALLOWED"
			if !resp.Allowed {
				status = "BLOCKED"
			}
			fmt.Printf("  Request %d: %s (remaining: %d)\n", i, status, resp.Remaining)
		}
	}

	if passed != 20 || blocked != 5 {
		fmt.Printf("  FAILED: Expected 20 allowed (burst), 5 blocked. Got %d allowed, %d blocked\n", passed, blocked)
		os.Exit(1)
	}

	fmt.Println("  Token bucket burst limit works correctly")

	time.Sleep(2 * time.Second)
	fmt.Println("  Waiting 2s for token refill...")

	resp, err := c.Check(client.CheckRequest{
		Service: "api-gateway",
		API:     "/api/data",
		IP:      "10.0.0.5",
		Headers: map[string]string{"X-User-ID": "user123"},
	})
	if err != nil {
		fmt.Printf("  ERROR - %v\n", err)
		os.Exit(1)
	}

	if !resp.Allowed {
		fmt.Printf("  FAILED: Request should be allowed after refill period\n")
		os.Exit(1)
	}

	fmt.Printf("  After refill: ALLOWED (remaining: %d)\n", resp.Remaining)
	fmt.Println("  Token bucket refill works correctly")
}

func testHeaderStrategy(c *client.Client) {
	fmt.Println("Test 3: Header-based Key Strategy")
	fmt.Println("Config: /payment/process - limit 10/300s per X-Session-ID")

	session1Passed := 0
	session2Passed := 0

	for i := 1; i <= 12; i++ {
		resp, err := c.Check(client.CheckRequest{
			Service: "payment-service",
			API:     "/payment/process",
			IP:      "203.0.113.50",
			Headers: map[string]string{"X-Session-ID": "session-AAA"},
		})
		if err != nil {
			fmt.Printf("  Session AAA Request %d: ERROR - %v\n", i, err)
			os.Exit(1)
		}
		if resp.Allowed {
			session1Passed++
		}
	}

	for i := 1; i <= 8; i++ {
		resp, err := c.Check(client.CheckRequest{
			Service: "payment-service",
			API:     "/payment/process",
			IP:      "203.0.113.50",
			Headers: map[string]string{"X-Session-ID": "session-BBB"},
		})
		if err != nil {
			fmt.Printf("  Session BBB Request %d: ERROR - %v\n", i, err)
			os.Exit(1)
		}
		if resp.Allowed {
			session2Passed++
		}
	}

	fmt.Printf("  Session AAA: %d/12 allowed\n", session1Passed)
	fmt.Printf("  Session BBB: %d/8 allowed\n", session2Passed)

	if session1Passed != 10 {
		fmt.Printf("  FAILED: Session AAA should allow exactly 10 requests, got %d\n", session1Passed)
		os.Exit(1)
	}

	if session2Passed != 8 {
		fmt.Printf("  FAILED: Session BBB should allow all 8 requests, got %d\n", session2Passed)
		os.Exit(1)
	}

	fmt.Println("  Header-based isolation works correctly")
}
