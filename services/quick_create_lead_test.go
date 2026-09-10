package services

import (
	"context"
	"strings"
	"testing"

	"crm-auth-service/conf"
	"crm-auth-service/helpers"
	"crm-auth-service/models"
	"crm-auth-service/repository"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// generateWords generates a string containing exactly n space-separated words.
func generateWords(n int) string {
	words := make([]string, n)
	for i := 0; i < n; i++ {
		words[i] = "word"
	}
	return strings.Join(words, " ")
}

func TestQuickCreateLeadValidationUnits(t *testing.T) {
	// 1. Test Word Counting Helper
	t.Run("Word Counting Helper", func(t *testing.T) {
		if count := helpers.CountWords("hello    world"); count != 2 {
			t.Errorf("Expected 2 words for 'hello    world', got %d", count)
		}
		if count := helpers.CountWords("   "); count != 0 {
			t.Errorf("Expected 0 words for empty spaces, got %d", count)
		}
		if count := helpers.CountWords("one\ntwo\tthree\r\nfour"); count != 4 {
			t.Errorf("Expected 4 words for whitespace delimited string, got %d", count)
		}
		if count := helpers.CountWords(generateWords(9)); count != 9 {
			t.Errorf("Expected 9 words, got %d", count)
		}
		if count := helpers.CountWords(generateWords(10)); count != 10 {
			t.Errorf("Expected 10 words, got %d", count)
		}
		if count := helpers.CountWords(generateWords(200)); count != 200 {
			t.Errorf("Expected 200 words, got %d", count)
		}
		if count := helpers.CountWords(generateWords(201)); count != 201 {
			t.Errorf("Expected 201 words, got %d", count)
		}
	})

	// 2. Test Phone Validation & Leading Zero Rule
	t.Run("Phone Validation & Leading Zero Rule", func(t *testing.T) {
		// Valid India number
		if err := helpers.ValidatePhoneNumber("9876543210", "+91"); err != nil {
			t.Errorf("Expected valid for 9876543210 +91, got %v", err)
		}
		if err := helpers.ValidatePhoneNumber("9876543210", "IN"); err != nil {
			t.Errorf("Expected valid for 9876543210 IN, got %v", err)
		}

		// Valid US number
		if err := helpers.ValidatePhoneNumber("4155552671", "+1"); err != nil {
			t.Errorf("Expected valid for 4155552671 +1, got %v", err)
		}

		// Valid UK number
		if err := helpers.ValidatePhoneNumber("7911123456", "+44"); err != nil {
			t.Errorf("Expected valid for 7911123456 +44, got %v", err)
		}

		// Leading zero should fail
		err := helpers.ValidatePhoneNumber("0987654321", "+91")
		if err == nil || !strings.Contains(err.Error(), "must not start with 0") {
			t.Errorf("Expected leading zero rejection error, got %v", err)
		}

		// Incomplete / invalid length for India
		err = helpers.ValidatePhoneNumber("98765", "+91")
		if err == nil {
			t.Error("Expected error for incomplete number, got nil")
		}

		// Wrong length / too long for US
		err = helpers.ValidatePhoneNumber("41555526719999", "+1")
		if err == nil {
			t.Error("Expected error for excessively long US number, got nil")
		}

		// Missing phone number
		err = helpers.ValidatePhoneNumber("", "+91")
		if err == nil {
			t.Error("Expected error for empty phone, got nil")
		}

		// Non-numeric characters should fail
		err = helpers.ValidatePhoneNumber("sjgdy525865", "+91")
		if err == nil || !strings.Contains(err.Error(), "must contain only numbers") {
			t.Errorf("Expected only numbers error for 'sjgdy525865', got %v", err)
		}

		err = helpers.ValidatePhoneNumber("nhtfhgk", "+91")
		if err == nil || !strings.Contains(err.Error(), "must contain only numbers") {
			t.Errorf("Expected only numbers error for 'nhtfhgk', got %v", err)
		}
	})
}

func TestQuickCreateLeadLifecycleIntegration(t *testing.T) {
	cfg, err := conf.LoadConfig()
	if err != nil {
		t.Skip("Skipping integration test; database credentials not found in env")
		return
	}

	log := helpers.NewLogger(cfg.Server.Env)
	pool, err := conf.ConnectDB(cfg.DB, log)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	importGormStr := cfg.DB.DSN(cfg.DB.Name)
	gormDB, err := gorm.Open(postgres.Open(importGormStr), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect gorm: %v", err)
	}

	userRepo := repository.NewUserRepository(pool)
	leadRepo := repository.NewLeadRepository(pool, gormDB)
	leadService := NewLeadService(leadRepo, userRepo)

	// Clean up any test leads
	_, _ = pool.Exec(ctx, "DELETE FROM leads WHERE email LIKE 'quick_test_%'")
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM leads WHERE email LIKE 'quick_test_%'")
	}()

	// 1. Request Type Validations
	t.Run("Request Type Validation Rules", func(t *testing.T) {
		baseReq := models.CreateLeadRequest{
			CompanyName:    "ReqType Test Corp 1",
			LeadName:       "Rajesh Sharma",
			Email:          "quick_test_reqtype@company.org",
			ContactNumber:  "9876543210",
			CountryCode:    "+91",
			Priority:       "High",
			RequestDetails: generateWords(55),
		}

		// Obsolete: "Product Request" => INVALID
		req1 := baseReq
		req1.RequestType = "Product Request"
		_, err := leadService.CreateLead(ctx, uuid.Nil, models.RoleAdmin, "admin@example.com", req1)
		if err == nil {
			t.Error("Expected error for obsolete 'Product Request', got nil")
		}

		// Obsolete: "Service Request" => INVALID
		req2 := baseReq
		req2.RequestType = "Service Request"
		_, err = leadService.CreateLead(ctx, uuid.Nil, models.RoleAdmin, "admin@example.com", req2)
		if err == nil {
			t.Error("Expected error for obsolete 'Service Request', got nil")
		}

		// Valid: "IT Product" => SUCCESS
		req3 := baseReq
		req3.CompanyName = "ReqType Test Corp ITProd"
		req3.RequestType = "IT Product"
		req3.Email = "quick_test_itproduct@company.org"
		resp3, err := leadService.CreateLead(ctx, uuid.Nil, models.RoleAdmin, "admin@example.com", req3)
		if err != nil {
			t.Fatalf("Expected valid creation for 'IT Product', got err: %v", err)
		}
		if resp3.RequestType == nil || *resp3.RequestType != "IT Product" {
			t.Errorf("Expected 'IT Product', got %v", resp3.RequestType)
		}

		// Valid: "IT Service" => SUCCESS
		req4 := baseReq
		req4.CompanyName = "ReqType Test Corp ITServ"
		req4.RequestType = "IT Service"
		req4.Email = "quick_test_itservice@company.in"
		resp4, err := leadService.CreateLead(ctx, uuid.Nil, models.RoleAdmin, "admin@example.com", req4)
		if err != nil {
			t.Fatalf("Expected valid creation for 'IT Service', got err: %v", err)
		}
		if resp4.RequestType == nil || *resp4.RequestType != "IT Service" {
			t.Errorf("Expected 'IT Service', got %v", resp4.RequestType)
		}
	})

	// 2. Request Details Word Count Validations (50-200 words)
	t.Run("Request Details Word Count Limits", func(t *testing.T) {
		baseReq := models.CreateLeadRequest{
			CompanyName:   "Word Count Corp",
			LeadName:      "Ananya Sen",
			ContactNumber: "9876543210",
			CountryCode:   "+91",
			RequestType:   "IT Service",
			Priority:      "Medium",
		}

		// Empty requestDetails => INVALID
		reqEmpty := baseReq
		reqEmpty.CompanyName = "Word Count Corp Empty"
		reqEmpty.Email = "quick_test_empty@company.net"
		reqEmpty.RequestDetails = "   "
		_, err := leadService.CreateLead(ctx, uuid.Nil, models.RoleAdmin, "admin@example.com", reqEmpty)
		if err == nil {
			t.Error("Expected error for whitespace-only requestDetails, got nil")
		}

		// 9 words => INVALID
		req9 := baseReq
		req9.CompanyName = "Word Count Corp 9"
		req9.Email = "quick_test_9@company.net"
		req9.RequestDetails = generateWords(9)
		_, err = leadService.CreateLead(ctx, uuid.Nil, models.RoleAdmin, "admin@example.com", req9)
		if err == nil || !strings.Contains(err.Error(), "between 10 and 200 words") {
			t.Errorf("Expected 'between 10 and 200 words' error for 9 words, got: %v", err)
		}

		// 10 words => VALID
		req10 := baseReq
		req10.CompanyName = "Word Count Corp 10"
		req10.Email = "quick_test_10@company.net"
		req10.RequestDetails = generateWords(10)
		resp10, err := leadService.CreateLead(ctx, uuid.Nil, models.RoleAdmin, "admin@example.com", req10)
		if err != nil {
			t.Errorf("Expected 10 words to be valid, got err: %v", err)
		} else if resp10.RequestDetails == nil || *resp10.RequestDetails != req10.RequestDetails {
			t.Errorf("RequestDetails not preserved on response")
		}

		// 200 words with extra spaces and newlines => VALID
		req200 := baseReq
		req200.CompanyName = "Word Count Corp 200"
		req200.Email = "quick_test_200@company.net"
		words200 := strings.Split(generateWords(200), " ")
		req200.RequestDetails = strings.Join(words200, "   \n\t  ")
		resp200, err := leadService.CreateLead(ctx, uuid.Nil, models.RoleAdmin, "admin@example.com", req200)
		if err != nil {
			t.Errorf("Expected 200 words with whitespace to be valid, got err: %v", err)
		} else if resp200.RequestDetails == nil {
			t.Errorf("RequestDetails nil for 200 words")
		}

		// 201 words => INVALID
		req201 := baseReq
		req201.CompanyName = "Word Count Corp 201"
		req201.Email = "quick_test_201@company.net"
		req201.RequestDetails = generateWords(201)
		_, err = leadService.CreateLead(ctx, uuid.Nil, models.RoleAdmin, "admin@example.com", req201)
		if err == nil || !strings.Contains(err.Error(), "between 10 and 200 words") {
			t.Errorf("Expected 'between 10 and 200 words' error for 201 words, got: %v", err)
		}
	})

	// 3. Email Format Validation
	t.Run("Email Format Validation", func(t *testing.T) {
		baseReq := models.CreateLeadRequest{
			CompanyName:    "Email Validation Corp",
			LeadName:       "Vikram Verma",
			ContactNumber:  "9876543210",
			CountryCode:    "+91",
			RequestType:    "IT Product",
			RequestDetails: generateWords(60),
			Priority:       "High",
		}

		// Valid domains (.in, .org, .net, .co.uk)
		validEmails := []struct {
			email   string
			company string
		}{
			{"quick_test_user@gmail.com", "Email Corp Gmail"},
			{"quick_test_user@company.in", "Email Corp IN"},
			{"quick_test_user@company.org", "Email Corp Org"},
			{"quick_test_user@company.net", "Email Corp Net"},
			{"quick_test_user@company.co.uk", "Email Corp UK"},
		}
		for _, item := range validEmails {
			req := baseReq
			req.CompanyName = item.company
			req.Email = item.email
			_, err := leadService.CreateLead(ctx, uuid.Nil, models.RoleAdmin, "admin@example.com", req)
			if err != nil {
				t.Errorf("Expected email %s to be valid, got err: %v", item.email, err)
			}
		}

		// Invalid emails
		invalidEmails := []string{
			"abc",
			"abc@",
			"@company.com",
			"user@.com",
			"",
		}
		for idx, email := range invalidEmails {
			req := baseReq
			req.CompanyName = "Invalid Email Corp"
			req.Email = email
			_, err := leadService.CreateLead(ctx, uuid.Nil, models.RoleAdmin, "admin@example.com", req)
			if err == nil {
				t.Errorf("Expected invalid email %s to fail, but got success (idx: %d)", email, idx)
			}
		}
	})

	// 4. Phone Validation & Leading Zero Rule
	t.Run("Phone Validation & Leading Zero Rule in CreateLead", func(t *testing.T) {
		baseReq := models.CreateLeadRequest{
			CompanyName:    "Telco Systems",
			LeadName:       "Priya Patel",
			Email:          "quick_test_phone_zero@company.com",
			RequestType:    "IT Product",
			RequestDetails: generateWords(52),
			Priority:       "Urgent",
			CountryCode:    "+91",
		}

		// Leading Zero => INVALID
		baseReq.ContactNumber = "0987654321"
		_, err := leadService.CreateLead(ctx, uuid.Nil, models.RoleAdmin, "admin@example.com", baseReq)
		if err == nil || !strings.Contains(err.Error(), "must not start with 0") {
			t.Errorf("Expected 'must not start with 0' error, got: %v", err)
		}

		// Valid India Number => VALID
		baseReq.Email = "quick_test_phone_ok@company.com"
		baseReq.ContactNumber = "9876543210"
		resp, err := leadService.CreateLead(ctx, uuid.Nil, models.RoleAdmin, "admin@example.com", baseReq)
		if err != nil {
			t.Fatalf("Expected valid phone to succeed, got: %v", err)
		}
		if resp.Phone == nil || *resp.Phone != "9876543210" {
			t.Errorf("Phone not persisted properly: %v", resp.Phone)
		}
	})

	// 5. Verify Priority and Database Column Persistence
	t.Run("Priority and RequestDetails Persisted in DB", func(t *testing.T) {
		details := generateWords(55)
		req := models.CreateLeadRequest{
			CompanyName:    "Zenith Enterprise",
			LeadName:       "Arjun Roy",
			Email:          "quick_test_db_persist@zenith.org",
			ContactNumber:  "9876543210",
			CountryCode:    "+91",
			RequestType:    "IT Service",
			RequestDetails: details,
			Priority:       "High",
		}

		created, err := leadService.CreateLead(ctx, uuid.Nil, models.RoleAdmin, "admin@example.com", req)
		if err != nil {
			t.Fatalf("CreateLead failed: %v", err)
		}

		// Fetch raw from database to verify schema and actual values
		var dbRequestDetails, dbRequestType, dbPriority string
		err = pool.QueryRow(ctx, "SELECT request_details, request_type, priority FROM leads WHERE lead_id = $1", created.ID).
			Scan(&dbRequestDetails, &dbRequestType, &dbPriority)
		if err != nil {
			t.Fatalf("Direct DB query failed: %v", err)
		}

		if dbRequestDetails != details {
			t.Errorf("DB request_details mismatch! Expected %s, got %s", details, dbRequestDetails)
		}
		if dbRequestType != "IT Service" {
			t.Errorf("DB request_type mismatch! Expected 'IT Service', got %s", dbRequestType)
		}
		if dbPriority != "High" {
			t.Errorf("DB priority mismatch! Expected 'High', got %s", dbPriority)
		}

		// Verify product_service column no longer exists in DB schema
		var colExists bool
		err = pool.QueryRow(ctx, `SELECT EXISTS (
			SELECT FROM information_schema.columns 
			WHERE table_name = 'leads' AND column_name = 'product_service'
		)`).Scan(&colExists)
		if err != nil {
			t.Fatalf("Column check query failed: %v", err)
		}
		if colExists {
			t.Error("product_service column still exists in leads table! Expected it to be dropped.")
		}
	})
}
