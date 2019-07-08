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
var shadowPlayer *model.Player
var gamemap = model.NewMap()
var controls = model.Controls{}

func add(some js.Value, i []js.Value) interface{} {
	js.Global().Set("output", js.ValueOf(i[0].Int()+i[1].Int()))
	return (js.ValueOf(i[0].Int() + i[1].Int()).String())
}

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

	localPlayer.ApplyMovement(controls, players, gamemap, float32(args[0].Float()))

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
	binary.LittleEndian.PutUint32(lookX[:], math.Float32bits(p.Collider.Look.X))
	binary.LittleEndian.PutUint32(lookY[:], math.Float32bits(p.Collider.Look.Y))
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

func getPlayerState(this js.Value, args []js.Value) interface{} {
	if localPlayer == nil {
		return js.ValueOf(nil)
	}

	id := uint8(args[0].Int())
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
	binary.LittleEndian.PutUint32(lookX[:], math.Float32bits(player.Collider.Look.X))
	binary.LittleEndian.PutUint32(lookY[:], math.Float32bits(player.Collider.Look.Y))
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

func applyShadowInput(this js.Value, args []js.Value) interface{} {
	if shadowPlayer == nil {
		return js.ValueOf(nil)
	}

	shadowControls := model.Controls{}
	shadowControls.Forward = !(args[0].Int() == 0)
	shadowControls.Backward = !(args[1].Int() == 0)
	shadowControls.Left = !(args[2].Int() == 0)
	shadowControls.Right = !(args[3].Int() == 0)
	shadowControls.TurretLeft = !(args[4].Int() == 0)
	shadowControls.TurretRight = !(args[5].Int() == 0)
	shadowControls.Shoot = !(args[6].Int() == 0)

	shadowPlayer.ApplyMovement(shadowControls, players, gamemap, float32(args[7].Float()))

	var uint8Array = js.Global().Get("Uint8Array")
	p := shadowPlayer
	buf := make([]byte, 0, 24)
	posX := make([]byte, 4)
	posY := make([]byte, 4)
	lookX := make([]byte, 4)
	lookY := make([]byte, 4)
	rotation := make([]byte, 4)
	turretRotation := make([]byte, 4)

	binary.LittleEndian.PutUint32(posX[:], math.Float32bits(p.Collider.Pivot.X))
	binary.LittleEndian.PutUint32(posY[:], math.Float32bits(p.Collider.Pivot.Y))
	binary.LittleEndian.PutUint32(lookX[:], math.Float32bits(p.Collider.Look.X))
	binary.LittleEndian.PutUint32(lookY[:], math.Float32bits(p.Collider.Look.Y))
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

func getLocalPlayerPosition(this js.Value, args []js.Value) interface{} {
	return js.ValueOf(fmt.Sprint("%f %f", localPlayer.Collider.Pivot.X, localPlayer.Collider.Pivot.Y)).String()
}

func checkCollision(this js.Value, args []js.Value) interface{} {
	println(args[0].Float())
	return js.ValueOf(args[0].Float())
}

func createLocalPlayer(this js.Value, args []js.Value) interface{} {
	id := uint8(args[0].Int())
	x := float32(args[1].Float())
	y := float32(args[2].Float())
	width := float32(args[3].Float())
	depth := float32(args[4].Float())

	fmt.Printf("%f %f %f %f \n", x, y, width, depth)

	player := &model.Player{
		ID:       id,
		Collider: model.NewRectCollider(x, y, width, depth),
	}

	localPlayer = player
	players = append(players, player)
	return js.ValueOf(player.Collider.Pivot.Y)
}

func createShadowPlayer(this js.Value, args []js.Value) interface{} {
	id := uint8(args[0].Int())
	x := float32(args[1].Float())
	y := float32(args[2].Float())
	width := float32(args[3].Float())
	depth := float32(args[4].Float())

	player := &model.Player{
		ID:       id,
		Collider: model.NewRectCollider(x, y, width, depth),
	}

	shadowPlayer = player
	return js.ValueOf(player.Collider.Pivot.Y)
}

func updateShadowPlayer(this js.Value, args []js.Value) interface{} {
	x := float32(args[1].Float())
	y := float32(args[2].Float())
	rotation := float32(args[3].Float())
	turretRotation := float32(args[4].Float())

	shadowPlayer.Collider.ChangePosition(x, y)
	shadowPlayer.Collider.Rotation = rotation
	shadowPlayer.Collider.TurretRotation = turretRotation

	return js.ValueOf(nil)
}

func createNetworkPlayer(this js.Value, args []js.Value) interface{} {
	id := uint8(args[0].Int())
	x := float32(args[1].Float())
	y := float32(args[2].Float())
	width := float32(args[3].Float())
	depth := float32(args[4].Float())

	player := &model.Player{
		ID:       id,
		Collider: model.NewRectCollider(x, y, width, depth),
	}

	players = append(players, player)
	return js.ValueOf(nil)
}

func updateNetworkPlayer(this js.Value, args []js.Value) interface{} {
	id := uint8(args[0].Int())
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

func getMap(some js.Value, i []js.Value) interface{} {
	var uint8Array = js.Global().Get("Uint8Array")

	src := make([]byte, 0)
	x := make([]byte, 4)
	y := make([]byte, 4)

	for _, point := range gamemap.Collider.Points {
		binary.LittleEndian.PutUint32(x[:], math.Float32bits(point.X))
		binary.LittleEndian.PutUint32(y[:], math.Float32bits(point.Y))
		src = append(src, x...)
		src = append(src, y...)
	}

	buf := uint8Array.New(len(src))
	js.CopyBytesToJS(buf, src)
	return buf
}

// func encodePlayerInput(this js.Value, i []js.Value) interface{} {
// 	protocol.EncodePlayerInput()
// 	return js.TypedArrayOf([]int8{1, 3})
// }

func registerCallbacks() {
	js.Global().Set("add", js.FuncOf(add))
	js.Global().Set("getMap", js.FuncOf(getMap))
	js.Global().Set("setInput", js.FuncOf(setInput))
	js.Global().Set("applyInput", js.FuncOf(applyInput))
	js.Global().Set("applyShadowInput", js.FuncOf(applyShadowInput))
	js.Global().Set("checkCollision", js.FuncOf(checkCollision))
	js.Global().Set("createLocalPlayer", js.FuncOf(createLocalPlayer))
	js.Global().Set("getLocalPlayerPosition", js.FuncOf(getLocalPlayerPosition))
	js.Global().Set("createNetworkPlayer", js.FuncOf(createNetworkPlayer))
	js.Global().Set("updateNetworkPlayer", js.FuncOf(updateNetworkPlayer))
	js.Global().Set("getPlayerState", js.FuncOf(getPlayerState))
	js.Global().Set("createShadowPlayer", js.FuncOf(createShadowPlayer))
	js.Global().Set("updateShadowPlayer", js.FuncOf(updateShadowPlayer))
	// js.Global().Set("encodePlayerInput", js.FuncOf(encodePlayerInput))
}

var proto protocol.BinaryProtocol

func main() {

	c := make(chan struct{}, 0)

	println("WASM Go Initialized")
	// register functions
	registerCallbacks()
	<-c
}
