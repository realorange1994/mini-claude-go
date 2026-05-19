package microlisp

// Foo is an exported function for testing.
func Foo(a int, b string) error {
	return nil
}

// Bar returns a string.
func Bar() string { return "hello" }

// MyType is a test struct.
type MyType struct {
	Name string
	ID   int
}

// ConfigVar is a global variable.
var ConfigVar = "default"

// MaxRetries is a constant.
const MaxRetries = 3
