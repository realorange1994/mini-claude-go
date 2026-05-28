package extensions

// ExtensionFactory is a factory function that produces a new Extension instance.
// Factories are registered via Register and retrieved via Get.
type ExtensionFactory func() Extension

// extensionFactories maps extension names to their factory functions.
// It is populated by init() calls in extension packages.
var extensionFactories = make(map[string]ExtensionFactory)

// Register adds an extension factory to the global registry.
// This is typically called from an init() function in an extension package:
//
//	func init() {
//	    extensions.Register("my-extension", func() Extension { return &MyExtension{} })
//	}
func Register(name string, factory ExtensionFactory) {
	extensionFactories[name] = factory
}

// Get returns a new instance of the named extension, or nil if no factory
// is registered under that name.
func Get(name string) Extension {
	if factory, ok := extensionFactories[name]; ok {
		return factory()
	}
	return nil
}

// Registered returns the names of all currently registered extension factories.
func Registered() []string {
	names := make([]string, 0, len(extensionFactories))
	for name := range extensionFactories {
		names = append(names, name)
	}
	return names
}

// Unregister removes a factory from the registry. Returns true if it existed.
func Unregister(name string) bool {
	_, existed := extensionFactories[name]
	delete(extensionFactories, name)
	return existed
}

// Count returns the number of registered extension factories.
func Count() int {
	return len(extensionFactories)
}
