package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultChannelName   = "snt-channel"
	DefaultChaincodeName = "snt"
)

var defaultGatewayEndpoints = map[string]string{
	"anmat":     "dns:///localhost:7051",
	"lab":       "dns:///localhost:8051",
	"drogueria": "dns:///localhost:9051",
	"farmacia":  "dns:///localhost:11051",
}

// Profile reúne los datos necesarios para conectarse al Gateway del peer de una
// organización con la identidad User1 emitida por su CA.
type Profile struct {
	Organization    string
	MSPID           string
	PeerHostname    string
	GatewayEndpoint string
	TLSCACertPath   string
	CertificatePath string
	PrivateKeyPath  string
}

type manifest struct {
	Organizations []manifestOrganization `json:"organizations"`
}

type manifestOrganization struct {
	MspID        string `json:"mspId"`
	Slug         string `json:"slug"`
	PeerHostname string `json:"peerHostname"`
	Active       bool   `json:"active"`
}

// FindRepositoryRoot asciende desde start hasta encontrar el manifiesto de red.
func FindRepositoryRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve starting directory: %w", err)
	}

	for {
		manifestPath := filepath.Join(current, "network", "organizations-manifest.json")
		if info, statErr := os.Stat(manifestPath); statErr == nil && !info.IsDir() {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("repository root not found from %q", start)
		}
		current = parent
	}
}

// Resolve carga la organización solicitada desde el manifiesto versionado y
// resuelve los artefactos criptográficos generados por NET-1.
func Resolve(repoRoot, organization, endpointOverride, serverNameOverride string) (Profile, error) {
	slug := strings.ToLower(strings.TrimSpace(organization))
	defaultEndpoint, supported := defaultGatewayEndpoints[slug]
	if !supported {
		return Profile{}, fmt.Errorf(
			"unsupported organization %q: use anmat, lab, drogueria, or farmacia",
			organization,
		)
	}

	organizationData, err := loadOrganization(repoRoot, slug)
	if err != nil {
		return Profile{}, err
	}
	if !organizationData.Active {
		return Profile{}, fmt.Errorf("organization %q is inactive in the network manifest", slug)
	}

	peerHostname := organizationData.PeerHostname
	if serverNameOverride != "" {
		peerHostname = serverNameOverride
	}
	endpoint := defaultEndpoint
	if endpointOverride != "" {
		endpoint = endpointOverride
	}

	organizationRoot := filepath.Join(repoRoot, "network", "organizations", slug)
	userMSP := filepath.Join(
		organizationRoot,
		"users",
		fmt.Sprintf("User1@%s.snt.local", slug),
		"msp",
	)

	certificatePath, err := onlyRegularFile(filepath.Join(userMSP, "signcerts"))
	if err != nil {
		return Profile{}, fmt.Errorf("resolve %s signing certificate: %w", slug, err)
	}
	privateKeyPath, err := onlyRegularFile(filepath.Join(userMSP, "keystore"))
	if err != nil {
		return Profile{}, fmt.Errorf("resolve %s private key: %w", slug, err)
	}

	tlsCACertPath := filepath.Join(
		organizationRoot,
		"peers",
		organizationData.PeerHostname,
		"tls",
		"ca.crt",
	)
	if err := requireRegularFile(tlsCACertPath); err != nil {
		return Profile{}, fmt.Errorf("resolve %s peer TLS CA certificate: %w", slug, err)
	}

	return Profile{
		Organization:    slug,
		MSPID:           organizationData.MspID,
		PeerHostname:    peerHostname,
		GatewayEndpoint: endpoint,
		TLSCACertPath:   tlsCACertPath,
		CertificatePath: certificatePath,
		PrivateKeyPath:  privateKeyPath,
	}, nil
}

func loadOrganization(repoRoot, slug string) (manifestOrganization, error) {
	manifestPath := filepath.Join(repoRoot, "network", "organizations-manifest.json")
	contents, err := os.ReadFile(manifestPath)
	if err != nil {
		return manifestOrganization{}, fmt.Errorf("read network manifest: %w", err)
	}

	var networkManifest manifest
	if err := json.Unmarshal(contents, &networkManifest); err != nil {
		return manifestOrganization{}, fmt.Errorf("decode network manifest: %w", err)
	}

	for _, organization := range networkManifest.Organizations {
		if organization.Slug != slug {
			continue
		}
		if organization.MspID == "" || organization.PeerHostname == "" {
			return manifestOrganization{}, fmt.Errorf(
				"organization %q has an incomplete network manifest entry",
				slug,
			)
		}
		return organization, nil
	}

	return manifestOrganization{}, fmt.Errorf(
		"organization %q is missing from the network manifest",
		slug,
	)
}

func onlyRegularFile(directory string) (string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", err
	}

	var matches []string
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil {
			return "", infoErr
		}
		if info.Mode().IsRegular() {
			matches = append(matches, filepath.Join(directory, entry.Name()))
		}
	}

	if len(matches) != 1 {
		return "", fmt.Errorf("expected exactly one regular file in %q, found %d", directory, len(matches))
	}
	return matches[0], nil
}

func requireRegularFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return &fs.PathError{Op: "stat", Path: path, Err: errors.New("not a regular file")}
	}
	return nil
}
