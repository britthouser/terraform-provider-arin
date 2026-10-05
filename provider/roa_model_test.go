package provider

import (
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestKeepsLinks(t *testing.T) {
	model := func(autoLink bool, as int64, prefixes ...string) *roaModel {
		out := &roaModel{AutoLink: types.BoolValue(autoLink), ASNumber: types.Int64Value(as)}
		for _, p := range prefixes {
			ip, length, _ := strings.Cut(p, "/")
			n, err := strconv.ParseInt(length, 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			out.Resources = append(out.Resources, roaResourceModel{StartAddress: types.StringValue(ip), CIDRLength: types.Int64Value(n)})
		}
		return out
	}
	for _, c := range []struct {
		name        string
		state, plan *roaModel
		want        bool
	}{
		{"added prefix", model(true, 10779, "23.161.104.0/24"), model(true, 10779, "23.161.104.0/24", "204.115.20.0/23"), true},
		{"same prefixes", model(true, 10779, "23.161.104.0/24"), model(true, 10779, "23.161.104.0/24"), true},
		{"padded state form", model(true, 10779, "023.161.104.000/24"), model(true, 10779, "204.115.20.0/23", "23.161.104.0/24"), true},
		{"dropped prefix", model(true, 10779, "23.161.104.0/24", "192.104.136.0/24"), model(true, 10779, "23.161.104.0/24"), false},
		{"changed length", model(true, 10779, "204.115.20.0/24"), model(true, 10779, "204.115.20.0/23"), false},
		{"changed origin", model(true, 10779, "23.161.104.0/24"), model(true, 18932, "23.161.104.0/24"), false},
		{"auto link turned off", model(true, 10779, "23.161.104.0/24"), model(false, 10779, "23.161.104.0/24"), false},
		{"nothing linked before", model(false, 10779, "23.161.104.0/24"), model(true, 10779, "23.161.104.0/24", "204.115.20.0/23"), false},
	} {
		if got := keepsLinks(c.state, c.plan); got != c.want {
			t.Errorf("%s: keepsLinks = %v, want %v", c.name, got, c.want)
		}
	}
}
