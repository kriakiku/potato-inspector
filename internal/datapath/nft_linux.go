//go:build linux

package datapath

import (
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
	"github.com/kriakiku/potato-network/internal/config"
	"golang.org/x/sys/unix"
)

const (
	tableName     = "potatonetwork"
	chainMark     = "mark"
	chainRedirect = "redirect"
	chainFilter   = "filter"
)

var mu sync.Mutex

// InstallBase creates the nftables table with MITM redirect + QUIC drop.
func InstallBase(mitmPort int) error {
	mu.Lock()
	defer mu.Unlock()
	return installBaseLocked(mitmPort, true)
}

// SetExempt installs mark rules for API + DNS upstream (shape/redirect exempt).
func SetExempt(apiPort int, dnsUpstream string) error {
	mu.Lock()
	defer mu.Unlock()
	c := &nftables.Conn{}
	table := &nftables.Table{Family: nftables.TableFamilyIPv4, Name: tableName}
	// Ensure table exists
	c.AddTable(table)
	markChain := ensureMarkChain(c, table)
	flushChain(c, table, markChain)

	markVal := uint32(config.MarkExemptShape | config.MarkNoRedirect)
	if apiPort > 0 {
		addTCPPortMark(c, table, markChain, true, uint16(apiPort), markVal)  // sport
		addTCPPortMark(c, table, markChain, false, uint16(apiPort), markVal) // dport
	}
	if host, portStr, err := net.SplitHostPort(dnsUpstream); err == nil {
		if ip4 := net.ParseIP(host).To4(); ip4 != nil {
			p, _ := strconv.Atoi(portStr)
			addIPPortMark(c, table, markChain, ip4, unix.IPPROTO_UDP, uint16(p), markVal)
			addIPPortMark(c, table, markChain, ip4, unix.IPPROTO_TCP, uint16(p), markVal)
		}
	}
	if err := c.Flush(); err != nil {
		return fmt.Errorf("nftables set exempt: %w", err)
	}
	return nil
}

// ClearExempt removes only mark rules (keeps redirect/QUIC drop).
func ClearExempt() {
	mu.Lock()
	defer mu.Unlock()
	c := &nftables.Conn{}
	table := &nftables.Table{Family: nftables.TableFamilyIPv4, Name: tableName}
	markChain := &nftables.Chain{Name: chainMark, Table: table}
	flushChain(c, table, markChain)
	_ = c.Flush()
}

// Teardown deletes the entire potatonetwork table.
func Teardown() {
	mu.Lock()
	defer mu.Unlock()
	c := &nftables.Conn{}
	c.DelTable(&nftables.Table{Family: nftables.TableFamilyIPv4, Name: tableName})
	_ = c.Flush()
}

func installBaseLocked(mitmPort int, replace bool) error {
	c := &nftables.Conn{}
	if replace {
		c.DelTable(&nftables.Table{Family: nftables.TableFamilyIPv4, Name: tableName})
		_ = c.Flush()
		c = &nftables.Conn{}
	}
	table := &nftables.Table{Family: nftables.TableFamilyIPv4, Name: tableName}
	c.AddTable(table)

	_ = ensureMarkChain(c, table)
	redirChain := c.AddChain(&nftables.Chain{
		Name:     chainRedirect,
		Table:    table,
		Type:     nftables.ChainTypeNAT,
		Hooknum:  nftables.ChainHookOutput,
		Priority: nftables.ChainPriorityNATDest,
	})
	filtChain := c.AddChain(&nftables.Chain{
		Name:     chainFilter,
		Table:    table,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookOutput,
		Priority: nftables.ChainPriorityFilter,
	})

	addRedirect(c, table, redirChain, uint16(mitmPort))
	c.AddRule(&nftables.Rule{
		Table: table,
		Chain: filtChain,
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.IPPROTO_UDP}},
			&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseTransportHeader, Offset: 2, Len: 2},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: binaryutil.BigEndian.PutUint16(443)},
			&expr.Verdict{Kind: expr.VerdictDrop},
		},
	})
	if err := c.Flush(); err != nil {
		return fmt.Errorf("nftables install base: %w", err)
	}
	return nil
}

func ensureMarkChain(c *nftables.Conn, table *nftables.Table) *nftables.Chain {
	return c.AddChain(&nftables.Chain{
		Name:     chainMark,
		Table:    table,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookOutput,
		Priority: nftables.ChainPriorityMangle,
	})
}

func flushChain(c *nftables.Conn, table *nftables.Table, chain *nftables.Chain) {
	c.FlushChain(chain)
}

func addTCPPortMark(c *nftables.Conn, table *nftables.Table, chain *nftables.Chain, sport bool, port uint16, mark uint32) {
	off := uint32(2)
	if sport {
		off = 0
	}
	c.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: append(
			[]expr.Any{
				&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.IPPROTO_TCP}},
				&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseTransportHeader, Offset: off, Len: 2},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: binaryutil.BigEndian.PutUint16(port)},
			},
			markSetExprs(mark)...,
		),
	})
}

func addIPPortMark(c *nftables.Conn, table *nftables.Table, chain *nftables.Chain, ip4 net.IP, proto uint8, port uint16, mark uint32) {
	c.AddRule(&nftables.Rule{
		Table: table,
		Chain: chain,
		Exprs: append(
			[]expr.Any{
				&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseNetworkHeader, Offset: 16, Len: 4},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte(ip4.To4())},
				&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{proto}},
				&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseTransportHeader, Offset: 2, Len: 2},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: binaryutil.BigEndian.PutUint16(port)},
			},
			markSetExprs(mark)...,
		),
	})
}

func markSetExprs(mark uint32) []expr.Any {
	return []expr.Any{
		&expr.Immediate{Register: 1, Data: binaryutil.NativeEndian.PutUint32(mark)},
		&expr.Meta{Key: expr.MetaKeyMARK, Register: 1, SourceRegister: true},
	}
}

func addRedirect(c *nftables.Conn, table *nftables.Table, chain *nftables.Chain, mitmPort uint16) {
	noRedir := uint32(config.MarkNoRedirect)
	for _, dport := range []uint16{80, 443} {
		c.AddRule(&nftables.Rule{
			Table: table,
			Chain: chain,
			Exprs: []expr.Any{
				&expr.Meta{Key: expr.MetaKeyMARK, Register: 1},
				&expr.Bitwise{
					SourceRegister: 1,
					DestRegister:   1,
					Len:            4,
					Mask:           binaryutil.NativeEndian.PutUint32(noRedir),
					Xor:            binaryutil.NativeEndian.PutUint32(0),
				},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: binaryutil.NativeEndian.PutUint32(0)},
				&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{unix.IPPROTO_TCP}},
				&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseTransportHeader, Offset: 2, Len: 2},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: binaryutil.BigEndian.PutUint16(dport)},
				&expr.Immediate{Register: 1, Data: binaryutil.BigEndian.PutUint16(mitmPort)},
				&expr.Redir{RegisterProtoMin: 1},
			},
		})
	}
}
