package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"syscall/js"

	"github.com/awdng/triebwerk/model"
	"github.com/awdng/triebwerk/protocol"
)

var players = make([]*model.Player, 0)
var localPlayer *model.Player
var gameState = model.NewGameState("local")
var controls = model.Controls{}

func setInput(this js.Value, args []js.Value) interface{} {
	controls.Forward = !(args[0].Int() == 0)
	controls.Backward = !(args[1].Int() == 0)
	controls.Left = !(args[2].Int() == 0)
	controls.Right = !(args[3].Int() == 0)
	controls.TurretLeft = !(args[4].Int() == 0)
	controls.TurretRight = !(args[5].Int() == 0)
	controls.Shoot = !(args[6].Int() == 0)
	return js.ValueOf(nil)
}

func applyInput(this js.Value, args []js.Value) interface{} {
	if localPlayer == nil {
		return js.ValueOf(nil)
	}
	localPlayer.Control = controls
	localPlayer.HandleMovement(players, gameState.Map, float32(args[0].Float()))

	var uint8Array = js.Global().Get("Uint8Array")
	p := localPlayer
	buf := make([]byte, 0, 24)
	posX := make([]byte, 4)
	posY := make([]byte, 4)
	lookX := make([]byte, 4)
	lookY := make([]byte, 4)
	rotation := make([]byte, 4)
	turretRotation := make([]byte, 4)

	binary.LittleEndian.PutUint32(posX[:], math.Float32bits(p.Collider.Pivot.X))
	binary.LittleEndian.PutUint32(posY[:], math.Float32bits(p.Collider.Pivot.Y))
	binary.LittleEndian.PutUint32(lookX[:], math.Float32bits(p.Collider.Turret.X))
	binary.LittleEndian.PutUint32(lookY[:], math.Float32bits(p.Collider.Turret.Y))
	binary.LittleEndian.PutUint32(rotation[:], math.Float32bits(p.Collider.Rotation))
	binary.LittleEndian.PutUint32(turretRotation[:], math.Float32bits(p.Collider.TurretRotation))

	buf = append(buf, posX...)
	buf = append(buf, posY...)
	buf = append(buf, lookX...)
	buf = append(buf, lookY...)
	buf = append(buf, rotation...)
	buf = append(buf, turretRotation...)

	dst := uint8Array.New(len(buf))
	js.CopyBytesToJS(dst, buf)
	return dst
}

func checkProjectileCollision(this js.Value, args []js.Value) interface{} {
	posX := float32(args[0].Float())
	posY := float32(args[1].Float())

	projectile := &model.Projectile{
		Position: &model.Point{
			X: posX,
			Y: posY,
		},
		Cleanup: false,
	}

	for _, p := range players {
		if projectile.IsCollidingWithPlayer(p) {
			return js.ValueOf(true)
		}
	}

	if projectile.IsCollidingWithEnvironment(gameState.Map) {
		return js.ValueOf(true)
	}

	return js.ValueOf(false)
}

func getPlayerState(this js.Value, args []js.Value) interface{} {
	if localPlayer == nil {
		return js.ValueOf(nil)
	}

	id := args[0].Int()
	var player *model.Player
	for _, p := range players {
		if p.ID != id {
			continue
		}
		player = p
	}

	var uint8Array = js.Global().Get("Uint8Array")
	buf := make([]byte, 0, 24)
	posX := make([]byte, 4)
	posY := make([]byte, 4)
	lookX := make([]byte, 4)
	lookY := make([]byte, 4)
	rotation := make([]byte, 4)
	turretRotation := make([]byte, 4)

	binary.LittleEndian.PutUint32(posX[:], math.Float32bits(player.Collider.Pivot.X))
	binary.LittleEndian.PutUint32(posY[:], math.Float32bits(player.Collider.Pivot.Y))
	binary.LittleEndian.PutUint32(lookX[:], math.Float32bits(player.Collider.Turret.X))
	binary.LittleEndian.PutUint32(lookY[:], math.Float32bits(player.Collider.Turret.Y))
	binary.LittleEndian.PutUint32(rotation[:], math.Float32bits(player.Collider.Rotation))
	binary.LittleEndian.PutUint32(turretRotation[:], math.Float32bits(player.Collider.TurretRotation))

	buf = append(buf, posX...)
	buf = append(buf, posY...)
	buf = append(buf, lookX...)
	buf = append(buf, lookY...)
	buf = append(buf, rotation...)
	buf = append(buf, turretRotation...)

	dst := uint8Array.New(len(buf))
	js.CopyBytesToJS(dst, buf)
	return dst
}

func getLocalPlayerPosition(this js.Value, args []js.Value) interface{} {
	return js.ValueOf(fmt.Sprint("%f %f", localPlayer.Collider.Pivot.X, localPlayer.Collider.Pivot.Y)).String()
}

func createLocalPlayer(this js.Value, args []js.Value) interface{} {
	id := args[0].Int()
	x := float32(args[1].Float())
	y := float32(args[2].Float())
	width := float32(args[3].Float())
	depth := float32(args[4].Float())

	fmt.Printf("%f %f %f %f \n", x, y, width, depth)

	player := model.NewPlayer(id, x, y, nil)

	localPlayer = player
	players = append(players, player)
	return js.ValueOf(player.Collider.Pivot.Y)
}

func createNetworkPlayer(this js.Value, args []js.Value) interface{} {
	id := args[0].Int()
	x := float32(args[1].Float())
	y := float32(args[2].Float())

	player := model.NewPlayer(id, x, y, nil)

	players = append(players, player)
	return js.ValueOf(nil)
}

func updateNetworkPlayer(this js.Value, args []js.Value) interface{} {
	id := args[0].Int()
	x := float32(args[1].Float())
	y := float32(args[2].Float())
	rotation := float32(args[3].Float())
	turretRotation := float32(args[4].Float())

	for _, player := range players {
		if player.ID != id {
			continue
		}
		player.Collider.ChangePosition(x, y)
		player.Collider.Rotation = rotation
		player.Collider.TurretRotation = turretRotation
	}

	return js.ValueOf(nil)
}

func removePlayer(this js.Value, args []js.Value) interface{} {
	id := args[0].Int()
	if localPlayer != nil && localPlayer.ID == id {
		localPlayer = nil
		return js.ValueOf(nil)
	}

	newPlayers := make([]*model.Player, 0)
	for _, p := range players {
		if p.ID != id {
			newPlayers = append(newPlayers, p)
		}
	}
	players = newPlayers
	return js.ValueOf(nil)
}

func registerCallbacks() {
	js.Global().Set("setInput", js.FuncOf(setInput))
	js.Global().Set("applyInput", js.FuncOf(applyInput))
	js.Global().Set("createLocalPlayer", js.FuncOf(createLocalPlayer))
	js.Global().Set("getLocalPlayerPosition", js.FuncOf(getLocalPlayerPosition))
	js.Global().Set("createNetworkPlayer", js.FuncOf(createNetworkPlayer))
	js.Global().Set("removePlayer", js.FuncOf(removePlayer))
	js.Global().Set("updateNetworkPlayer", js.FuncOf(updateNetworkPlayer))
	js.Global().Set("getPlayerState", js.FuncOf(getPlayerState))
	js.Global().Set("checkProjectileCollision", js.FuncOf(checkProjectileCollision))
}

var proto protocol.BinaryProtocol

func main() {

	c := make(chan struct{}, 0)

	println("TANKS WASM Go Initialized")
	// register functions
	registerCallbacks()
	<-c
}
