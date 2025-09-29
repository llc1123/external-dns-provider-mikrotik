# Real MikroTik Device Testing - Quick Start Guide

This guide will help you quickly get started with functional testing of external-dns-provider-mikrotik on real MikroTik devices.

## 🚀 Quick Start

### 1. Environment Setup

First, ensure you have an accessible MikroTik RouterOS device with REST API enabled.

#### Configure MikroTik Device
```bash
# Enable REST API on MikroTik device
/ip service set www-ssl disabled=no port=443
/ip service set www disabled=no port=80

# Create user for external-dns (recommended)
/user add name=external-dns group=full password=your_password_here
```

#### Setup Environment Variables
Create a `.env` file in the project root directory:
```bash
MIKROTIK_BASEURL=http://192.168.0.1:80
MIKROTIK_USERNAME=external-dns
MIKROTIK_PASSWORD=your_password_here
MIKROTIK_SKIP_TLS_VERIFY=true
MIKROTIK_DEFAULT_COMMENT=external-dns-e2e-test
```

### 2. Safe Testing Process

Our testing uses a phased approach, starting with safe read-only tests:

#### 🔍 Phase 1: Connectivity Testing (Completely Safe)
Test connection to device, authentication, and basic information retrieval.
```powershell
# Windows
.\test\real-device\run_tests.ps1 -Phase 1

# Linux/macOS
./test/real-device/run_tests.sh --phase 1
```

#### 📖 Phase 2: Read-Only Testing (Completely Safe)
Test DNS record reading and parsing functionality without modifying any records.
```powershell
# Windows
.\test\real-device\run_tests.ps1 -Phase 2

# Linux/macOS
./test/real-device/run_tests.sh --phase 2
```

#### ✍️ Phase 3: Controlled Write Testing (Safe Test Records)
Create and delete test records with `test-external-dns-` prefix, without affecting production records.
```powershell
# Windows
.\test\real-device\run_tests.ps1 -Phase 3

# Linux/macOS
./test/real-device/run_tests.sh --phase 3
```

#### 🔄 Phase 4: Full Lifecycle Testing (Complete Functionality)
Test complete CRUD operations for records, including updates and smart synchronization.
```powershell
# Windows
.\test\real-device\run_tests.ps1 -Phase 4

# Linux/macOS
./test/real-device/run_tests.sh --phase 4
```

#### 🎯 Run All Tests
```powershell
# Windows (Recommended: includes automatic cleanup)
.\test\real-device\run_tests.ps1 -Cleanup

# Linux/macOS
./test/real-device/run_tests.sh --cleanup
```

### 3. Safety Guarantees

✅ **Test Domain Isolation**: All tests use `test-external-dns-` prefix  
✅ **Automatic Cleanup**: Each test includes cleanup mechanisms  
✅ **Production Protection**: Strict domain filtering prevents accidental modifications  
✅ **Phased Testing**: Starting from safe read-only tests  

### 4. Common Usage

```powershell
# First time testing (recommended)
.\test\real-device\run_tests.ps1 -Phase 1 -Verbose

# Development debugging
.\test\real-device\run_tests.ps1 -Phase 3 -LogLevel debug -Cleanup

# Continuous integration
.\test\real-device\run_tests.ps1 -Cleanup

# Manual cleanup
go run .\test\real-device\tools\cleanup_test_records.go
```

### 5. Troubleshooting

If tests fail, please check:

1. **Network Connection**: Ensure MikroTik device is accessible
2. **Authentication**: Verify username and password
3. **REST API**: Ensure REST API is enabled on device
4. **Firewall**: Check device firewall rules
5. **Permissions**: Ensure user has sufficient permissions

Use verbose logging for debugging:
```powershell
.\test\real-device\run_tests.ps1 -LogLevel debug -Verbose
```

### 6. Pre-Production Checklist

Before deploying external-dns-provider-mikrotik to production, ensure:

- [ ] Phase 1 tests pass (connectivity)
- [ ] Phase 2 tests pass (read functionality)
- [ ] Phase 3 tests pass (basic write functionality)
- [ ] Phase 4 tests pass (complete functionality)
- [ ] Cleanup tools work properly
- [ ] Production domain filtering is configured correctly
- [ ] Monitoring and alerting are configured

## 📚 More Information

- See [README.md](README.md) for detailed test structure
- Check the phase test code in `test/real-device/` directory
- Use `-Help` parameter to view complete script options

## 🆘 Getting Help

If you encounter issues:

1. Review test log output
2. Use `-Verbose` and `-LogLevel debug` for detailed information
3. Check MikroTik device logs
4. Verify network connection and firewall settings
