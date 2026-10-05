package provider_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/stepbrobd/terraform-provider-arin/arin"
	"github.com/stepbrobd/terraform-provider-arin/arin/arintest"
)

func TestAccROA(t *testing.T) {
	srv := arintest.New(t, "TESTKEY", "TESTORG")
	roa := func(name, extra string) string {
		return providerConfig(srv.URL) + fmt.Sprintf(`
resource "arin_roa" "test" {
  as_number = 64496
  name      = %q
  auto_link = true
  resources = [{
    start_address = "192.0.2.0"
    cidr_length   = 24%s
  }]
}
`, name, extra)
	}
	var firstHandle string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: roa("headquarters", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("arin_roa.test", "roa_handle"),
					resource.TestCheckResourceAttr("arin_roa.test", "auto_renewed", "true"),
					resource.TestCheckResourceAttr("arin_roa.test", "resources.0.end_address", "192.0.2.255"),
					resource.TestCheckResourceAttr("arin_roa.test", "resources.0.ip_version", "4"),
					resource.TestCheckResourceAttr("arin_roa.test", "resources.0.auto_linked", "true"),
					resource.TestCheckNoResourceAttr("arin_roa.test", "resources.0.max_length"),
					func(s *terraform.State) error {
						firstHandle = s.RootModule().Resources["arin_roa.test"].Primary.Attributes["roa_handle"]
						if firstHandle == "" {
							return fmt.Errorf("empty roa_handle")
						}
						return nil
					},
				),
			},
			{
				Config: roa("renamed", "\n    max_length    = 25"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("arin_roa.test", "name", "renamed"),
					resource.TestCheckResourceAttr("arin_roa.test", "resources.0.max_length", "25"),
					func(s *terraform.State) error {
						h := s.RootModule().Resources["arin_roa.test"].Primary.Attributes["roa_handle"]
						if h == firstHandle {
							return fmt.Errorf("roa_handle unchanged after update")
						}
						return nil
					},
				),
			},
			{
				ResourceName: "arin_roa.test",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return s.RootModule().Resources["arin_roa.test"].Primary.Attributes["roa_handle"], nil
				},
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "roa_handle",
				// auto_link is write-only on the arin side
				// imports fall back to the schema default
				ImportStateVerifyIgnore: []string{"auto_link"},
			},
		},
	})
}

func TestAccROASettleRetry(t *testing.T) {
	srv := arintest.New(t, "TESTKEY", "TESTORG")
	// the pre-create listing passes, the first settlement listing
	// fails, and the retry identifies the roa
	srv.FailROAList(1, 1)
	cfg := providerConfig(srv.URL) + `
resource "arin_roa" "retry" {
  as_number = 64496
  name      = "retry"
  resources = [{
    start_address = "192.0.2.0"
    cidr_length   = 24
  }]
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check:  resource.TestCheckResourceAttrSet("arin_roa.retry", "roa_handle"),
			},
		},
	})
}

func TestAccROASettleFailureGuidance(t *testing.T) {
	srv := arintest.New(t, "TESTKEY", "TESTORG")
	// every settlement listing fails, the error must point at import
	srv.FailROAList(1, 10)
	cfg := providerConfig(srv.URL) + `
resource "arin_roa" "lost" {
  as_number = 64496
  name      = "lost"
  resources = [{
    start_address = "192.0.2.0"
    cidr_length   = 24
  }]
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config:      cfg,
				ExpectError: regexp.MustCompile(`arin_roas data source`),
			},
		},
	})
}

func TestAccROAMultiResource(t *testing.T) {
	srv := arintest.New(t, "TESTKEY", "TESTORG")
	// config order deliberately differs from the fake's canonical
	// (sorted) order to exercise refresh alignment
	cfg := providerConfig(srv.URL) + `
resource "arin_roa" "multi" {
  as_number = 64496
  name      = "multi"
  resources = [
    {
      start_address = "198.51.100.0"
      cidr_length   = 24
    },
    {
      start_address = "192.0.2.0"
      cidr_length   = 24
    },
  ]
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("arin_roa.multi", "resources.0.start_address", "198.51.100.0"),
					resource.TestCheckResourceAttr("arin_roa.multi", "resources.0.end_address", "198.51.100.255"),
					resource.TestCheckResourceAttr("arin_roa.multi", "resources.1.start_address", "192.0.2.0"),
					resource.TestCheckResourceAttr("arin_roa.multi", "resources.1.end_address", "192.0.2.255"),
					resource.TestCheckNoResourceAttr("arin_roa.multi", "resources.0.max_length"),
					resource.TestCheckNoResourceAttr("arin_roa.multi", "resources.1.max_length"),
				),
			},
		},
	})
}

func TestAccROAKeepsLinkedRoutes(t *testing.T) {
	srv := arintest.New(t, "TESTKEY", "TESTORG")
	roa := func(prefixes ...string) string {
		var rs []string
		for _, p := range prefixes {
			rs = append(rs, fmt.Sprintf("{\n    start_address = %q\n    cidr_length   = 24\n  }", p))
		}
		return providerConfig(srv.URL) + fmt.Sprintf(`
resource "arin_roa" "links" {
  as_number = 64496
  name      = "links"
  auto_link = true
  resources = [%s]
}
`, strings.Join(rs, ", "))
	}
	linked := func(prefix string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			h := s.RootModule().Resources["arin_roa.links"].Primary.Attributes["roa_handle"]
			r, ok := srv.Route(prefix, "AS64496")
			if !ok {
				return fmt.Errorf("no route object for %s", prefix)
			}
			if r.AutoLinkedRoaHandle != h {
				return fmt.Errorf("route %s linked to %q, roa is %q", prefix, r.AutoLinkedRoaHandle, h)
			}
			return nil
		}
	}
	var created string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		CheckDestroy: func(*terraform.State) error {
			for _, p := range []string{"192.0.2.0/24", "198.51.100.0/24"} {
				if _, ok := srv.Route(p, "AS64496"); ok {
					return fmt.Errorf("route %s left behind by destroy", p)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: roa("192.0.2.0"),
				Check:  linked("192.0.2.0/24"),
			},
			{
				// stands in for an attribute set by hand on the route object,
				// such as member-of, which only lives as long as the object
				PreConfig: func() {
					r, _ := srv.Route("192.0.2.0/24", "AS64496")
					r.MemberOf = []arin.NameRef{{Name: "RS-TEST"}}
					srv.InjectRoute(r)
					created = r.Created
				},
				// adding a prefix reissues the roa and must leave the existing route object in place
				Config: roa("192.0.2.0", "198.51.100.0"),
				Check: resource.ComposeAggregateTestCheckFunc(
					linked("192.0.2.0/24"),
					linked("198.51.100.0/24"),
					func(*terraform.State) error {
						r, _ := srv.Route("192.0.2.0/24", "AS64496")
						if r.Created != created || len(r.MemberOf) != 1 {
							return fmt.Errorf("route object recreated by an update that only adds a prefix, %+v", r)
						}
						return nil
					},
				),
			},
			{
				// dropping a prefix takes its route object with it
				Config: roa("198.51.100.0"),
				Check: resource.ComposeAggregateTestCheckFunc(
					linked("198.51.100.0/24"),
					func(*terraform.State) error {
						if r, ok := srv.Route("192.0.2.0/24", "AS64496"); ok {
							return fmt.Errorf("route object of a dropped prefix survived, %+v", r)
						}
						return nil
					},
				),
			},
		},
	})
}
