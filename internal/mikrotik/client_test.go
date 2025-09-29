// client_test.go
package mikrotik

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"sigs.k8s.io/external-dns/endpoint"
)

var (
	mockUsername = "testuser"
	mockPassword = "testpass"
)

func TestNewMikrotikClient(t *testing.T) {
	config := &MikrotikConnectionConfig{
		BaseUrl:       "https://192.168.88.1:443",
		Username:      "admin",
		Password:      "password",
		SkipTLSVerify: true,
	}

	defaults := &MikrotikDefaults{
		DefaultTTL:     1900,
		DefaultComment: "test",
	}

	client, err := NewMikrotikClient(config, defaults)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if client.MikrotikConnectionConfig != config {
		t.Errorf("Expected config to be %v, got %v", config, client.MikrotikConnectionConfig)
	}

	if client.MikrotikDefaults != defaults {
		t.Errorf("Expected defaults to be %v, got %v", defaults, client.MikrotikDefaults)
	}

	if client.Client == nil {
		t.Errorf("Expected HTTP client to be initialized")
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Errorf("Expected Transport to be *http.Transport")
	}

	if transport.TLSClientConfig == nil {
		t.Errorf("Expected TLSClientConfig to be set")
	} else if !transport.TLSClientConfig.InsecureSkipVerify {
		t.Errorf("Expected InsecureSkipVerify to be true")
	}
}

func TestGetSystemInfo(t *testing.T) {
	mockServerInfo := MikrotikSystemInfo{
		ArchitectureName:     "arm64",
		BadBlocks:            "0.1",
		BoardName:            "RB5009UG+S+",
		BuildTime:            "2024-09-20 13:00:27",
		CPU:                  "ARM64",
		CPUCount:             "4",
		CPUFrequency:         "1400",
		CPULoad:              "0",
		FactorySoftware:      "7.4.1",
		FreeHDDSpace:         "1019346944",
		FreeMemory:           "916791296",
		Platform:             "MikroTik",
		TotalHDDSpace:        "1073741824",
		TotalMemory:          "1073741824",
		Uptime:               "4d19h9m34s",
		Version:              "7.16 (stable)",
		WriteSectSinceReboot: "5868",
		WriteSectTotal:       "131658",
	}

	// Set up mock server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate the Basic Auth header
		username, password, ok := r.BasicAuth()
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if username != mockUsername || password != mockPassword {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// Return dummy data for /rest/system/resource
		if r.URL.Path == "/rest/system/resource" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(w).Encode(mockServerInfo)
			if err != nil {
				t.Errorf("error json encoding server info")
			}
			return
		}

		// Return 404 for any other path
		http.NotFound(w, r)
	}))
	defer server.Close()

	// Define test cases
	testCases := []struct {
		name          string
		config        MikrotikConnectionConfig
		defaults      MikrotikDefaults
		expectedError bool
	}{
		{
			name: "Valid credentials",
			config: MikrotikConnectionConfig{
				BaseUrl:       server.URL,
				Username:      mockUsername,
				Password:      mockPassword,
				SkipTLSVerify: true,
			},
			defaults:      MikrotikDefaults{},
			expectedError: false,
		},
		{
			name: "Incorrect password",
			config: MikrotikConnectionConfig{
				BaseUrl:       server.URL,
				Username:      mockUsername,
				Password:      "wrongpass",
				SkipTLSVerify: true,
			},
			defaults:      MikrotikDefaults{},
			expectedError: true,
		},
		{
			name: "Incorrect username",
			config: MikrotikConnectionConfig{
				BaseUrl:       server.URL,
				Username:      "wronguser",
				Password:      mockPassword,
				SkipTLSVerify: true,
			},
			defaults:      MikrotikDefaults{},
			expectedError: true,
		},
		{
			name: "Incorrect username and password",
			config: MikrotikConnectionConfig{
				BaseUrl:       server.URL,
				Username:      "wronguser",
				Password:      "wrongpass",
				SkipTLSVerify: true,
			},
			defaults:      MikrotikDefaults{},
			expectedError: true,
		},
		{
			name: "Missing credentials",
			config: MikrotikConnectionConfig{
				BaseUrl:       server.URL,
				Username:      "",
				Password:      "",
				SkipTLSVerify: true,
			},
			defaults:      MikrotikDefaults{},
			expectedError: true,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := &tc.config
			defaults := &tc.defaults

			client, err := NewMikrotikClient(config, defaults)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			info, err := client.GetSystemInfo()

			if tc.expectedError {
				if err == nil {
					t.Fatalf("Expected error due to unauthorized access, got none")
				}
				if info != nil {
					t.Errorf("Expected no system info, got %v", info)
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if info.ArchitectureName != mockServerInfo.ArchitectureName {
					t.Errorf("Expected ArchitectureName %s, got %s", mockServerInfo.ArchitectureName, info.ArchitectureName)
				}
				if info.Version != mockServerInfo.Version {
					t.Errorf("Expected Version %s, got %s", mockServerInfo.Version, info.Version)
				}
				// i think there's no point in checking any more fields
			}
		})
	}
}

func TestGetAllDNSRecords(t *testing.T) {
	testCases := []struct {
		name         string
		records      []DNSRecord
		expectError  bool
		unauthorized bool
	}{
		{
			name: "Multiple DNS records",
			records: []DNSRecord{
				{
					ID:      "*1",
					Address: "192.168.88.1",
					Comment: "defconf",
					Name:    "router.lan",
					TTL:     "1d",
					Type:    "A",
				},
				{
					ID:      "*3",
					Address: "1.2.3.4",
					Comment: "test A-Record",
					Name:    "example.com",
					TTL:     "1d",
					Type:    "A",
				},
				{
					ID:      "*4",
					CName:   "example.com",
					Comment: "test CNAME",
					Name:    "subdomain.example.com",
					TTL:     "1d",
					Type:    "CNAME",
				},
				{
					ID:      "*5",
					Address: "::1",
					Comment: "test AAAA",
					Name:    "test quad-A",
					TTL:     "1d",
					Type:    "AAAA",
				},
				{
					ID:      "*6",
					Comment: "test TXT",
					Name:    "example.com",
					Text:    "lorem ipsum",
					TTL:     "1d",
					Type:    "TXT",
				},
			},
		},
		{
			name:    "No DNS records",
			records: []DNSRecord{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Basic Auth validation
				username, password, ok := r.BasicAuth()
				if !ok || username != mockUsername || password != mockPassword || tc.unauthorized {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}

				// Handle GET requests to /rest/ip/dns/static
				if r.Method == http.MethodGet && r.URL.Path == "/rest/ip/dns/static" {
					w.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(w).Encode(tc.records); err != nil {
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
						return
					}
					return
				}

				// Return 404 for any other path
				http.NotFound(w, r)
			}))
			defer server.Close()

			// Set up the client
			config := &MikrotikConnectionConfig{
				BaseUrl:       server.URL,
				Username:      mockUsername,
				Password:      mockPassword,
				SkipTLSVerify: true,
			}
			defaults := &MikrotikDefaults{}
			client, err := NewMikrotikClient(config, defaults)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			records, err := client.GetAllDNSRecords()

			if tc.expectError {
				if err == nil {
					t.Fatalf("Expected error, got none")
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}

				// Verify the number of records
				if len(records) != len(tc.records) {
					t.Fatalf("Expected %d records, got %d", len(tc.records), len(records))
				}

				// Compare records if there are any
				if len(tc.records) > 0 {
					expectedRecordsMap := make(map[string]DNSRecord)
					for _, rec := range tc.records {
						key := rec.Name + "|" + rec.Type
						expectedRecordsMap[key] = rec
					}

					for _, record := range records {
						key := record.Name + "|" + record.Type
						expectedRecord, exists := expectedRecordsMap[key]
						if !exists {
							t.Errorf("Unexpected record found: %v", record)
							continue
						}
						// Compare fields
						if record.ID != expectedRecord.ID {
							t.Errorf("Expected ID '%s', got '%s' for record %s", expectedRecord.ID, record.ID, key)
						}
						switch record.Type {
						case "A", "AAAA":
							if record.Address != expectedRecord.Address {
								t.Errorf("Expected Address '%s', got '%s' for record %s", expectedRecord.Address, record.Address, key)
							}
						case "CNAME":
							if record.CName != expectedRecord.CName {
								t.Errorf("Expected CName '%s', got '%s' for record %s", expectedRecord.CName, record.CName, key)
							}
						case "TXT":
							if record.Text != expectedRecord.Text {
								t.Errorf("Expected Text '%s', got '%s' for record %s", expectedRecord.Text, record.Text, key)
							}
						default:
							t.Errorf("Unsupported RecordType '%s' for record %s", record.Type, key)
						}
					}
				}
			}
		})
	}
}

func TestDeleteDNSRecords(t *testing.T) {
	testCases := []struct {
		name              string
		endpoint          *endpoint.Endpoint
		existingRecords   []DNSRecord
		defaultComment    string
		expectError       bool
		expectedDeletions int
	}{
		{
			name: "Successful deletion of single record",
			endpoint: &endpoint.Endpoint{
				DNSName:    "example.com",
				RecordType: "A",
				Targets:    []string{"1.2.3.4"},
			},
			existingRecords: []DNSRecord{
				{
					ID:      "*1",
					Name:    "example.com",
					Type:    "A",
					Address: "1.2.3.4",
					Comment: "external-dns",
				},
			},
			defaultComment:    "external-dns",
			expectError:       false,
			expectedDeletions: 1,
		},
		{
			name: "Successful deletion of multiple records",
			endpoint: &endpoint.Endpoint{
				DNSName:    "multi.example.com",
				RecordType: "A",
				Targets:    []string{"1.2.3.4", "5.6.7.8"},
			},
			existingRecords: []DNSRecord{
				{
					ID:      "*1",
					Name:    "multi.example.com",
					Type:    "A",
					Address: "1.2.3.4",
					Comment: "external-dns",
				},
				{
					ID:      "*2",
					Name:    "multi.example.com",
					Type:    "A",
					Address: "5.6.7.8",
					Comment: "external-dns",
				},
			},
			defaultComment:    "external-dns",
			expectError:       false,
			expectedDeletions: 2,
		},
		{
			name: "No records found to delete",
			endpoint: &endpoint.Endpoint{
				DNSName:    "nonexistent.com",
				RecordType: "A",
				Targets:    []string{"1.2.3.4"},
			},
			existingRecords:   []DNSRecord{},
			defaultComment:    "external-dns",
			expectError:       false,
			expectedDeletions: 0,
		},
		{
			name: "Skip records with different comments",
			endpoint: &endpoint.Endpoint{
				DNSName:    "example.com",
				RecordType: "A",
				Targets:    []string{"1.2.3.4"},
			},
			existingRecords: []DNSRecord{
				{
					ID:      "*1",
					Name:    "example.com",
					Type:    "A",
					Address: "1.2.3.4",
					Comment: "manual-entry",
				},
			},
			defaultComment:    "external-dns",
			expectError:       false,
			expectedDeletions: 0,
		},
		{
			name: "Mix of matching and non-matching comments",
			endpoint: &endpoint.Endpoint{
				DNSName:    "mixed.example.com",
				RecordType: "A",
				Targets:    []string{"1.2.3.4"},
			},
			existingRecords: []DNSRecord{
				{
					ID:      "*1",
					Name:    "mixed.example.com",
					Type:    "A",
					Address: "1.2.3.4",
					Comment: "external-dns",
				},
				{
					ID:      "*2",
					Name:    "mixed.example.com",
					Type:    "A",
					Address: "5.6.7.8",
					Comment: "manual",
				},
			},
			defaultComment:    "external-dns",
			expectError:       false,
			expectedDeletions: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			deletedCount := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Basic Auth validation
				username, password, ok := r.BasicAuth()
				if !ok || username != mockUsername || password != mockPassword {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}

				// Handle GET requests to /rest/ip/dns/static (for GetAllDNSRecords)
				if r.Method == http.MethodGet && r.URL.Path == "/rest/ip/dns/static" {
					w.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(w).Encode(tc.existingRecords); err != nil {
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
						return
					}
					return
				}

				// Handle DELETE requests to /rest/ip/dns/static/*
				if r.Method == http.MethodDelete && len(r.URL.Path) > len("/rest/ip/dns/static/") &&
					r.URL.Path[:len("/rest/ip/dns/static/")] == "/rest/ip/dns/static/" {
					deletedCount++
					w.WriteHeader(http.StatusOK)
					return
				}

				// Return 404 for any other path
				http.NotFound(w, r)
			}))
			defer server.Close()

			// Set up the client
			config := &MikrotikConnectionConfig{
				BaseUrl:       server.URL,
				Username:      mockUsername,
				Password:      mockPassword,
				SkipTLSVerify: true,
			}
			defaults := &MikrotikDefaults{
				DefaultComment: tc.defaultComment,
			}
			client, err := NewMikrotikClient(config, defaults)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			err = client.DeleteDNSRecords(tc.endpoint)

			if tc.expectError {
				if err == nil {
					t.Fatalf("Expected error, got none")
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if deletedCount != tc.expectedDeletions {
					t.Errorf("Expected %d deletions, got %d", tc.expectedDeletions, deletedCount)
				}
			}
		})
	}
}

func TestDeleteDNSRecordByID(t *testing.T) {
	testCases := []struct {
		name        string
		recordID    string
		expectError bool
		statusCode  int
	}{
		{
			name:        "Successful deletion",
			recordID:    "*1",
			expectError: false,
			statusCode:  http.StatusOK,
		},
		{
			name:        "Record not found",
			recordID:    "*999",
			expectError: true,
			statusCode:  http.StatusNotFound,
		},
		{
			name:        "Unauthorized access",
			recordID:    "*1",
			expectError: true,
			statusCode:  http.StatusUnauthorized,
		},
		{
			name:        "Server error",
			recordID:    "*1",
			expectError: true,
			statusCode:  http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Handle unauthorized test case
				if tc.statusCode == http.StatusUnauthorized {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}

				// Basic Auth validation for other cases
				username, password, ok := r.BasicAuth()
				if !ok || username != mockUsername || password != mockPassword {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}

				// Handle DELETE requests to /rest/ip/dns/static/{id}
				if r.Method == http.MethodDelete && len(r.URL.Path) > len("/rest/ip/dns/static/") &&
					r.URL.Path[:len("/rest/ip/dns/static/")] == "/rest/ip/dns/static/" {

					recordID := r.URL.Path[len("/rest/ip/dns/static/"):]

					// Simulate different response codes based on test case
					if tc.statusCode == http.StatusNotFound && recordID == "*999" {
						http.Error(w, "Not Found", http.StatusNotFound)
						return
					}
					if tc.statusCode == http.StatusInternalServerError {
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
						return
					}

					w.WriteHeader(http.StatusOK)
					return
				}

				// Return 404 for any other path
				http.NotFound(w, r)
			}))
			defer server.Close()

			// Set up the client
			config := &MikrotikConnectionConfig{
				BaseUrl:       server.URL,
				Username:      mockUsername,
				Password:      mockPassword,
				SkipTLSVerify: true,
			}
			defaults := &MikrotikDefaults{}
			client, err := NewMikrotikClient(config, defaults)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			err = client.DeleteDNSRecordByID(tc.recordID)

			if tc.expectError {
				if err == nil {
					t.Fatalf("Expected error, got none")
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
			}
		})
	}
}

func TestCreateDNSRecords(t *testing.T) {
	testCases := []struct {
		name            string
		endpoint        *endpoint.Endpoint
		expectError     bool
		expectedRecords int
		statusCode      int
	}{
		{
			name: "Successful creation of single A record",
			endpoint: &endpoint.Endpoint{
				DNSName:    "example.com",
				RecordType: "A",
				Targets:    []string{"1.2.3.4"},
				RecordTTL:  endpoint.TTL(3600),
			},
			expectError:     false,
			expectedRecords: 1,
			statusCode:      http.StatusOK,
		},
		{
			name: "Successful creation of multiple A records",
			endpoint: &endpoint.Endpoint{
				DNSName:    "multi.example.com",
				RecordType: "A",
				Targets:    []string{"1.2.3.4", "5.6.7.8"},
				RecordTTL:  endpoint.TTL(3600),
			},
			expectError:     false,
			expectedRecords: 2,
			statusCode:      http.StatusOK,
		},
		{
			name: "API error during creation",
			endpoint: &endpoint.Endpoint{
				DNSName:    "error.example.com",
				RecordType: "A",
				Targets:    []string{"1.2.3.4"},
				RecordTTL:  endpoint.TTL(3600),
			},
			expectError:     true,
			expectedRecords: 0,
			statusCode:      http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			createdCount := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Basic Auth validation
				username, password, ok := r.BasicAuth()
				if !ok || username != mockUsername || password != mockPassword {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}

				// Handle PUT requests to /rest/ip/dns/static
				if r.Method == http.MethodPut && r.URL.Path == "/rest/ip/dns/static" {
					if tc.statusCode == http.StatusInternalServerError {
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
						return
					}

					// Parse request body to get record data
					var record DNSRecord
					if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
						http.Error(w, "Bad Request", http.StatusBadRequest)
						return
					}

					createdCount++
					record.ID = "*1"

					w.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(w).Encode(record); err != nil {
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
						return
					}
					return
				}

				// Return 404 for any other path
				http.NotFound(w, r)
			}))
			defer server.Close()

			// Set up the client
			config := &MikrotikConnectionConfig{
				BaseUrl:       server.URL,
				Username:      mockUsername,
				Password:      mockPassword,
				SkipTLSVerify: true,
			}
			defaults := &MikrotikDefaults{
				DefaultTTL:     3600,
				DefaultComment: "external-dns",
			}
			client, err := NewMikrotikClient(config, defaults)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			records, err := client.CreateDNSRecords(tc.endpoint)

			if tc.expectError {
				if err == nil {
					t.Fatalf("Expected error, got none")
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if len(records) != tc.expectedRecords {
					t.Errorf("Expected %d records created, got %d", tc.expectedRecords, len(records))
				}
			}
		})
	}
}

func TestCreateSingleDNSRecord(t *testing.T) {
	testCases := []struct {
		name        string
		record      *DNSRecord
		expectError bool
		statusCode  int
	}{
		{
			name: "Successful creation",
			record: &DNSRecord{
				Name:    "example.com",
				Type:    "A",
				Address: "1.2.3.4",
				TTL:     "1h",
				Comment: "test",
			},
			expectError: false,
			statusCode:  http.StatusOK,
		},
		{
			name: "Server error",
			record: &DNSRecord{
				Name:    "error.com",
				Type:    "A",
				Address: "1.2.3.4",
				TTL:     "1h",
				Comment: "test",
			},
			expectError: true,
			statusCode:  http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Basic Auth validation
				username, password, ok := r.BasicAuth()
				if !ok || username != mockUsername || password != mockPassword {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}

				// Handle PUT requests to /rest/ip/dns/static
				if r.Method == http.MethodPut && r.URL.Path == "/rest/ip/dns/static" {
					if tc.statusCode != http.StatusOK {
						http.Error(w, http.StatusText(tc.statusCode), tc.statusCode)
						return
					}

					// Parse and echo back the record with an ID
					var record DNSRecord
					if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
						http.Error(w, "Bad Request", http.StatusBadRequest)
						return
					}
					record.ID = "*1"

					w.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(w).Encode(record); err != nil {
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
						return
					}
					return
				}

				// Return 404 for any other path
				http.NotFound(w, r)
			}))
			defer server.Close()

			// Set up the client
			config := &MikrotikConnectionConfig{
				BaseUrl:       server.URL,
				Username:      mockUsername,
				Password:      mockPassword,
				SkipTLSVerify: true,
			}
			defaults := &MikrotikDefaults{}
			client, err := NewMikrotikClient(config, defaults)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			createdRecord, err := client.createSingleDNSRecord(tc.record)

			if tc.expectError {
				if err == nil {
					t.Fatalf("Expected error, got none")
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if createdRecord == nil {
					t.Fatal("Expected created record, got nil")
				}
				if createdRecord.ID != "*1" {
					t.Errorf("Expected ID '*1', got %s", createdRecord.ID)
				}
				if createdRecord.Name != tc.record.Name {
					t.Errorf("Expected Name %s, got %s", tc.record.Name, createdRecord.Name)
				}
			}
		})
	}
}

func TestDoRequest(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		path           string
		body           string
		expectedStatus int
		expectError    bool
	}{
		{
			name:           "Successful GET request",
			method:         "GET",
			path:           "system/resource",
			body:           "",
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Successful POST request with body",
			method:         "POST",
			path:           "ip/dns/static",
			body:           `{"name":"test.com","type":"A","address":"1.2.3.4"}`,
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "404 Not Found",
			method:         "GET",
			path:           "nonexistent/path",
			body:           "",
			expectedStatus: http.StatusNotFound,
			expectError:    true,
		},
		{
			name:           "401 Unauthorized",
			method:         "GET",
			path:           "unauthorized",
			body:           "",
			expectedStatus: http.StatusUnauthorized,
			expectError:    true,
		},
		{
			name:           "500 Internal Server Error",
			method:         "GET",
			path:           "error",
			body:           "",
			expectedStatus: http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Handle special paths that simulate different status codes
				if r.URL.Path == "/rest/unauthorized" {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
				if r.URL.Path == "/rest/error" {
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}
				if r.URL.Path == "/rest/nonexistent/path" {
					http.NotFound(w, r)
					return
				}

				// Basic Auth validation for normal requests
				username, password, ok := r.BasicAuth()
				if !ok || username != mockUsername || password != mockPassword {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}

				// Handle valid requests
				if r.URL.Path == "/rest/system/resource" {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"version":"7.16"}`))
					return
				}
				if r.URL.Path == "/rest/ip/dns/static" {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"id":"*1","name":"test.com"}`))
					return
				}

				http.NotFound(w, r)
			}))
			defer server.Close()

			// Set up the client
			config := &MikrotikConnectionConfig{
				BaseUrl:       server.URL,
				Username:      mockUsername,
				Password:      mockPassword,
				SkipTLSVerify: true,
			}
			defaults := &MikrotikDefaults{}
			client, err := NewMikrotikClient(config, defaults)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			var bodyReader io.Reader
			if tc.body != "" {
				bodyReader = bytes.NewReader([]byte(tc.body))
			}

			resp, err := client.doRequest(tc.method, tc.path, bodyReader)

			if tc.expectError {
				if err == nil {
					t.Fatalf("Expected error, got none")
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if resp == nil {
					t.Fatal("Expected response, got nil")
				}
				defer resp.Body.Close()
				if resp.StatusCode != tc.expectedStatus {
					t.Errorf("Expected status %d, got %d", tc.expectedStatus, resp.StatusCode)
				}
			}
		})
	}
}
