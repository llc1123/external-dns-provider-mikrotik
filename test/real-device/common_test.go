package real_device

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mirceanton/external-dns-provider-mikrotik/internal/mikrotik"
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/external-dns/endpoint"
)

// Test configuration variables - obtained from environment variables
var (
	// TestDomainPrefix ensures that production DNS records are not affected
	TestDomainPrefix string

	// TestComment identifier for testing purposes
	TestComment string

	// TestTimeout for tests
	TestTimeout time.Duration
)

// initTestConfig initializes the test configuration
func initTestConfig() {
	TestDomainPrefix = getEnvOrDefault("TEST_DOMAIN_PREFIX", "test-external-dns-")
	TestComment = getEnvOrDefault("TEST_COMMENT", "external-dns-e2e-test")

	timeoutStr := getEnvOrDefault("TEST_TIMEOUT", "30s")
	if timeout, err := time.ParseDuration(timeoutStr); err == nil {
		TestTimeout = timeout
	} else {
		TestTimeout = 30 * time.Second
	}
}

// RealDeviceTestSuite is a test suite for real devices
type RealDeviceTestSuite struct {
	client   *mikrotik.MikrotikApiClient
	config   *mikrotik.MikrotikConnectionConfig
	defaults *mikrotik.MikrotikDefaults
	t        *testing.T
}

// NewRealDeviceTestSuite creates a new real device test suite
func NewRealDeviceTestSuite(t *testing.T) *RealDeviceTestSuite {
	// First, set the log level to debug to see environment loading information
	log.SetLevel(log.DebugLevel)

	// Try to load the .env file (if it exists)
	loadEnvFile()

	// Initialize test configuration
	initTestConfig()

	// Note: Make sure to set environment variables or have a .env file in the project root before running tests
	// The test framework reads configuration from operating system environment variables

	// Reset log level based on environment variables
	if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
		if level, err := log.ParseLevel(logLevel); err == nil {
			log.SetLevel(level)
		}
	} else {
		log.SetLevel(log.InfoLevel)
	} // Get configuration from environment variables
	baseUrl := getEnvOrDefault("MIKROTIK_BASEURL", "http://192.168.0.1:80")
	username := getEnvOrDefault("MIKROTIK_USERNAME", "admin")
	password := getEnvOrDefault("MIKROTIK_PASSWORD", "")
	skipTLS := getEnvOrDefault("MIKROTIK_SKIP_TLS_VERIFY", "false") == "true"

	log.Debugf("Environment config loaded: BaseURL=%s, Username=%s, SkipTLS=%v", baseUrl, username, skipTLS)

	config := &mikrotik.MikrotikConnectionConfig{
		BaseUrl:       baseUrl,
		Username:      username,
		Password:      password,
		SkipTLSVerify: skipTLS,
	}

	defaults := &mikrotik.MikrotikDefaults{
		DefaultTTL:     3600,
		DefaultComment: TestComment, // Use test-specific comment
	}

	// Validate required environment variables
	if config.BaseUrl == "" || config.Username == "" || config.Password == "" {
		t.Fatal("Missing required environment variables. Please set MIKROTIK_BASEURL, MIKROTIK_USERNAME, and MIKROTIK_PASSWORD")
	}

	suite := &RealDeviceTestSuite{
		config:   config,
		defaults: defaults,
		t:        t,
	}

	return suite
}

// InitializeClient initializes the client connection
func (s *RealDeviceTestSuite) InitializeClient() error {
	client, err := mikrotik.NewMikrotikClient(s.config, s.defaults)
	if err != nil {
		return err
	}
	s.client = client
	return nil
}

// GetSystemInfo gets system information (for connectivity testing)
func (s *RealDeviceTestSuite) GetSystemInfo() (*mikrotik.MikrotikSystemInfo, error) {
	return s.client.GetSystemInfo()
}

// GetAllManagedRecords gets all records managed by external-dns
func (s *RealDeviceTestSuite) GetAllManagedRecords() ([]mikrotik.DNSRecord, error) {
	return s.client.GetDNSRecordsByName("")
}

// GetTestRecords gets all test records (records starting with the test prefix)
func (s *RealDeviceTestSuite) GetTestRecords() ([]mikrotik.DNSRecord, error) {
	allRecords, err := s.GetAllManagedRecords()
	if err != nil {
		return nil, err
	}

	var testRecords []mikrotik.DNSRecord
	for _, record := range allRecords {
		if isTestRecord(record.Name) {
			testRecords = append(testRecords, record)
		}
	}

	return testRecords, nil
}

// CleanupTestRecords cleans up all test records
func (s *RealDeviceTestSuite) CleanupTestRecords() error {
	log.Info("Cleaning up test records...")

	testRecords, err := s.GetTestRecords()
	if err != nil {
		return err
	}

	if len(testRecords) == 0 {
		log.Info("No test records to clean up")
		return nil
	}

	log.Infof("Found %d test records to clean up", len(testRecords))

	// Group records by type for cleanup
	recordsByName := make(map[string][]mikrotik.DNSRecord)
	for _, record := range testRecords {
		key := record.Name + ":" + record.Type
		recordsByName[key] = append(recordsByName[key], record)
	}

	// Create endpoints for each group of records and delete them
	for nameType, records := range recordsByName {
		if len(records) == 0 {
			continue
		}

		// Use information from the first record to create the endpoint
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

		log.Infof("Deleting test records for %s (%s)", firstRecord.Name, firstRecord.Type)
		if err := s.client.DeleteDNSRecords(endpoint); err != nil {
			log.Errorf("Failed to delete test records for %s: %v", nameType, err)
			// Continue cleaning up other records, do not stop on a single error
		}
	}

	log.Info("Test records cleanup completed")
	return nil
}

// ValidateNoProductionImpact verifies that no production records were affected
func (s *RealDeviceTestSuite) ValidateNoProductionImpact() error {
	allRecords, err := s.GetAllManagedRecords()
	if err != nil {
		return err
	}

	// Check if any non-test records were accidentally modified
	for _, record := range allRecords {
		if !isTestRecord(record.Name) && record.Comment == TestComment {
			return fmt.Errorf("SECURITY VIOLATION: Production record %s has test comment", record.Name)
		}
	}

	return nil
}

// AssertRecordExists asserts that a record exists
func (s *RealDeviceTestSuite) AssertRecordExists(name, recordType, target string) error {
	records, err := s.client.GetDNSRecordsByName(name)
	if err != nil {
		return err
	}

	for _, record := range records {
		if record.Type == recordType && getRecordTarget(&record) == target {
			return nil // Record exists
		}
	}

	return fmt.Errorf("record not found: %s %s %s", name, recordType, target)
}

// AssertRecordNotExists asserts that a record does not exist
func (s *RealDeviceTestSuite) AssertRecordNotExists(name, recordType, target string) error {
	records, err := s.client.GetDNSRecordsByName(name)
	if err != nil {
		return err
	}

	for _, record := range records {
		if record.Type == recordType && getRecordTarget(&record) == target {
			return fmt.Errorf("record should not exist but found: %s %s %s", name, recordType, target)
		}
	}

	return nil // Record does not exist
}

// getEnvOrDefault gets an environment variable or a default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// isTestRecord checks if a record is a test record
func isTestRecord(name string) bool {
	// Check if the domain name contains the test prefix, supporting subdomains like mail.test-external-dns-mixed.example.com
	return len(name) >= len(TestDomainPrefix) &&
		(name[:len(TestDomainPrefix)] == TestDomainPrefix || // Starts directly with the test prefix
			strings.Contains(name, "."+TestDomainPrefix) || // Contains the test prefix as a subdomain part
			strings.Contains(name, TestDomainPrefix)) // Contains the test prefix anywhere in the domain name
}

// getRecordTarget extracts the target value from a DNS record
func getRecordTarget(record *mikrotik.DNSRecord) string {
	switch record.Type {
	case "A", "AAAA":
		return record.Address
	case "CNAME":
		return record.CName
	case "TXT":
		return record.Text
	case "MX":
		// MX record needs to include priority: format "priority exchange"
		return fmt.Sprintf("%s %s", record.MXPreference, record.MXExchange)
	case "SRV":
		// SRV record needs to include the full format: "priority weight port target"
		return fmt.Sprintf("%s %s %s %s", record.SrvPriority, record.SrvWeight, record.SrvPort, record.SrvTarget)
	case "NS":
		return record.NS
	default:
		return ""
	}
}

// loadEnvFile tries to load the .env file
func loadEnvFile() {
	// Try to load .env file from the current directory and project root
	possiblePaths := []string{
		".env",
		"../.env",
		"../../.env",
		"../../../.env",
	}

	for _, path := range possiblePaths {
		log.Debugf("Trying to load env file: %s", path)
		if file, err := os.Open(path); err == nil {
			defer file.Close()
			log.Debugf("Loading environment variables from: %s", path)

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
					// Only set environment variables that are not already set
					if os.Getenv(key) == "" {
						os.Setenv(key, value)
						log.Debugf("Set env var: %s=%s", key, value)
					} else {
						log.Debugf("Env var already set: %s=%s", key, os.Getenv(key))
					}
				}
			}
			return // File found and loaded, exit
		}
	}

	log.Debug("No .env file found, using system environment variables")
}

// GenerateTestDomainName generates a test domain name
func GenerateTestDomainName(suffix string) string {
	return TestDomainPrefix + suffix + ".example.com"
}

// WaitForDNSPropagation waits for DNS propagation (might take some time on a real device)
func (s *RealDeviceTestSuite) WaitForDNSPropagation() {
	time.Sleep(1 * time.Second) // MikroTik is usually fast, but allow for some buffer time
}
