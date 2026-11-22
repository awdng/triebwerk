package model

import (
	"encoding/json"
	"math"
)

// The kinds of tanks, by the number they have on the wire. The first kind
// is the tank the game always had: clients and servers that know nothing of
// kinds drive it.
const (
	Stormblade uint8 = iota
	Charger
	Grunt
	PainMaker
	SuperKiller
	Liquidator
	Buggy
)

// TankSpec is what a kind of tank can do. The browser predicts its own
// tank with the same numbers: it reads this table from this code compiled
// to WebAssembly (cmd/wasm, tankSpecs).
type TankSpec struct {
	Name string `json:"name"`
	// Speed in units per second, forwards and backwards
	Speed float32 `json:"speed"`
	// TurnRate of the hull and TurretRate of the turret on it, in radians per second
	TurnRate   float32 `json:"turnRate"`
	TurretRate float32 `json:"turretRate"`
	// Reload is the time between two shots in seconds
	Reload float32 `json:"reload"`
	// Damage is the health a hit takes from a tank with plain armour
	Damage int `json:"damage"`
	// Armour is how much of a hit's damage gets through: 1 is all of it
	Armour float32 `json:"armour"`
	// Width and Depth of what collides and can be hit
	Width float32 `json:"width"`
	Depth float32 `json:"depth"`
}

// Tanks are the kinds there are, indexed by their number. A first balance,
// made by counting duels against the first kind: what a tank has more of
// in one column it pays for in another. The damage of a gun per second is
// close for all of them (between 17 and 21 against plain armour), so a
// duel is decided by armour, by who is hit, and by who gets away.
// Width and Depth follow the hulls of the models (client/tools/tanks).
var Tanks = []TankSpec{
	// the all-rounder: four hits destroy one of its kind
	Stormblade: {Name: "Stormblade", Speed: 15, TurnRate: 1.5, TurretRate: 1.5, Reload: 1.2, Damage: 25, Armour: 1, Width: 5, Depth: 7},
	// fast and nimble, with thinner armour and a gun that takes a little longer
	Charger: {Name: "Charger", Speed: 17.5, TurnRate: 1.7, TurretRate: 1.7, Reload: 1.3, Damage: 25, Armour: 1.1, Width: 5, Depth: 7},
	// sturdy and narrow, with a turret that follows quickly and a slower gun
	Grunt: {Name: "Grunt", Speed: 13.5, TurnRate: 1.4, TurretRate: 1.9, Reload: 1.4, Damage: 25, Armour: 0.85, Width: 4, Depth: 7},
	// one heavy gun: slow in everything, three hits destroy most tanks
	PainMaker: {Name: "Pain Maker", Speed: 11.5, TurnRate: 1.1, TurretRate: 1.1, Reload: 2.2, Damage: 40, Armour: 0.8, Width: 5, Depth: 7},
	// two guns that fire in turn: many light hits
	SuperKiller: {Name: "Super Killer", Speed: 13, TurnRate: 1.3, TurretRate: 1.4, Reload: 0.7, Damage: 15, Armour: 0.9, Width: 5.5, Depth: 7},
	// a gun platform: a big target that keeps firing
	Liquidator: {Name: "Liquidator", Speed: 12, TurnRate: 1.2, TurretRate: 1.3, Reload: 0.9, Damage: 18, Armour: 0.85, Width: 5.5, Depth: 5},
	// a scout: fast, small, three hits of a tank gun destroy it
	Buggy: {Name: "Buggy", Speed: 22, TurnRate: 2.4, TurretRate: 2.6, Reload: 0.4, Damage: 9, Armour: 1.4, Width: 4, Depth: 5},
}

// SpecOf returns the numbers of a kind. A kind that does not exist is the first.
func SpecOf(kind uint8) TankSpec {
	if int(kind) >= len(Tanks) {
		return Tanks[0]
	}
	return Tanks[kind]
}

// ReloadTicks is the number of ticks from one shot of a held trigger to the
// next: the weapon counts its reload time up in steps of a tick (see
// Weapon.Update), and this counts the same way.
func (s TankSpec) ReloadTicks() int {
	var countdown float32
	ticks := 1
	for countdown <= s.Reload {
		countdown += StepSeconds
		ticks++
	}
	return ticks
}

// DamageTo is the health one hit of this kind takes from a tank of another
func (s TankSpec) DamageTo(target TankSpec) int {
	damage := int(math.Round(float64(float32(s.Damage) * target.Armour)))
	if damage < 1 {
		return 1
	}
	return damage
}

// TankTableJSON is the table as clients read it: the fields of TankSpec and
// reloadTicks for every kind, in the order of their numbers
func TankTableJSON() string {
	type row struct {
		TankSpec
		ReloadTicks int `json:"reloadTicks"`
	}
	rows := make([]row, 0, len(Tanks))
	for _, spec := range Tanks {
		rows = append(rows, row{TankSpec: spec, ReloadTicks: spec.ReloadTicks()})
	}
	encoded, err := json.Marshal(rows)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// Loadout is what a player drives and how it is painted. The skin is a
// number the clients know the paint jobs of a kind by; the server passes it on.
type Loadout struct {
	Tank uint8
	Skin uint8
}
