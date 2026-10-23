package fabric

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/config"
)

func TestNewIdentityLoadsX509Certificate(t *testing.T) {
	certificatePath, _, _ := writeCredentialFixture(t)

	clientIdentity, err := newIdentity(config.Profile{
		MSPID:           "LabMSP",
		CertificatePath: certificatePath,
	})
	if err != nil {
		t.Fatalf("newIdentity() error = %v", err)
	}
	if clientIdentity.MspID() != "LabMSP" {
		t.Fatalf("newIdentity().MspID() = %q", clientIdentity.MspID())
	}
	if !strings.Contains(string(clientIdentity.Credentials()), "BEGIN CERTIFICATE") {
		t.Fatal("newIdentity().Credentials() is not a PEM certificate")
	}
}

func TestNewSignLoadsAndUsesPrivateKey(t *testing.T) {
	_, privateKeyPath, privateKey := writeCredentialFixture(t)

	sign, err := newSign(privateKeyPath)
	if err != nil {
		t.Fatalf("newSign() error = %v", err)
	}
	digest := sha256.Sum256([]byte("proposal"))
	signature, err := sign(digest[:])
	if err != nil {
		t.Fatalf("sign() error = %v", err)
	}
	if !ecdsa.VerifyASN1(&privateKey.PublicKey, digest[:], signature) {
		t.Fatal("sign() returned an invalid ECDSA signature")
	}
}

func TestNewGRPCConnectionRejectsInvalidTLSCA(t *testing.T) {
	certificatePath := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(certificatePath, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := newGRPCConnection(config.Profile{TLSCACertPath: certificatePath})
	if err == nil || !strings.Contains(err.Error(), "contains no valid PEM certificate") {
		t.Fatalf("newGRPCConnection() error = %v", err)
	}
}

func TestNewGRPCConnectionAcceptsValidTLSCA(t *testing.T) {
	certificatePath, _, _ := writeCredentialFixture(t)
	connection, err := newGRPCConnection(config.Profile{
		GatewayEndpoint: "dns:///localhost:65535",
		PeerHostname:    "peer0.lab.snt.local",
		TLSCACertPath:   certificatePath,
	})
	if err != nil {
		t.Fatalf("newGRPCConnection() error = %v", err)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("connection.Close() error = %v", err)
	}
}

func writeCredentialFixture(t *testing.T) (string, string, *ecdsa.PrivateKey) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "peer0.lab.snt.local"},
		DNSNames:              []string{"peer0.lab.snt.local"},
		NotBefore:             time.Unix(0, 0),
		NotAfter:              time.Unix(4102444800, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	certificateDER, err := x509.CreateCertificate(
		rand.Reader,
		template,
		template,
		&privateKey.PublicKey,
		privateKey,
	)
	if err != nil {
		t.Fatal(err)
	}
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}

	directory := t.TempDir()
	certificatePath := filepath.Join(directory, "cert.pem")
	privateKeyPath := filepath.Join(directory, "private_sk")
	if err := os.WriteFile(certificatePath, pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certificateDER,
	}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(privateKeyPath, pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyDER,
	}), 0o600); err != nil {
		t.Fatal(err)
	}

	return certificatePath, privateKeyPath, privateKey
}
