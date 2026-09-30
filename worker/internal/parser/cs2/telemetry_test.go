package cs2

import (
	"csdemoreview/worker/internal/model"
	base "csdemoreview/worker/internal/parser"
	"math"
	"strings"
	"testing"

	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/msg"
	"google.golang.org/protobuf/proto"
)

func TestNativePlayerUsesRecordedPawnHandle(t *testing.T) {
	pawnHandle := uint32(7438885) // A real CS2 fire packet, not controller entity 6.
	player := &common.Player{SteamID64: 123, EntityID: 6}
	lookups := 0
	lookupPawn := func(handle uint64) *common.Player {
		lookups++
		if handle != uint64(pawnHandle) {
			t.Fatalf("changed recorded pawn handle: %d", handle)
		}
		return player
	}
	if got := nativePlayer(&msg.CMsgTEFireBullets{Player: &pawnHandle}, lookupPawn); got != player {
		t.Fatal("fire packet failed to resolve its pawn")
	}
	if nativePlayer(&msg.CMsgTEFireBullets{}, lookupPawn) != nil || nativePlayer(&msg.CMsgTEFireBullets{Player: proto.Uint32(msg.Default_CMsgTEFireBullets_Player)}, lookupPawn) != nil || lookups != 1 {
		t.Fatal("missing/default handle must not resolve to a participant")
	}
}

func TestNativeAssociationRetainsRecoilAndFireDirection(t *testing.T) {
	s := &model.Shot{PlayerID: "one", Tick: 2358, ViewAngles: &model.Vec3{X: 1, Y: 2}}
	c := &base.Collector{Pending: []*model.Shot{s}}
	m := &msg.CMsgTEFireBullets{
		Tick:        proto.Int32(5405), // Packet server tick has a distinct clock origin.
		Origin:      &msg.CMsgVector{X: proto.Float32(10), Z: proto.Float32(64)},
		Angles:      &msg.CMsgQAngle{X: proto.Float32(-8.5), Y: proto.Float32(41)},
		RecoilIndex: proto.Float32(3), Spread: proto.Float32(.002), Inaccuracy: proto.Float32(.005),
		Extra: &msg.CMsgTEFireBullets_Extra{AimPunch: &msg.CMsgQAngle{X: proto.Float32(-1.5), Y: proto.Float32(.2)}},
	}
	attachNative(c, "one", 2358, m)
	if s.AimPunch == nil || s.NativeAngles == nil || s.Origin == nil || s.RecoilIndex == nil || s.Ambiguous {
		t.Fatalf("native telemetry dropped: %+v", s)
	}
	if s.AimPunch.X != -1.5 || s.NativeAngles.Y != 41 || *s.RecoilIndex != 3 || s.ViewAngles.Y != 2 || s.AimPunchScale == nil || *s.AimPunchScale != 1 {
		t.Fatal("native fields must preserve values without changing recorded view angles")
	}
	if !strings.Contains(s.Provenance, "shot-native") || !strings.Contains(s.Provenance, "pawn-handle") || strings.Contains(s.Provenance, "validated-shot-direction") {
		t.Fatal("packet association must not claim validated pellet-direction semantics")
	}
	attachNative(c, "one", 2358, m)
	if !s.Ambiguous {
		t.Fatal("duplicate packets must exclude ambiguous associations")
	}
}

func TestNativeAssociationRejectsAmbiguityAndNonFiniteFields(t *testing.T) {
	a, b := &model.Shot{PlayerID: "one", Tick: 10}, &model.Shot{PlayerID: "one", Tick: 10}
	c := &base.Collector{Pending: []*model.Shot{a, b}}
	m := &msg.CMsgTEFireBullets{Angles: &msg.CMsgQAngle{X: proto.Float32(5)}}
	attachNative(c, "one", 10, m)
	if !a.Ambiguous || !b.Ambiguous || a.NativeAngles != nil || b.NativeAngles != nil {
		t.Fatal("multiple same-tick shots cannot receive one native packet")
	}
	s := &model.Shot{PlayerID: "one", Tick: 10, Origin: &model.Vec3{Z: 64}}
	c.Pending = []*model.Shot{s}
	m.Origin = &msg.CMsgVector{X: proto.Float32(float32(math.NaN()))}
	m.Angles.X = proto.Float32(float32(math.Inf(1)))
	m.Spread = proto.Float32(-1)
	m.Extra = &msg.CMsgTEFireBullets_Extra{AimPunch: &msg.CMsgQAngle{Y: proto.Float32(float32(math.NaN()))}}
	attachNative(c, "one", 10, m)
	if s.NativeAngles != nil || s.AimPunch != nil || s.Spread != nil || s.Origin.Z != 64 {
		t.Fatal("invalid optional fields contaminated stored telemetry")
	}
}
