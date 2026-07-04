package simverse

import (
	"database/sql"
	"fmt"
	"math/rand"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

const benchNPCCount = 10000

func makeBenchNPCs(n int) []NPCV2 {
	npcs := make([]NPCV2, n)
	rng := rand.New(rand.NewSource(42))
	speciesNames := []string{"Human", "Elf", "Dwarf", "Orc", "Beastman", "Dragonkin", "Fey", "Undead"}
	profNames := []string{"Farmer", "Miner", "Woodcutter", "Blacksmith", "Merchant", "Warrior", "Mage", "Priest"}

	for i := 0; i < n; i++ {
		sp := i % int(SpeciesMax)
		pr := i % int(ProfMax)
		npcs[i] = NPCV2{
			ID:         uint64(i),
			Name:       fmt.Sprintf("%s_%s_%08d", speciesNames[sp], profNames[pr%len(profNames)], i),
			Species:    SpeciesType(sp),
			Profession: ProfessionType(pr),
			Level:      uint16(1 + i%50),
			Age:        uint16(18 + i%60),
			Health:     uint16(800 + rng.Intn(400)),
			MaxHealth:  uint16(1000 + rng.Intn(500)),
			Energy:     uint16(600 + rng.Intn(400)),
			MaxEnergy:  uint16(800 + rng.Intn(400)),
			Mana:       uint16(rng.Intn(500)),
			MaxMana:    uint16(rng.Intn(800)),
			Mood:       int8(rng.Intn(100) - 50),
			Satisfaction: int8(rng.Intn(100) - 50),
			OrgID:      uint32(i % 1000),
			RegionID:   uint32(i % 100),
			Experience: uint32(rng.Intn(100000)),
			WealthTier: uint8(i % 5),
			SocialTier: uint8(i % 4),
		}
		for s := 0; s < int(SkillMax); s++ {
			npcs[i].Skills[s] = uint8(rng.Intn(100))
		}
		for r := 0; r < int(ResMax); r++ {
			npcs[i].Inventory[r] = uint32(rng.Intn(1000))
			npcs[i].Bank[r] = uint32(rng.Intn(100000))
		}
	}
	return npcs
}

func BenchmarkSQLite_NPCCreate(b *testing.B) {
	db := newSQLiteBenchDB(b)
	defer db.Close()
	npcs := makeBenchNPCs(benchNPCCount)

	buf := make([]byte, 512)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		npc := &npcs[i%benchNPCCount]
		size := npc.MarshalTo(buf)
		stmt, _ := db.Prepare("INSERT OR REPLACE INTO sim_npcs (id, name, data) VALUES (?, ?, ?)")
		stmt.Exec(npc.ID, npc.Name, buf[:size])
		stmt.Close()
	}
}

func BenchmarkSQLite_NPCCreateBatch(b *testing.B) {
	batchSizes := []int{10, 100, 1000}
	for _, batchSize := range batchSizes {
		b.Run(fmt.Sprintf("batch_%d", batchSize), func(b *testing.B) {
			db := newSQLiteBenchDB(b)
			defer db.Close()
			npcs := makeBenchNPCs(batchSize)
			buf := make([]byte, 512)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tx, _ := db.Begin()
				stmt, _ := tx.Prepare("INSERT OR REPLACE INTO sim_npcs (id, name, data) VALUES (?, ?, ?)")
				for j := 0; j < batchSize; j++ {
					npc := &npcs[j]
					size := npc.MarshalTo(buf)
					stmt.Exec(npc.ID, npc.Name, buf[:size])
				}
				stmt.Close()
				tx.Commit()
			}
		})
	}
}

func BenchmarkSQLite_NPCGetByID(b *testing.B) {
	db := newSQLiteBenchDB(b)
	defer db.Close()
	npcs := makeBenchNPCs(benchNPCCount)
	buf := make([]byte, 512)

	tx, _ := db.Begin()
	stmt, _ := tx.Prepare("INSERT OR REPLACE INTO sim_npcs (id, name, data) VALUES (?, ?, ?)")
	for i := 0; i < benchNPCCount; i++ {
		size := npcs[i].MarshalTo(buf)
		stmt.Exec(npcs[i].ID, npcs[i].Name, buf[:size])
	}
	stmt.Close()
	tx.Commit()

	b.ResetTimer()
	result := NPCV2{}
	dataBuf := make([]byte, 512)
	for i := 0; i < b.N; i++ {
		id := uint64(i % benchNPCCount)
		row := db.QueryRow("SELECT data FROM sim_npcs WHERE id = ?", id)
		var data []byte
		row.Scan(&data)
		copy(dataBuf, data)
		result.Unmarshal(dataBuf[:len(data)])
	}
}

func BenchmarkSQLite_NPCListByRegion(b *testing.B) {
	db := newSQLiteBenchDB(b)
	defer db.Close()
	npcs := makeBenchNPCs(benchNPCCount)
	buf := make([]byte, 512)

	tx, _ := db.Begin()
	stmt, _ := tx.Prepare("INSERT OR REPLACE INTO sim_npcs (id, name, region_id, data) VALUES (?, ?, ?, ?)")
	for i := 0; i < benchNPCCount; i++ {
		size := npcs[i].MarshalTo(buf)
		stmt.Exec(npcs[i].ID, npcs[i].Name, npcs[i].RegionID, buf[:size])
	}
	stmt.Close()
	tx.Commit()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		regionID := uint32(i % 100)
		rows, _ := db.Query("SELECT id, data FROM sim_npcs WHERE region_id = ? LIMIT 100", regionID)
		count := 0
		for rows.Next() {
			var id uint64
			var data []byte
			rows.Scan(&id, &data)
			count++
		}
		rows.Close()
	}
}

func BenchmarkSQLite_NPCUpdate(b *testing.B) {
	db := newSQLiteBenchDB(b)
	defer db.Close()
	npcs := makeBenchNPCs(benchNPCCount)
	buf := make([]byte, 512)

	tx, _ := db.Begin()
	stmt, _ := tx.Prepare("INSERT OR REPLACE INTO sim_npcs (id, name, data) VALUES (?, ?, ?)")
	for i := 0; i < benchNPCCount; i++ {
		size := npcs[i].MarshalTo(buf)
		stmt.Exec(npcs[i].ID, npcs[i].Name, buf[:size])
	}
	stmt.Close()
	tx.Commit()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := i % benchNPCCount
		npc := &npcs[idx]
		npc.Energy = uint16(int(npc.Energy) - 1)
		if npc.Energy < 10 {
			npc.Energy = npc.MaxEnergy
		}
		size := npc.MarshalTo(buf)
		db.Exec("UPDATE sim_npcs SET data = ? WHERE id = ?", buf[:size], npc.ID)
	}
}

func newSQLiteBenchDB(b *testing.B) *sql.DB {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "sim-bench.db")
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(30000)&_pragma=synchronous(NORMAL)",
		dbPath,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		b.Fatal(err)
	}
	db.SetMaxOpenConns(1)

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS sim_npcs (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			species INTEGER DEFAULT 0,
			profession INTEGER DEFAULT 0,
			org_id INTEGER DEFAULT 0,
			region_id INTEGER DEFAULT 0,
			level INTEGER DEFAULT 1,
			data BLOB
		);
		CREATE INDEX IF NOT EXISTS idx_sim_npcs_region ON sim_npcs(region_id);
		CREATE INDEX IF NOT EXISTS idx_sim_npcs_org ON sim_npcs(org_id);
		CREATE INDEX IF NOT EXISTS idx_sim_npcs_prof ON sim_npcs(profession);
	`)
	if err != nil {
		b.Fatal(err)
	}
	return db
}
