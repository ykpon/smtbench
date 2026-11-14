package topology

import "smtbench/internal/x86"

func withCPUName(t *Topology, err error) (*Topology, error) {
	if err != nil {
		return nil, err
	}
	t.Model, t.Vendor = x86.Brand(), x86.Vendor()
	return t, nil
}
