package discovery

import (
	"path/filepath"
	"testing"

	"bb_erp_echo/internal/database"
)

func TestLoadOrCreateKeepsSingletonInstanceID(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "erp.sqlite3"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&Identity{}); err != nil {
		t.Fatalf("migrate identity: %v", err)
	}

	first, err := LoadOrCreate(db, IdentityMetadata{
		Product:           Product,
		DiscoveryProtocol: ProtocolVersion,
		ServerName:        "主服务",
		ServerVersion:     "1.0.0",
	})
	if err != nil {
		t.Fatalf("create identity: %v", err)
	}
	if first.ID != singletonIdentityID || first.InstanceID == "" {
		t.Fatalf("unexpected first identity: %+v", first)
	}

	second, err := LoadOrCreate(db, IdentityMetadata{
		Product:           Product,
		DiscoveryProtocol: ProtocolVersion,
		ServerName:        "主服务-更新",
		ServerVersion:     "1.0.1",
	})
	if err != nil {
		t.Fatalf("reload identity: %v", err)
	}
	if second.InstanceID != first.InstanceID {
		t.Fatalf("instance id changed from %q to %q", first.InstanceID, second.InstanceID)
	}
	if second.ServerName != "主服务-更新" || second.ServerVersion != "1.0.1" {
		t.Fatalf("metadata was not refreshed: %+v", second)
	}

	var count int64
	if err := db.Model(&Identity{}).Count(&count).Error; err != nil {
		t.Fatalf("count identities: %v", err)
	}
	if count != 1 {
		t.Fatalf("identity row count = %d, want 1", count)
	}
}

func TestLoadOrCreateRejectsInvalidMetadata(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "erp.sqlite3"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&Identity{}); err != nil {
		t.Fatalf("migrate identity: %v", err)
	}
	if _, err := LoadOrCreate(db, IdentityMetadata{Product: "wrong", DiscoveryProtocol: ProtocolVersion, ServerName: "server", ServerVersion: "1"}); err == nil {
		t.Fatal("invalid product should fail")
	}
}
