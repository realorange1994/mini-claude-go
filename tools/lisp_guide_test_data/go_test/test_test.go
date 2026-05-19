package microlisp

// HelperFunc is an exported function used in testing.
func HelperFunc(a int) bool { return a > 0 }

// MockStruct is a test struct for FFI testing.
type MockStruct struct {
	Name  string
	Value int
}

// SetupMock initializes mock environment.
func SetupMock() *MockStruct {
	return &MockStruct{Name: "test", Value: 42}
}
