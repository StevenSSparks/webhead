package main

import (
	"fmt"
	"os"
	"strings"
)

const hostsMarker = "roost"

func hostsLine(domain, ip string) string {
	return fmt.Sprintf("%s\t%s # %s", ip, domain, hostsMarker)
}

// ensureHostsEntry appends "<ip> <domain>" to /etc/hosts if not already present.
// Requires write access (run with sudo). Returns a human-readable status.
func ensureHostsEntry(domain, ip string) string {
	const path = "/etc/hosts"
	data, err := os.ReadFile(path)
	if err != nil {
		return "could not read /etc/hosts: " + err.Error()
	}
	if strings.Contains(string(data), domain+" # "+hostsMarker) {
		return "/etc/hosts already maps " + domain
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return "could not write /etc/hosts (try: sudo roost run --setup-hosts): " + err.Error()
	}
	defer f.Close()
	if _, err := f.WriteString("\n" + hostsLine(domain, ip) + "\n"); err != nil {
		return "write failed: " + err.Error()
	}
	return fmt.Sprintf("added %s → %s to /etc/hosts (remove the line marked '%s' to undo)", domain, ip, hostsMarker)
}
