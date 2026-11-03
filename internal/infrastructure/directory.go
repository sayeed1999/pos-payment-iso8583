package infrastructure

// Directory is the mock store of terminals and merchants.
//
// TODO(poc): replace with the real terminal and merchant records. POS00001 and
// MERCHANT0000001 are seeded so the demo is accepted. Nothing else is, and
// nothing is loaded from a database.
type Directory struct {
	terminals map[string]party
	merchants map[string]party
}

type party struct {
	name   string
	active bool
}

func NewDirectory() *Directory {
	return &Directory{
		terminals: map[string]party{
			"POS00001": {name: "Counter 1", active: true},
		},
		merchants: map[string]party{
			"MERCHANT0000001": {name: "Demo Store", active: true},
		},
	}
}

func (d *Directory) Terminal(id string) (name string, active, found bool) {
	p, found := d.terminals[id]
	if !found {
		return "", false, false
	}
	return p.name, p.active, true
}

func (d *Directory) Merchant(id string) (name string, active, found bool) {
	p, found := d.merchants[id]
	if !found {
		return "", false, false
	}
	return p.name, p.active, true
}
