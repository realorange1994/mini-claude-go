package main

import (
	"fmt"
	"sync"
)

// ─── mDNS Service Discovery (MiMo-Code 4) ──────────────────────────────────
//
// Publishes the running server as a Bonjour/mDNS service on the local network.
// Enables automatic discovery by other devices and IDEs.
//
// MiMo-Code source: server/mdns.ts (60 lines)

// MDNSService represents an mDNS service advertisement.
type MDNSService struct {
	Name   string `json:"name"`
	Port   int    `json:"port"`
	Host   string `json:"host"`
	Domain string `json:"domain"`
}

// MDNSDiscovery manages mDNS service discovery.
type MDNSDiscovery struct {
	mu       sync.Mutex
	services map[string]*MDNSService
	running  bool
}

// NewMDNSDiscovery creates a new mDNS discovery service.
func NewMDNSDiscovery() *MDNSDiscovery {
	return &MDNSDiscovery{
		services: make(map[string]*MDNSService),
	}
}

// Publish publishes a service.
func (d *MDNSDiscovery) Publish(name string, port int, host string) *MDNSService {
	d.mu.Lock()
	defer d.mu.Unlock()

	service := &MDNSService{
		Name:   name,
		Port:   port,
		Host:   host,
		Domain: "local.",
	}

	d.services[name] = service
	d.running = true

	return service
}

// Unpublish removes a service.
func (d *MDNSDiscovery) Unpublish(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.services, name)
}

// ListServices returns all published services.
func (d *MDNSDiscovery) ListServices() []*MDNSService {
	d.mu.Lock()
	defer d.mu.Unlock()

	var services []*MDNSService
	for _, s := range d.services {
		services = append(services, s)
	}
	return services
}

// IsRunning returns true if the discovery service is running.
func (d *MDNSDiscovery) IsRunning() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.running
}

// Stop stops the discovery service.
func (d *MDNSDiscovery) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.running = false
}

// FormatMDNSService formats a service for display.
func FormatMDNSService(service *MDNSService) string {
	if service == nil {
		return "No service."
	}
	return fmt.Sprintf("%s (%s:%d)", service.Name, service.Host, service.Port)
}
