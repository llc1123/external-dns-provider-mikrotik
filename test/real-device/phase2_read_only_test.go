package real_device

import (
	"testing"

	log "github.com/sirupsen/logrus"
)

// TestPhase2_SafeReadOperations Phase 2: Safe Read-only Operations Test
// This test only performs read operations and will not modify any DNS records
func TestPhase2_SafeReadOperations(t *testing.T) {
	log.Info("=== StartingPhase 2: Safe Read-only Operations Test ===")

	suite := NewRealDeviceTestSuite(t)

	// Initialize client
	err := suite.InitializeClient()
	if err != nil {
		t.Fatalf("Failed to initialize client: %v", err)
	}

	// Test 1: Read all DNS records
	t.Run("ReadAllManagedRecords", func(t *testing.T) {
		log.Info("Reading all DNS records managed by external-dns...")

		records, err := suite.GetAllManagedRecords()
		if err != nil {
			t.Fatalf("Failed to read DNS records: %v", err)
		}

		log.Infof("✓ Successfully read %d DNS records", len(records))

		// Display record statistics
		recordTypes := make(map[string]int)
		testRecordCount := 0

		for _, record := range records {
			recordTypes[record.Type]++
			if isTestRecord(record.Name) {
				testRecordCount++
			}
		}

		log.Info("Record type statistics:")
		for recordType, count := range recordTypes {
			log.Infof("  %s: %d records", recordType, count)
		}

		log.Infof("Test record count: %d records", testRecordCount)

		// Display first few records as examples (sanitized)
		if len(records) > 0 {
			log.Info("Example records (showing up to 5):")
			maxDisplay := 5
			if len(records) < maxDisplay {
				maxDisplay = len(records)
			}

			for i := 0; i < maxDisplay; i++ {
				record := records[i]
				target := getRecordTarget(&record)
				// Sanitized: Do not display full domain names and IP addresses
				safeName := maskDomainName(record.Name)
				safeTarget := maskTarget(target)
				log.Infof("  [%d] %s %s -> %s (TTL: %s)",
					i+1, record.Type, safeName, safeTarget, record.TTL)
			}
		}
	})

	// Test 2: Test record filtering functionality
	t.Run("TestRecordFiltering", func(t *testing.T) {
		log.Info("Testing DNS record filtering...")

		// Get all records
		allRecords, err := suite.GetAllManagedRecords()
		if err != nil {
			t.Fatalf("Failed to read all records: %v", err)
		}

		// Test filtering by name (if there are existing records)
		if len(allRecords) > 0 {
			// Use the first record for filtering test
			firstRecord := allRecords[0]

			filteredRecords, err := suite.client.GetDNSRecordsByName(firstRecord.Name)
			if err != nil {
				t.Fatalf("Failed to filter records by name: %v", err)
			}

			// Verify the filtering result
			found := false
			for _, record := range filteredRecords {
				if record.Name == firstRecord.Name && record.Type == firstRecord.Type {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("Filtering did not return the expected record")
			}

			log.Infof("✓ Record filtering is working, filtered by name '%s' and got %d records",
				maskDomainName(firstRecord.Name), len(filteredRecords))
		} else {
			log.Info("✓ No existing records to test filtering, skipping.")
		}
	})

	// Test 3: Verify record parsing functionality
	t.Run("RecordParsingValidation", func(t *testing.T) {
		log.Info("Verifying DNS record parsing functionality...")

		records, err := suite.GetAllManagedRecords()
		if err != nil {
			t.Fatalf("Failed to read records: %v", err)
		}

		// Verify parsing for each record type
		recordTypesSeen := make(map[string]bool)
		validRecords := 0

		for _, record := range records {
			recordTypesSeen[record.Type] = true

			// Verify the record has a valid target
			target := getRecordTarget(&record)
			if target != "" {
				validRecords++
			}

			// Verify required fields
			if record.Name == "" {
				t.Errorf("Record has empty name: %+v", record)
			}
			if record.Type == "" {
				t.Errorf("Record has empty type: %+v", record)
			}
			if record.ID == "" {
				t.Errorf("Record has empty ID: %+v", record)
			}
		}

		log.Infof("✓ Record parsing validation completed")
		log.Infof("  Total records: %d", len(records))
		log.Infof("  Valid records: %d", validRecords)
		log.Infof("  Record types found: %v", getMapKeys(recordTypesSeen))
	})

	// Test 4: Security validation - ensure no accidental modification of production records
	t.Run("ProductionSafetyValidation", func(t *testing.T) {
		log.Info("Verifying production environment security...")

		err := suite.ValidateNoProductionImpact()
		if err != nil {
			t.Fatalf("Production safety validation failed: %v", err)
		}

		log.Info("✓ Production environment security validation passed, no unexpected record modifications found")
	})

	// Test 5: Check for existing test records
	t.Run("CheckExistingTestRecords", func(t *testing.T) {
		log.Info("Checking for existing test records...")

		testRecords, err := suite.GetTestRecords()
		if err != nil {
			t.Fatalf("Failed to get test records: %v", err)
		}

		if len(testRecords) > 0 {
			log.Warnf("Found %d existing test records, it is recommended to clean them up before running write tests:", len(testRecords))

			// Display test record information
			for i, record := range testRecords {
				if i >= 10 { // Show at most 10 records
					log.Infof("  ... and %d more test records", len(testRecords)-10)
					break
				}
				target := getRecordTarget(&record)
				log.Infof("  %s %s -> %s", record.Type, record.Name, target)
			}

			log.Info("You can run the cleanup command: go run ./test/real-device/cleanup_test_records.go")
		} else {
			log.Info("✓ No existing test records found, the environment is clean")
		}
	})

	log.Info("=== Phase 2: Safe Read-only Operations Test Completed ===")
}

// maskDomainName masks the domain name for display
func maskDomainName(name string) string {
	if len(name) <= 10 {
		return name // Show short domain names directly
	}

	// For long domain names, only show the first and last few characters
	if isTestRecord(name) {
		return name // Test domains can be displayed in full
	}

	return name[:3] + "***" + name[len(name)-4:]
}

// maskTarget masks the target value for display
func maskTarget(target string) string {
	if len(target) <= 8 {
		return target
	}

	// Check if it is an IP address
	if isIPAddress(target) {
		parts := target
		if len(parts) > 8 {
			return parts[:4] + "***" + parts[len(parts)-2:]
		}
	}

	return target[:3] + "***" + target[len(target)-3:]
}

// isIPAddress simply checks if it is an IP address
func isIPAddress(s string) bool {
	return len(s) >= 7 && (s[0] >= '0' && s[0] <= '9') &&
		(s[len(s)-1] >= '0' && s[len(s)-1] <= '9')
}

// getMapKeys gets all the keys of the map
func getMapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
