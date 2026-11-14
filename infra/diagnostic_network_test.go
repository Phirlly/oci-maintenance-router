package infra_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestTestEgressPreservesPrivateAndExistingNetworks(t *testing.T) {
	data, err := os.ReadFile("network.tf")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, kind := range []string{"vcn", "nat_gateway", "service_gateway", "route_table", "security_list", "subnet"} {
		block := checksumBlock(source, `resource "oci_core_`+kind+`" "router"`)
		if !regexp.MustCompile(`count\s*=\s*var.create_network \? 1 : 0`).MatchString(block) {
			t.Fatalf("%s must exist only for the created network, independent of mode", kind)
		}
	}
	if strings.Contains(source, `resource "oci_core_internet_gateway"`) || strings.Contains(source, "ingress_security_rules") || !regexp.MustCompile(`prohibit_public_ip_on_vnic\s*=\s*true`).MatchString(source) {
		t.Fatal("test must not add inbound or public subnet access")
	}
	routes := checksumBlock(source, `resource "oci_core_route_table" "router"`)
	for _, pattern := range []string{`destination\s*=\s*"0.0.0.0/0"`, `network_entity_id\s*=\s*oci_core_nat_gateway.router\[0\].id`, `network_entity_id\s*=\s*oci_core_service_gateway.router\[0\].id`} {
		if !regexp.MustCompile(pattern).MatchString(routes) {
			t.Fatalf("missing private outbound route: %s", pattern)
		}
	}
	security := checksumBlock(source, `resource "oci_core_security_list" "router"`)
	for _, pattern := range []string{`egress_security_rules\s*\{`, `protocol\s*=\s*"6"`, `stateless\s*=\s*false`, `min\s*=\s*443\b`, `max\s*=\s*443\b`} {
		if len(regexp.MustCompile(pattern).FindAllString(security, -1)) != 2 {
			t.Fatalf("both regional and internet egress must be stateful HTTPS: %s", pattern)
		}
	}
	if !regexp.MustCompile(`destination\s*=\s*"0.0.0.0/0"`).MatchString(security) {
		t.Fatal("PagerDuty requires outbound HTTPS beyond the regional service network")
	}
}
