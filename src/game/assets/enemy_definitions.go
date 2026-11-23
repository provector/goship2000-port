package assets

import "fmt"

// EnemyDefinition preserves the original WRLD.ENEM eight-byte row. These are
// immutable source bytes, not the installation's mutable ENEMY.DAT output.
type EnemyDefinition [8]byte

const EnemyDefinitionCount = 16

// Step50's complete twenty-member/five-bank index. The supported release has
// world-specific banks; accepting any known bank for any world would lose that
// source relationship. Hashes bind the original untransformed 128 bytes.
var originalEnemyDefinitionHashes = map[string]string{
	"ANTC1.WRL": "40735924f6d67ef21cadc901b253174c529f4eee12924360df1c4274d4f9a97c",
	"ANTC2.WRL": "40735924f6d67ef21cadc901b253174c529f4eee12924360df1c4274d4f9a97c",
	"ANTC3.WRL": "40735924f6d67ef21cadc901b253174c529f4eee12924360df1c4274d4f9a97c",
	"EURO6.WRL": "3f0f78ded9611ddfcf1b0cf0edd7299b00986d121b5ac9b720a15b54bb6a0c59",
	"EURO7.WRL": "3f0f78ded9611ddfcf1b0cf0edd7299b00986d121b5ac9b720a15b54bb6a0c59",
	"EURO8.WRL": "3f0f78ded9611ddfcf1b0cf0edd7299b00986d121b5ac9b720a15b54bb6a0c59",
	"EURO9.WRL": "3f0f78ded9611ddfcf1b0cf0edd7299b00986d121b5ac9b720a15b54bb6a0c59",
	"EUROX.WRL": "3f0f78ded9611ddfcf1b0cf0edd7299b00986d121b5ac9b720a15b54bb6a0c59",
	"GULF2.WRL": "d9a754852d92304352fe662d8c19f46e6211baa4bbf5354d551a9b96e2640708",
	"GULF3.WRL": "d9a754852d92304352fe662d8c19f46e6211baa4bbf5354d551a9b96e2640708",
	"GULF4.WRL": "d9a754852d92304352fe662d8c19f46e6211baa4bbf5354d551a9b96e2640708",
	"GULF5.WRL": "d9a754852d92304352fe662d8c19f46e6211baa4bbf5354d551a9b96e2640708",
	"GULFX.WRL": "d9a754852d92304352fe662d8c19f46e6211baa4bbf5354d551a9b96e2640708",
	"ISLE1.WRL": "c1267325d74ff87ffefc685ddd00551d0271e2fc66c1821917555ff4cb6df6c0",
	"ISLE2.WRL": "c1267325d74ff87ffefc685ddd00551d0271e2fc66c1821917555ff4cb6df6c0",
	"ISLE3.WRL": "c1267325d74ff87ffefc685ddd00551d0271e2fc66c1821917555ff4cb6df6c0",
	"ISLEX.WRL": "c1267325d74ff87ffefc685ddd00551d0271e2fc66c1821917555ff4cb6df6c0",
	"PHIL1.WRL": "5998586dbfa7978d5daf9030e7e06d5fb04a3045fd7defd02c06516e1c8fae3a",
	"PHIL2.WRL": "5998586dbfa7978d5daf9030e7e06d5fb04a3045fd7defd02c06516e1c8fae3a",
	"PHIL3.WRL": "5998586dbfa7978d5daf9030e7e06d5fb04a3045fd7defd02c06516e1c8fae3a",
}

func enemyDefinitionBytes(rows []EnemyDefinition) []byte {
	raw := make([]byte, 0, 8*len(rows))
	for _, row := range rows {
		raw = append(raw, row[:]...)
	}
	return raw
}

func decodeEnemyDefinitions(world string, raw []byte) ([]EnemyDefinition, error) {
	if len(raw) != EnemyDefinitionCount*8 {
		return nil, fmt.Errorf("%s WRLD.ENEM needs16 eight-byte rows, got%d bytes", world, len(raw))
	}
	want := originalEnemyDefinitionHashes[world]
	if want == "" || Hash(raw) != want {
		return nil, fmt.Errorf("%s immutable WRLD.ENEM source identity differs", world)
	}
	rows := make([]EnemyDefinition, EnemyDefinitionCount)
	for i := range rows {
		copy(rows[i][:], raw[i*8:(i+1)*8])
	}
	return rows, nil
}

// ValidateEnemyDefinitions checks the complete immutable capability. Historical
// preparations load without it, but current startup must prepare this capability
// before initializing a new hostile-enabled launch.
func (d *MissionGenerationData) ValidateEnemyDefinitions() error {
	if d == nil || len(d.EnemyDefinitions) != len(originalEnemyDefinitionHashes) {
		return fmt.Errorf("immutable world enemy definitions are missing or outdated; prepare32")
	}
	for world := range originalEnemyDefinitionHashes {
		if _, err := decodeEnemyDefinitions(world, enemyDefinitionBytes(d.EnemyDefinitions[world])); err != nil {
			return err
		}
	}
	return nil
}

// EnemyDefinitionsForWorld returns independent rows for the exact frozen world.
// The caller may perform the original quality/environment byte transformation
// without changing the reusable prepared definitions for another launch.
func (d *MissionGenerationData) EnemyDefinitionsForWorld(world string) ([]EnemyDefinition, error) {
	if d == nil {
		return nil, fmt.Errorf("mission enemy definitions unavailable")
	}
	rows, ok := d.EnemyDefinitions[world]
	if !ok {
		return nil, fmt.Errorf("%s immutable enemy definitions unavailable; prepare32", world)
	}
	if _, err := decodeEnemyDefinitions(world, enemyDefinitionBytes(rows)); err != nil {
		return nil, err
	}
	return append([]EnemyDefinition(nil), rows...), nil
}
