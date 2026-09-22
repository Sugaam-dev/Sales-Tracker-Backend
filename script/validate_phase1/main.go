package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"crm-auth-service/conf"
	"crm-auth-service/helpers"

	"github.com/google/uuid"
)

type EndpointResult struct {
	Endpoint   string
	Duration   time.Duration
	StatusCode int
	Error      string
}

func main() {
	cfg, err := conf.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	jwtManager := helpers.NewJWTManager(cfg.JWT)
	// Admin user ID from database: d8e8e51d-5d49-46a1-8a9c-f7362ec87ec4
	adminID, _ := uuid.Parse("d8e8e51d-5d49-46a1-8a9c-f7362ec87ec4")
	token, err := jwtManager.GenerateAccessToken(adminID, "admin", "admin@pmrgsolution.com")
	if err != nil {
		log.Fatalf("Failed to generate access token: %v", err)
	}
	fmt.Println("=== AUTHENTICATION INITIALIZED (ADMIN JWT) ===")

	client := &http.Client{Timeout: 90 * time.Second}

	// 1. Fetch a real lead ID from the database
	reqLeadsList, _ := http.NewRequest("GET", "http://localhost:8080/api/v1/leads?limit=5", nil)
	reqLeadsList.Header.Set("Authorization", "Bearer "+token)
	respLeadsList, err := client.Do(reqLeadsList)
	leadID := "L-7001"
	if err == nil {
		defer respLeadsList.Body.Close()
		var leadsData struct {
			Data struct {
				Leads []struct {
					LeadID string `json:"lead_id"`
				} `json:"leads"`
			} `json:"data"`
		}
		decBytes, _ := io.ReadAll(respLeadsList.Body)
		json.Unmarshal(decBytes, &leadsData)
		if len(leadsData.Data.Leads) > 0 && leadsData.Data.Leads[0].LeadID != "" {
			leadID = leadsData.Data.Leads[0].LeadID
		}
	}
	fmt.Printf("Using Lead ID for commercial tests: %s\n\n", leadID)

	// 2. Sequential Validation of Slow Endpoints
	endpoints := []struct {
		Name string
		URL  string
	}{
		{"GET /commercial", fmt.Sprintf("http://localhost:8080/api/v1/leads/%s/commercial?currency=USD", leadID)},
		{"GET /analytics", "http://localhost:8080/api/v1/dashboard/summary"},
		{"GET /users", "http://localhost:8080/api/v1/users"},
		{"GET /activities", "http://localhost:8080/api/v1/activities?limit=10"},
		{"GET /leads/summary", "http://localhost:8080/api/v1/activities/summary"},
		{"GET /leads", "http://localhost:8080/api/v1/leads?limit=10"},
	}

	fmt.Println("=== 1. SEQUENTIAL ENDPOINT MEASUREMENTS ===")
	for _, ep := range endpoints {
		req, _ := http.NewRequest("GET", ep.URL, nil)
		req.Header.Set("Authorization", "Bearer "+token)

		start := time.Now()
		res, err := client.Do(req)
		dur := time.Since(start)

		if err != nil {
			fmt.Printf("Endpoint: %-20s | Time: %8.2f ms | Status: ERR (%v)\n", ep.Name, float64(dur.Milliseconds()), err)
		} else {
			io.Copy(io.Discard, res.Body)
			res.Body.Close()
			fmt.Printf("Endpoint: %-20s | Time: %8.2f ms | Status: %d\n", ep.Name, float64(dur.Milliseconds()), res.StatusCode)
		}
	}

	// 3. Realistic Page Load Burst (25 Concurrent Requests)
	fmt.Println("\n=== 2. SIMULATED PAGE LOAD BURST (25 CONCURRENT REQUESTS) ===")
	burstUrls := []string{
		fmt.Sprintf("http://localhost:8080/api/v1/leads/%s/commercial?currency=USD", leadID),
		"http://localhost:8080/api/v1/dashboard/summary",
		"http://localhost:8080/api/v1/users",
		"http://localhost:8080/api/v1/activities?limit=10",
		"http://localhost:8080/api/v1/activities/summary",
		"http://localhost:8080/api/v1/leads?limit=10",
		"http://localhost:8080/api/v1/current_users/",
		"http://localhost:8080/api/v1/master/stages",
		"http://localhost:8080/api/v1/tasks",
		"http://localhost:8080/api/v1/reports/heat-map",
		"http://localhost:8080/api/v1/auth/me/permissions",
		// Duplicates as observed on mount
		"http://localhost:8080/api/v1/users",
		"http://localhost:8080/api/v1/leads?limit=100",
		"http://localhost:8080/api/v1/activities?limit=20",
		"http://localhost:8080/api/v1/dashboard/summary",
		fmt.Sprintf("http://localhost:8080/api/v1/leads/%s/commercial?currency=USD", leadID),
		"http://localhost:8080/api/v1/master/stages",
		"http://localhost:8080/api/v1/current_users/",
		"http://localhost:8080/api/v1/leads?limit=10",
		"http://localhost:8080/api/v1/activities/summary",
		"http://localhost:8080/api/v1/tasks",
		"http://localhost:8080/api/v1/reports/heat-map",
		"http://localhost:8080/api/v1/users",
		"http://localhost:8080/api/v1/activities?limit=10",
		"http://localhost:8080/api/v1/auth/me/permissions",
	}

	var wg sync.WaitGroup
	burstStart := time.Now()
	results := make([]EndpointResult, len(burstUrls))

	for i, u := range burstUrls {
		wg.Add(1)
		go func(idx int, targetURL string) {
			defer wg.Done()
			r, _ := http.NewRequest("GET", targetURL, nil)
			r.Header.Set("Authorization", "Bearer "+token)
			st := time.Now()
			res, err := client.Do(r)
			el := time.Since(st)
			if err != nil {
				results[idx] = EndpointResult{Endpoint: targetURL, Duration: el, StatusCode: 0, Error: err.Error()}
			} else {
				io.Copy(io.Discard, res.Body)
				res.Body.Close()
				results[idx] = EndpointResult{Endpoint: targetURL, Duration: el, StatusCode: res.StatusCode}
			}
		}(i, u)
	}

	wg.Wait()
	totalBurstTime := time.Since(burstStart)
	fmt.Printf("Total Concurrent Burst Time for 25 requests: %.2f ms (%.2f seconds)\n\n", float64(totalBurstTime.Milliseconds()), totalBurstTime.Seconds())

	fmt.Println("=== 3. BURST REQUEST RESULTS (ALL 25) ===")
	for i, res := range results {
		fmt.Printf("[%2d] Status: %3d | Duration: %8.2f ms | Endpoint: %s\n", i+1, res.StatusCode, float64(res.Duration.Milliseconds()), res.Endpoint)
	}
}
