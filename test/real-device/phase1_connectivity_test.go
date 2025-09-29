package real_device

import (
	"testing"

	log "github.com/sirupsen/logrus"
)

// TestPhase1_Connectivity Phase 1: Connectivity Test
// This test verifies the connection to a real MikroTik device without making any DNS record modifications
func TestPhase1_Connectivity(t *testing.T) {
	log.Info("=== Starting Phase 1: Connectivity Test ===")

	suite := NewRealDeviceTestSuite(t)

	// Test 1: Initialize the client
	t.Run("InitializeClient", func(t *testing.T) {
		log.Info("Testing client initialization...")

		err := suite.InitializeClient()
		if err != nil {
			t.Fatalf("Failed to initialize MikroTik client: %v", err)
		}

		log.Info("✓ Client initialized successfully")
	})

	// Test 2: Verify connection and authentication
	t.Run("ConnectionAndAuthentication", func(t *testing.T) {
		log.Info("Testing connection and authentication...")

		systemInfo, err := suite.GetSystemInfo()
		if err != nil {
			t.Fatalf("Failed to connect to MikroTik device or authenticate: %v", err)
		}

		if systemInfo == nil {
			t.Fatal("System info is nil")
		}

		log.Infof("✓ Successfully connected to the MikroTik device")
		log.Infof("  Device model: %s", systemInfo.BoardName)
		log.Infof("  RouterOS version: %s", systemInfo.Version)
		log.Infof("  Architecture: %s", systemInfo.ArchitectureName)
		log.Infof("  CPU: %s (%s cores)", systemInfo.CPU, systemInfo.CPUCount)
		log.Infof("  Memory: %s total, %s free", systemInfo.TotalMemory, systemInfo.FreeMemory)
		log.Infof("  Uptime: %s", systemInfo.Uptime)
	})

	// Test 3: Verify API endpoint availability
	t.Run("APIEndpointAvailability", func(t *testing.T) {
		log.Info("Testing API endpoint availability...")

		// Test DNS record API endpoint (read-only)
		_, err := suite.GetAllManagedRecords()
		if err != nil {
			t.Fatalf("Failed to access DNS records API endpoint: %v", err)
		}

		log.Info("✓ DNS record API endpoint is available")
	})

	// Test 4: Verify environment configuration
	t.Run("EnvironmentConfiguration", func(t *testing.T) {
		log.Info("Verifying environment configuration...")

		config := suite.config
		defaults := suite.defaults

		if config.BaseUrl == "" {
			t.Fatal("MIKROTIK_BASEURL environment variable not set")
		}
		if config.Username == "" {
			t.Fatal("MIKROTIK_USERNAME environment variable not set")
		}
		if config.Password == "" {
			t.Fatal("MIKROTIK_PASSWORD environment variable not set")
		}

		log.Infof("✓ Environment configuration validated successfully")
		log.Infof("  Device URL: %s", config.BaseUrl)
		log.Infof("  Username: %s", config.Username)
		log.Infof("  Skip TLS verify: %v", config.SkipTLSVerify)
		log.Infof("  Default TTL: %d", defaults.DefaultTTL)
		log.Infof("  Default comment: %s", defaults.DefaultComment)
	})

	log.Info("=== Phase 1: Connectivity Test Completed ===")
}
