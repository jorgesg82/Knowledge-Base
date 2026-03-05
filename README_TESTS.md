# KB - Test Suite

Comprehensive test coverage for the Personal Knowledge Base tool.

## Running Tests

```bash
cd /tmp/kb-dev
go test -v ./...
```

## Test Files

### config_test.go
Tests for configuration management:
- Default config generation
- Save and load config
- Config with missing fields (defaults)
- KB path detection

### entry_test.go
Tests for entry parsing and generation:
- Entry template creation
- ID generation from title and category
- Title case conversion
- Write and parse entry round-trip
- Invalid frontmatter handling
- Entry path generation

### index_test.go
Tests for index operations:
- Load and save index
- Add/update/remove entries
- Index rebuilding from entries
- Entry lookup (by ID, by query)
- Partial query matching

### search_test.go
Tests for search functionality:
- Search by tags (single and multiple, case insensitive)
- Search by category
- Full-text search in content
- Tag and category aggregation

### edge_cases_test.go
**Critical security and robustness tests:**

#### Security Tests
- **Path traversal prevention**: Validates that malicious paths cannot escape kbPath
- **Special characters in input**: Tests unicode, emojis, special chars in tags and titles
- **Empty/whitespace queries**: Ensures no crashes on edge case inputs

#### Robustness Tests
- **Malformed frontmatter**: Invalid YAML, missing delimiters, empty frontmatter
- **Large content**: 1MB+ entries to test memory handling
- **Concurrent operations**: Tests for race conditions in index updates
- **File permission errors**: Handles read-only directories gracefully
- **Index corruption**: Recovers from corrupted JSON
- **Duplicate IDs**: Handles ID conflicts correctly
- **Time edge cases**: Zero times, invalid timestamps

## Bugs Found and Fixed

### Critical Security Issues
1. **Path Traversal Vulnerability** (FIXED)
   - GetEntryPath allowed `../../../etc/passwd` escapes
   - Fixed with path sanitization and validation

### Data Integrity Issues
2. **Invalid Frontmatter Accepted** (FIXED)
   - Parser accepted entries without proper frontmatter delimiters
   - Fixed with strict validation

3. **Missing Title Validation** (FIXED)
   - Entries without titles were accepted
   - Fixed by requiring title field

### Functionality Bugs
4. **TitleCase Broken** (FIXED)
   - Didn't convert `-` and `_` to spaces
   - Fixed to properly handle separators

5. **Content Length Mismatch** (FIXED)
   - Inconsistent newline handling
   - Fixed with consistent TrimSpace behavior

6. **Category Inference** (FIXED)
   - Categories not inferred from file path
   - Fixed to extract from entries/category/file.md pattern

## Test Coverage

**Total**: 60+ test cases covering:
- ✅ Normal operations
- ✅ Edge cases
- ✅ Error conditions
- ✅ Security vulnerabilities
- ✅ Performance (large data)
- ✅ Concurrency

## Coverage by Module

| Module | Tests | Coverage Area |
|--------|-------|---------------|
| config.go | 5 | Config lifecycle, defaults, path detection |
| entry.go | 8 | Parsing, writing, ID generation, validation |
| index.go | 9 | CRUD operations, rebuilding, lookups |
| search.go | 6 | Tags, categories, full-text, aggregation |
| Edge cases | 13 | Security, robustness, error handling |

## Running Specific Tests

```bash
# Run specific test file
go test -v -run TestConfigFile

# Run specific test function
go test -v -run TestPathTraversalPrevention

# Run with race detector
go test -race ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Adding New Tests

When adding new functionality:
1. Write the test first (TDD)
2. Include at least one edge case
3. Test error paths
4. Consider security implications

Example test structure:
```go
func TestNewFeature(t *testing.T) {
    // Setup
    tmpDir := t.TempDir()

    // Normal case
    result := NewFeature(normalInput)
    if result != expected {
        t.Errorf("expected %v, got %v", expected, result)
    }

    // Edge case
    result = NewFeature(edgeInput)
    if err == nil {
        t.Error("expected error for edge case")
    }
}
```

## Continuous Integration

Tests should be run:
- Before every commit
- On every pull request
- Before releases

All tests must pass before merging to main.
