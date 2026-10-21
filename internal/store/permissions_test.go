package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestPermissionTiersPersistAndInvalidateTargetRevision(t *testing.T) {
	ctx := context.Background()
	db, vault := openSSHConfigurationTestVault(t)
	target := saveSSHConfigurationTestTarget(t, db, vault, SSHTarget{IP: "192.0.2.90", Mode: SSHDirect, SSHPort: 22, LoginUsername: "ops", Enabled: true}, []byte("fixture"), "SHA256:fixture")
	if target.PermissionTier != PermissionConfirm {
		t.Fatalf("default=%s", target.PermissionTier)
	}
	for _, tier := range []PermissionTier{PermissionReadOnly, PermissionFull, PermissionConfirm} {
		oldRevision := target.Revision
		target.PermissionTier = tier
		var err error
		target, err = vault.CommitValidatedSSHTargetConfiguration(ctx, ValidatedSSHTargetConfiguration{Target: target, ConfirmedFingerprint: "SHA256:fixture"})
		if err != nil {
			t.Fatal(err)
		}
		listed, err := db.ListSSHTargets(ctx)
		if err != nil || len(listed) != 1 || listed[0].PermissionTier != tier || target.PermissionTier != tier || target.Revision <= oldRevision {
			t.Fatalf("target=%+v list=%+v err=%v", target, listed, err)
		}
		loaded, err := db.SSHTarget(ctx, target.IP)
		if err != nil || loaded.PermissionTier != tier {
			t.Fatalf("lookup=%+v err=%v", loaded, err)
		}
	}
	target.PermissionTier = "unrecognized"
	if err := db.UpsertSSHTarget(ctx, target); err == nil {
		t.Fatal("invalid tier accepted")
	}
}

func TestDatabasePermissionTiersPersistAndInvalidateRevision(t *testing.T) {
	ctx := context.Background()
	db, vault := openDatabaseConfigurationTestVault(t)
	instance := saveDatabaseConfigurationTestInstance(t, db, vault, DatabaseInstance{Host: "192.0.2.90", Port: 5432, Engine: EnginePostgreSQL, DefaultDatabase: "app", TransportSecurity: TransportPlaintext, ReadUsername: "reader", WriteUsername: "writer", Enabled: true}, []byte("fixture-read"), []byte("fixture-write"))
	if instance.PermissionTier != PermissionConfirm {
		t.Fatalf("default=%s", instance.PermissionTier)
	}
	for _, tier := range []PermissionTier{PermissionReadOnly, PermissionFull, PermissionConfirm} {
		old := instance.Revision
		instance.PermissionTier = tier
		var err error
		instance, err = vault.CommitValidatedDatabaseInstanceConfiguration(ctx, ValidatedDatabaseInstanceConfiguration{Instance: instance})
		if err != nil {
			t.Fatal(err)
		}
		listed, err := db.ListDatabaseInstances(ctx)
		if err != nil || len(listed) != 1 || listed[0].PermissionTier != tier || instance.Revision <= old {
			t.Fatalf("instance=%+v list=%+v err=%v", instance, listed, err)
		}
		loaded, err := db.DatabaseInstance(ctx, instance.Host, instance.Port)
		if err != nil || loaded.PermissionTier != tier {
			t.Fatalf("lookup=%+v err=%v", loaded, err)
		}
	}
	instance.PermissionTier = "unrecognized"
	if err := db.UpsertDatabaseInstance(ctx, instance); err == nil {
		t.Fatal("invalid tier accepted")
	}
}

func TestV18TargetsMigrateToConfirmRatherThanFull(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`ALTER TABLE ssh_targets DROP COLUMN permission_tier; ALTER TABLE database_instances DROP COLUMN permission_tier; DELETE FROM schema_migrations WHERE version=19;
	INSERT INTO ssh_targets(ip,connection_mode,login_username,enabled,created_at,updated_at) VALUES('192.0.2.10','direct','ops',1,0,0);
	INSERT INTO database_instances(host,port,engine,read_username,enabled,created_at,updated_at) VALUES('192.0.2.20',5432,'postgresql','reader',1,0,0);`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{"ssh_targets", "database_instances"} {
		var tier string
		if err := db.db.QueryRow("SELECT permission_tier FROM " + table).Scan(&tier); err != nil || tier != "confirm" {
			t.Fatalf("%s: %q %v", table, tier, err)
		}
	}
}
