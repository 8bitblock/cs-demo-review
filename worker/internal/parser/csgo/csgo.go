package csgo

import (
	"context"
	"csdemoreview/worker/internal/model"
	base "csdemoreview/worker/internal/parser"
	"fmt"
	dem "github.com/markus-wa/demoinfocs-golang/v3/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v3/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v3/pkg/demoinfocs/events"
	"github.com/markus-wa/demoinfocs-golang/v3/pkg/demoinfocs/msg"
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
	if p == nil || p.Entity == nil {
		return s, false
	}
	pos := p.Position()
	s = model.Sample{Tick: tick, Time: seconds, PlayerID: pid(p), X: pos.X, Y: pos.Y, Z: pos.Z, Yaw: float64(p.ViewDirectionX()), Pitch: float64(p.ViewDirectionY()), Health: p.Health(), Armor: p.Armor(), Team: base.Team(int(p.Team)), Alive: p.IsAlive(), Weapon: weapon(p.ActiveWeapon()), Scoped: p.IsScoped(), Flashed: p.IsBlinded(), Crouching: p.IsDucking()}
	if _, valid := p.Entity.PropertyValue("localdata.m_vecViewOffset[2]"); valid {
		eye := p.PositionEyes()
		s.Eye = &model.Vec3{X: eye.X, Y: eye.Y, Z: eye.Z}
	}
	// Source 1 exposes a decoded vector here, unlike the affected 32-bit
	// QAngle decoder in the pinned Source 2 adapter.
	if v, present := p.Entity.PropertyValue("localdata.m_Local.m_aimPunchAngle"); present {
		punch := &model.Vec3{X: v.VectorVal.X, Y: v.VectorVal.Y, Z: v.VectorVal.Z}
		if base.FiniteVec(punch) {
			s.AimPunch = punch
		}
	}
	if w := p.ActiveWeapon(); w != nil && w.Entity != nil {
		if v, present := w.Entity.PropertyValue("m_flRecoilIndex"); present {
			s.RecoilIndex = base.Ptr(float64(v.FloatVal))
		}
	}
	return s, true
}

// Recorded spotting is a radar/network indicator, not an exact sight trace.
// Preserve an explicit known flag so an absent mask is not read as unspotted.
func spottedBy(target *common.Player, players []*common.Player) (ids []string, known bool) {
	if target == nil || target.Entity == nil {
		return nil, false
	}
	low, lo := target.Entity.PropertyValue("m_bSpottedByMask.000")
	high, hi := target.Entity.PropertyValue("m_bSpottedByMask.001")
	if !lo || !hi {
		return nil, false
	}
	mask := uint64(uint32(low.IntVal)) | uint64(uint32(high.IntVal))<<32
	for _, observer := range players {
		if observer != nil && observer.EntityID >= 1 && observer.EntityID <= 64 && mask&(uint64(1)<<uint(observer.EntityID-1)) != 0 {
			ids = append(ids, pid(observer))
		}
	}
	sort.Strings(ids)
	return ids, true
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
	h, err := p.ParseHeader()
	if err != nil {
		return err
	}
	if err = base.ValidateGame(h.GameDirectory); err != nil {
		return err
	}
	c.Demo.Map = h.MapName
	c.Demo.MapVersion = ""
	c.Demo.RecordingType = h.ClientName
	c.Demo.TickRate = p.TickRate()
	p.RegisterNetMessageHandler(func(info *msg.CSVCMsg_ServerInfo) {
		if crc := base.MapCRCVersion(info.GetMapCrc()); crc != "" {
			c.Demo.MapVersion = crc
		}
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
			s.AimPunch = q.AimPunch
			if q.AimPunch != nil {
				s.AimPunchScale = base.Ptr(2)
				s.Provenance += "; sampled-recoil CS:GO localdata.m_Local.m_aimPunchAngle at weapon_fire; nominal weapon_recoil_scale=2 assumption"
			}
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
	p.RegisterEventHandler(func(events.FrameDone) {
		clock()
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
