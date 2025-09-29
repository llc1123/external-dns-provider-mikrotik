package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mirceanton/external-dns-provider-mikrotik/internal/mikrotik"
	"sigs.k8s.io/external-dns/endpoint"
)

// Test configuration variables - loaded from environment variables
var (
	TestDomainPrefix string
	TestComment      string
)

func main() {
	fmt.Println("=== MikroTik Test Records Cleanup Tool ===")

	// Load .env file
	loadEnvFile()

	// Initialize test configuration
	TestDomainPrefix = getEnvOrDefault("TEST_DOMAIN_PREFIX", "test-external-dns-")
	TestComment = getEnvOrDefault("TEST_COMMENT", "external-dns-e2e-test")

	// Get configuration from environment variables
	config := &mikrotik.MikrotikConnectionConfig{
		BaseUrl:       getEnvOrDefault("MIKROTIK_BASEURL", "http://192.168.0.1:80"),
		Username:      getEnvOrDefault("MIKROTIK_USERNAME", ""),
		Password:      getEnvOrDefault("MIKROTIK_PASSWORD", ""),
		SkipTLSVerify: getEnvOrDefault("MIKROTIK_SKIP_TLS_VERIFY", "false") == "true",
	}

	defaults := &mikrotik.MikrotikDefaults{
		DefaultTTL:     3600,
		DefaultComment: TestComment,
	}

	// Validate required environment variables
	if config.BaseUrl == "" || config.Username == "" || config.Password == "" {
		log.Fatal("Missing required environment variables. Please set MIKROTIK_BASEURL, MIKROTIK_USERNAME, and MIKROTIK_PASSWORD")
	}

	// Create client
	client, err := mikrotik.NewMikrotikClient(config, defaults)
	if err != nil {
		log.Fatalf("Failed to create MikroTik client: %v", err)
	}

	// Connection test
	fmt.Println("Testing connection to MikroTik device...")
	systemInfo, err := client.GetSystemInfo()
	if err != nil {
		log.Fatalf("Failed to connect to MikroTik device: %v", err)
	}

	fmt.Printf("Connected to: %s running RouterOS %s\n", systemInfo.BoardName, systemInfo.Version)

	// Get all records
	fmt.Println("Fetching all DNS records...")
	allRecords, err := client.GetDNSRecordsByName("")
	if err != nil {
		log.Fatalf("Failed to fetch DNS records: %v", err)
	}

	fmt.Printf("Found %d total DNS records managed by external-dns\n", len(allRecords))

	// Filter test records
	var testRecords []mikrotik.DNSRecord
	for _, record := range allRecords {
		if isTestRecord(record.Name) {
			testRecords = append(testRecords, record)
		}
	}

	if len(testRecords) == 0 {
		fmt.Println("✓ No test records found. Environment is clean.")
		os.Exit(0)
	}

	fmt.Printf("⚠️  Found %d test records to clean up:\n", len(testRecords))

	// Show records to be deleted
	recordsByName := make(map[string][]mikrotik.DNSRecord)
	for _, record := range testRecords {
		key := record.Name + ":" + record.Type
		recordsByName[key] = append(recordsByName[key], record)
		fmt.Printf("  - %s %s -> %s\n", record.Type, record.Name, getRecordTarget(&record))
	}

	// Confirm deletion
	fmt.Print("\nDo you want to delete these test records? (y/N): ")
	var response string
	fmt.Scanln(&response)

	if response != "y" && response != "Y" && response != "yes" && response != "Yes" {
		fmt.Println("Cleanup cancelled.")
		os.Exit(0)
	}

	// Execute deletion
	fmt.Println("\nDeleting test records...")
	deletedCount := 0

	for nameType, records := range recordsByName {
		if len(records) == 0 {
			continue
		}

		// Use first record's information to create endpoint
		firstRecord := records[0]
		var targets []string
		for _, record := range records {
			target := getRecordTarget(&record)
			if target != "" {
				targets = append(targets, target)
			}
		}

		if len(targets) == 0 {
			continue
		}

		endpoint := &endpoint.Endpoint{
			DNSName:    firstRecord.Name,
			RecordType: firstRecord.Type,
			Targets:    targets,
		}

		fmt.Printf("Deleting %s (%s) with %d targets...", firstRecord.Name, firstRecord.Type, len(targets))

		err := client.DeleteDNSRecords(endpoint)
		if err != nil {
			fmt.Printf(" FAILED: %v\n", err)
			log.Printf("Failed to delete records for %s: %v", nameType, err)
		} else {
			fmt.Printf(" SUCCESS\n")
			deletedCount += len(records)
		}
	}

	fmt.Printf("\n✓ Cleanup completed. Deleted %d test records.\n", deletedCount)

	// Verify cleanup results
	fmt.Println("Verifying cleanup...")
	remainingRecords, err := client.GetDNSRecordsByName("")
	if err != nil {
		log.Printf("Warning: Could not verify cleanup: %v", err)
		os.Exit(0)
	}

	remainingTestRecords := 0
	for _, record := range remainingRecords {
		if isTestRecord(record.Name) {
			remainingTestRecords++
		}
	}

	if remainingTestRecords > 0 {
		fmt.Printf("⚠️  Warning: %d test records still remain\n", remainingTestRecords)
		os.Exit(1)
	} else {
		fmt.Println("✓ Verification complete. No test records remain.")
	}
}

// getEnvOrDefault gets environment variable or default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// isTestRecord checks if it's a test record
func isTestRecord(name string) bool {
	// Check if domain contains test prefix, supports third-level domains like mail.test-external-dns-mixed.example.com
	return len(name) >= len(TestDomainPrefix) &&
		(name[:len(TestDomainPrefix)] == TestDomainPrefix || // Directly starts with test prefix
			strings.Contains(name, "."+TestDomainPrefix) || // Subdomain contains test prefix
			strings.Contains(name, TestDomainPrefix)) // Domain contains test prefix anywhere
}

// getRecordTarget extracts target value from DNS record
func getRecordTarget(record *mikrotik.DNSRecord) string {
	switch record.Type {
	case "A", "AAAA":
		return record.Address
	case "CNAME":
		return record.CName
	case "TXT":
		return record.Text
	case "MX":
		return record.MXExchange
	case "SRV":
		return record.SrvTarget
	case "NS":
		return record.NS
	default:
		return ""
	}
}

// loadEnvFile attempts to load .env file
func loadEnvFile() {
	// Try loading .env file from current directory and project root
	possiblePaths := []string{
		".env",
		"../.env",
		"../../.env",
		"../../../.env",
	}

	for _, path := range possiblePaths {
		if file, err := os.Open(path); err == nil {
			defer file.Close()
			fmt.Printf("Loading environment variables from: %s\n", path)

			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}

				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					// Only set environment variables that haven't been set yet
					if os.Getenv(key) == "" {
						os.Setenv(key, value)
					}
				}
			}
			return
		}
	}
}
