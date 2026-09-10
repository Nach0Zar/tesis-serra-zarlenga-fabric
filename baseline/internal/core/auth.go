package core

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

type credentialInput struct {
	Key   string `json:"key"`
	MSPID string `json:"mspId"`
	Role  string `json:"role"`
}

type Credential struct {
	MSPID string
	Role  string
}

// Credentials conserva solamente el digest de cada API key. Las claves se
// suministran por entorno; nunca se persisten ni se registran en logs.
type Credentials struct {
	byDigest map[[sha256.Size]byte]Credential
}

func ParseCredentials(raw string) (Credentials, error) {
	var entries []credentialInput
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return Credentials{}, fmt.Errorf("SNT_BASELINE_API_KEYS debe ser un array JSON valido: %w", err)
	}
	if len(entries) == 0 {
		return Credentials{}, fmt.Errorf("SNT_BASELINE_API_KEYS debe declarar al menos una credencial")
	}
	allowedRoles := map[string]bool{
		RoleOperator: true, RoleAuditor: true,
		RoleRegulatoryAdmin: true, RoleFinancierAuditor: true,
	}
	byDigest := make(map[[sha256.Size]byte]Credential, len(entries))
	byPair := make(map[string]struct{}, len(entries))
	for index, entry := range entries {
		if entry.Key == "" || entry.MSPID == "" || !allowedRoles[entry.Role] {
			return Credentials{}, fmt.Errorf("credencial %d incompleta o con rol desconocido", index)
		}
		digest := sha256.Sum256([]byte(entry.Key))
		if _, duplicate := byDigest[digest]; duplicate {
			return Credentials{}, fmt.Errorf("la misma API key aparece mas de una vez")
		}
		pair := entry.MSPID + "\x00" + entry.Role
		if _, duplicate := byPair[pair]; duplicate {
			return Credentials{}, fmt.Errorf("debe existir una sola API key por organizacion y rol")
		}
		byDigest[digest] = Credential{MSPID: entry.MSPID, Role: entry.Role}
		byPair[pair] = struct{}{}
	}
	return Credentials{byDigest: byDigest}, nil
}

func (c Credentials) Resolve(key string) (Credential, bool) {
	if key == "" {
		return Credential{}, false
	}
	credential, ok := c.byDigest[sha256.Sum256([]byte(key))]
	return credential, ok
}
