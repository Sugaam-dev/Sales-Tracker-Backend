package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

type LoginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

type LoginResponse struct {
	Success bool `json:"success"`
	Data    struct {
		AccessToken string `json:"accessToken"`
	} `json:"data"`
}

func main() {
	loginReq := LoginRequest{
		Identifier: "admin@pmrgsolution.com",
		Password:   "Welcome@123",
	}

	bodyBytes, _ := json.Marshal(loginReq)
	resp, err := http.Post("http://localhost:8080/api/v1/auth/login", "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		log.Fatal("Login failed: ", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("Login failed with status %d: %s", resp.StatusCode, string(respBytes))
	}

	var loginResp LoginResponse
	json.Unmarshal(respBytes, &loginResp)

	token := loginResp.Data.AccessToken
	fmt.Println("Login successful. Token obtained.")

	// 1. Fetch leads
	reqLeads, _ := http.NewRequest("GET", "http://localhost:8080/api/v1/leads?limit=1000", nil)
	reqLeads.Header.Set("Authorization", "Bearer "+token)
	respLeads, err := http.DefaultClient.Do(reqLeads)
	if err != nil {
		log.Fatal("Fetch leads failed: ", err)
	}
	defer respLeads.Body.Close()
	leadsBytes, _ := io.ReadAll(respLeads.Body)
	fmt.Printf("Leads Response (%d): %s\n\n", respLeads.StatusCode, string(leadsBytes))

	// 2. Fetch current users
	reqUsers, _ := http.NewRequest("GET", "http://localhost:8080/api/v1/current_users/", nil)
	reqUsers.Header.Set("Authorization", "Bearer "+token)
	respUsers, err := http.DefaultClient.Do(reqUsers)
	if err != nil {
		log.Fatal("Fetch users failed: ", err)
	}
	defer respUsers.Body.Close()
	usersBytes, _ := io.ReadAll(respUsers.Body)
	fmt.Printf("Users Response (%d): %s\n", respUsers.StatusCode, string(usersBytes))
}
