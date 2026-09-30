package cs2

import (
	"context"
	"csdemoreview/worker/internal/model"
	base "csdemoreview/worker/internal/parser"
	"fmt"
	dem "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/msg"
	"os"
	"sort"
	"strings"
)

func pid(p *common.Player) string {
	if p == nil {
		return ""
	}
	return base.PlayerID(p.SteamID64, p.UserID)
}
func weapon(w *common.Equipment) string {
	if w == nil {
		return "unknown"
	}
	return w.String()
}
func sample(p *common.Player, tick int, seconds float64) (s model.Sample, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	if p == nil || p.PlayerPawnEntity() == nil {
		return s, false
	}
	pos := p.Position()
	s = model.Sample{Tick: tick, Time: seconds, PlayerID: pid(p), X: pos.X, Y: pos.Y, Z: pos.Z, Yaw: float64(p.ViewDirectionX()), Pitch: float64(p.ViewDirectionY()), Health: p.Health(), Armor: p.Armor(), Team: base.Team(int(p.Team)), Alive: p.IsAlive(), Weapon: weapon(p.ActiveWeapon()), Scoped: p.IsScoped(), Flashed: p.IsBlinded(), Crouching: p.IsDucking()}
	if eye, valid := p.PositionEyes(); valid {
		s.Eye = &model.Vec3{X: eye.X, Y: eye.Y, Z: eye.Z}
	}
	if w := p.ActiveWeapon(); w != nil && w.Entity != nil {
		if v, present := w.Entity.PropertyValue("m_flRecoilIndex"); present {
			s.RecoilIndex = base.Ptr(float64(v.Float()))
		}
	}
	return s, true
}

// The mask reports recorded radar spotting, not exact client visibility.
func spottedBy(target *common.Player, players []*common.Player) (ids []string, known bool) {
	if target == nil || target.PlayerPawnEntity() == nil {
		return nil, false
	}
	entity := target.PlayerPawnEntity()
	low, lo := entity.PropertyValue("m_bSpottedByMask.0000")
	high, hi := entity.PropertyValue("m_bSpottedByMask.0001")
	if !lo || !hi {
		return nil, false
	}
	lowBits, lo := low.Any.(uint64)
	highBits, hi := high.Any.(uint64)
	if !lo || !hi {
		return nil, false
	}
	mask := uint64(uint32(lowBits)) | uint64(uint32(highBits))<<32
	for _, observer := range players {
		if observer != nil && observer.EntityID >= 1 && observer.EntityID <= 64 && mask&(uint64(1)<<uint(observer.EntityID-1)) != 0 {
			ids = append(ids, pid(observer))
		}
	}
	sort.Strings(ids)
	return ids, true
}

// CMsgTEFireBullets.player refers to the pawn, not the controller. Use the
// adapter's pawn lookup; controller lookups silently discarded every packet
// in real CS2 fixtures even though native aim-punch and directions were present.
func nativePlayer(m *msg.CMsgTEFireBullets, lookupPawn func(uint64) *common.Player) *common.Player {
	if m == nil || m.Player == nil || m.GetPlayer() == msg.Default_CMsgTEFireBullets_Player {
		return nil
	}
	return lookupPawn(uint64(m.GetPlayer()))
}

func attachNative(c *base.Collector, playerID string, tick int, m *msg.CMsgTEFireBullets) {
	if playerID == "" || m == nil {
		return
	}
	var found *model.Shot
	for _, s := range c.Pending {
		if s.PlayerID == playerID && s.Tick == tick {
			if found != nil {
				found.Ambiguous = true
				s.Ambiguous = true
				return
			}
			found = s
		}
	}
	if found == nil {
		return
	}
	if strings.Contains(found.Provenance, "shot-native") {
		found.Ambiguous = true
		return
	}
	if m.Origin != nil {
		origin := &model.Vec3{X: float64(m.Origin.GetX()), Y: float64(m.Origin.GetY()), Z: float64(m.Origin.GetZ())}
		if base.FiniteVec(origin) {
			found.Origin = origin
		}
	}
	if m.Angles != nil {
		angles := &model.Vec3{X: float64(m.Angles.GetX()), Y: float64(m.Angles.GetY()), Z: float64(m.Angles.GetZ())}
		if base.FiniteVec(angles) {
			found.NativeAngles = angles
		}
	}
	for _, field := range []struct {
		native *float32
		stored **float64
	}{{m.RecoilIndex, &found.RecoilIndex}, {m.Spread, &found.Spread}, {m.Inaccuracy, &found.Inaccuracy}} {
		if field.native != nil && base.Finite(float64(*field.native)) && *field.native >= 0 {
			*field.stored = base.Ptr(float64(*field.native))
		}
	}
	if m.Extra != nil && m.Extra.AimPunch != nil {
		a := m.Extra.AimPunch
		punch := &model.Vec3{X: float64(a.GetX()), Y: float64(a.GetY()), Z: float64(a.GetZ())}
		if base.FiniteVec(punch) {
			found.AimPunch = punch
			// The fire packet contains the effective shot-angle offset. Across
			// real build 10924 recordings, native angles - view match this value
			// at scale 1; applying the legacy entity scale 2 doubles recoil.
			found.AimPunchScale = base.Ptr(1)
		}
	}
	found.Provenance += "; shot-native CS2 CMsgTEFireBullets; pawn-handle and unique same-demo-tick association; effective Extra.aim_punch scale=1 (verified against recorded fire/view angles on build 10924); native angles are fire angles, not reconstructed pellet trajectories"
}

func Parse(ctx context.Context, path string, c *base.Collector) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	info, _ := f.Stat()
	p := dem.NewParser(f)
	defer p.Close()
	clock := func() { c.Tick = p.GameState().IngameTick(); c.Time = p.CurrentTime().Seconds() }
	player := func(q *common.Player) *model.Player {
		if q == nil {
			return nil
		}
		return c.Player(pid(q), q.Name, base.Team(int(q.Team)))
	}
	c.Warn("Continuous CS2 entity aim-punch is unavailable in the pinned decoder. Recorded native fire-packet aim-punch is retained when present; radar spotting is a visibility proxy only.")
	p.RegisterNetMessageHandler(func(h *msg.CDemoFileHeader) {
		c.Error(base.ValidateGame(h.GetGameDirectory()))
		c.Demo.Map = h.GetMapName()
		if h.BuildNum != nil {
			c.Demo.MapVersion = fmt.Sprintf("build:%d", h.GetBuildNum())
		}
		if strings.Contains(strings.ToLower(h.GetClientName()), "tv") {
			c.Demo.RecordingType = "GOTV / CSTV server recording"
		}
	})
	p.RegisterEventHandler(func(e events.POVRecordingPlayerDetected) {
		c.Demo.RecordingType = "POV recording"
		c.Warn("POV recordings may omit opponents or events outside the recorded client's visibility.")
	})
	p.RegisterEventHandler(func(e events.ParserWarn) { c.Warn(e.Message) })
	p.RegisterEventHandler(func(e events.RoundStart) { clock(); c.StartRound() })
	p.RegisterEventHandler(func(e events.RoundEnd) { clock(); c.EndRound(base.Team(int(e.Winner)), e.Message) })
	p.RegisterEventHandler(func(e events.Kill) {
		clock()
		if q := player(e.Victim); q != nil {
			q.Deaths++
		}
		if q := player(e.Killer); q != nil && e.Killer != e.Victim {
			q.Kills++
			if e.IsHeadshot {
				q.Headshots++
			}
		}
		if q := player(e.Assister); q != nil {
			q.Assists++
		}
		c.Event(model.GameEvent{Kind: "kill", PlayerID: pid(e.Killer), TargetID: pid(e.Victim), Weapon: weapon(e.Weapon), Headshot: e.IsHeadshot, Text: "Elimination"})
	})
	p.RegisterEventHandler(func(e events.PlayerHurt) {
		clock()
		if q := player(e.Attacker); q != nil {
			q.Damage += e.HealthDamageTaken
		}
		c.Event(model.GameEvent{Kind: "damage", PlayerID: pid(e.Attacker), TargetID: pid(e.Player), Weapon: weapon(e.Weapon), Text: fmt.Sprintf("%d damage", e.HealthDamageTaken)})
	})
	p.RegisterEventHandler(func(e events.WeaponFire) {
		clock()
		if e.Shooter == nil {
			return
		}
		player(e.Shooter)
		s := model.Shot{PlayerID: pid(e.Shooter), Weapon: weapon(e.Weapon)}
		if q, ok := sample(e.Shooter, c.Tick, c.Time); ok {
			s.Origin = q.Eye
			s.ViewAngles = &model.Vec3{X: q.Pitch, Y: q.Yaw}
			s.RecoilIndex = q.RecoilIndex
		}
		base.MatchEligibility(&s, p.GameState().IsWarmupPeriod(), p.GameState().IsMatchStarted())
		c.Shot(s)
	})
	p.RegisterEventHandler(func(e events.GenericGameEvent) {
		clock()
		if e.Name == "bullet_impact" {
			u, x, y, z := e.Data["userid"], e.Data["x"], e.Data["y"], e.Data["z"]
			if u != nil && x != nil && y != nil && z != nil {
				q := p.GameState().Participants().ByUserID()[int(u.GetValShort())]
				if q == nil {
					q = p.GameState().Participants().ByUserID()[int(u.GetValLong())]
				}
				c.Impact(pid(q), model.Vec3{X: float64(x.GetValFloat()), Y: float64(y.GetValFloat()), Z: float64(z.GetValFloat())})
			}
		}
	})
	p.RegisterEventHandler(func(e events.GrenadeEventIf) {
		clock()
		b := e.Base()
		kind := "grenade"
		switch e.(type) {
		case events.SmokeStart:
			kind = "smoke-start"
		case events.SmokeExpired:
			kind = "smoke-end"
		case events.FlashExplode:
			kind = "flash"
		}
		c.Event(model.GameEvent{Kind: kind, PlayerID: pid(b.Thrower), Weapon: b.GrenadeType.String(), X: base.Ptr(b.Position.X), Y: base.Ptr(b.Position.Y), Z: base.Ptr(b.Position.Z), Text: fmt.Sprintf("%T", e)})
	})
	bomb := func(kind string, q *common.Player) {
		clock()
		ev := model.GameEvent{Kind: kind, PlayerID: pid(q), Text: strings.ReplaceAll(kind, "-", " ")}
		if q != nil {
			v := q.Position()
			ev.X = base.Ptr(v.X)
			ev.Y = base.Ptr(v.Y)
			ev.Z = base.Ptr(v.Z)
		}
		c.Event(ev)
	}
	p.RegisterEventHandler(func(e events.BombPlanted) { bomb("bomb-planted", e.Player) })
	p.RegisterEventHandler(func(e events.BombDefused) { bomb("bomb-defused", e.Player) })
	p.RegisterEventHandler(func(e events.BombExplode) { bomb("bomb-exploded", e.Player) })
	// FireBullets uses a player-pawn entity handle. Only attach a unique
	// same-tick packet; uncertain associations never contribute to detectors.
	type nativePacket struct {
		tick     int
		playerID string
		message  *msg.CMsgTEFireBullets
	}
	nativePackets := []nativePacket{}
	p.RegisterNetMessageHandler(func(m *msg.CMsgTEFireBullets) {
		clock()
		q := nativePlayer(m, p.GameState().Participants().FindByPawnHandle)
		if q == nil {
			return
		}
		nativePackets = append(nativePackets, nativePacket{tick: c.Tick, playerID: pid(q), message: m})
	})
	p.RegisterEventHandler(func(events.FrameDone) {
		clock()
		// Network messages and weapon_fire may arrive in either order within
		// a frame. Correlate only after both have been processed.
		for _, packet := range nativePackets {
			attachNative(c, packet.playerID, packet.tick, packet.message)
		}
		nativePackets = nativePackets[:0]
		offset, _ := f.Seek(0, 1)
		progress := float64(offset) / float64(info.Size())
		c.Frame(c.Tick, c.Time, p.TickRate(), progress)
		players := p.GameState().Participants().Playing()
		for _, q := range players {
			if q == nil {
				continue
			}
			player(q)
			if s, ok := sample(q, c.Tick, c.Time); ok {
				s.SpottedBy, s.SpottedKnown = spottedBy(q, players)
				c.Sample(s)
			} else {
				c.Warn("Some player snapshots lacked required entity fields and were omitted.")
			}
		}
	})
	for {
		if e := ctx.Err(); e != nil {
			return e
		}
		more, e := p.ParseNextFrame()
		if e != nil {
			return e
		}
		if c.Err != nil {
			return c.Err
		}
		if !more {
			break
		}
	}
	return nil
}
