package wg

import "testing"

func TestParseDefaultRouteIface(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"default via 10.88.0.1 dev eth0 proto dhcp metric 100\n", "eth0", true},
		{"default via 192.168.1.1 dev eno1 \n", "eno1", true},
		{"default dev eth0 scope link\n", "eth0", true},
		{"10.8.0.0/24 dev wg0 proto kernel\ndefault via 1.1.1.1 dev eth0\n", "eth0", true},
		{"", "", false},
		{"10.0.0.0/8 via 10.0.0.1 dev eth0\n", "", false},
	}
	for _, c := range cases {
		got, err := parseDefaultRouteIface(c.in)
		if c.ok {
			if err != nil {
				t.Errorf("parse(%q): %v", c.in, err)
				continue
			}
			if got != c.want {
				t.Errorf("parse(%q)=%q want %q", c.in, got, c.want)
			}
		} else if err == nil {
			t.Errorf("parse(%q)=%q want error", c.in, got)
		}
	}
}
