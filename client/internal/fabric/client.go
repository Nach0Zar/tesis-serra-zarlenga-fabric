// Package fabric conecta la CLI con Hyperledger Fabric Gateway.
package fabric

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/client/internal/config"
	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-gateway/pkg/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Client ejecuta transacciones genéricas sobre un contrato Fabric.
type Client struct {
	connection *grpc.ClientConn
	gateway    *client.Gateway
	contract   *client.Contract
}

// Connect abre una conexión gRPC con TLS y crea el Gateway firmado por User1.
func Connect(profile config.Profile, channelName, chaincodeName string, timeout time.Duration) (*Client, error) {
	connection, err := newGRPCConnection(profile)
	if err != nil {
		return nil, err
	}

	clientIdentity, err := newIdentity(profile)
	if err != nil {
		_ = connection.Close()
		return nil, err
	}

	sign, err := newSign(profile.PrivateKeyPath)
	if err != nil {
		_ = connection.Close()
		return nil, err
	}

	gateway, err := client.Connect(
		clientIdentity,
		client.WithSign(sign),
		client.WithClientConnection(connection),
		client.WithEvaluateTimeout(timeout),
		client.WithEndorseTimeout(timeout),
		client.WithSubmitTimeout(timeout),
		client.WithCommitStatusTimeout(timeout),
	)
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("connect Fabric Gateway: %w", err)
	}

	network := gateway.GetNetwork(channelName)
	contract := network.GetContract(chaincodeName)
	return &Client{
		connection: connection,
		gateway:    gateway,
		contract:   contract,
	}, nil
}

// Query evalúa una función sin enviarla al servicio de ordenamiento.
func (c *Client) Query(
	ctx context.Context,
	function string,
	arguments []string,
	transient map[string][]byte,
) ([]byte, error) {
	options := proposalOptions(arguments, transient)
	return c.contract.EvaluateWithContext(ctx, function, options...)
}

// Invoke envía una función, espera su commit y devuelve el payload.
func (c *Client) Invoke(
	ctx context.Context,
	function string,
	arguments []string,
	transient map[string][]byte,
) ([]byte, error) {
	options := proposalOptions(arguments, transient)
	return c.contract.SubmitWithContext(ctx, function, options...)
}

// Close libera Gateway y la conexión gRPC compartida.
func (c *Client) Close() error {
	gatewayErr := c.gateway.Close()
	connectionErr := c.connection.Close()
	if gatewayErr != nil {
		return fmt.Errorf("close Fabric Gateway: %w", gatewayErr)
	}
	if connectionErr != nil {
		return fmt.Errorf("close gRPC connection: %w", connectionErr)
	}
	return nil
}

func proposalOptions(arguments []string, transient map[string][]byte) []client.ProposalOption {
	options := make([]client.ProposalOption, 0, 2)
	if len(arguments) > 0 {
		options = append(options, client.WithArguments(arguments...))
	}
	if len(transient) > 0 {
		options = append(options, client.WithTransient(transient))
	}
	return options
}

func newGRPCConnection(profile config.Profile) (*grpc.ClientConn, error) {
	certificatePEM, err := os.ReadFile(profile.TLSCACertPath)
	if err != nil {
		return nil, fmt.Errorf("read peer TLS CA certificate: %w", err)
	}

	certificatePool := x509.NewCertPool()
	if !certificatePool.AppendCertsFromPEM(certificatePEM) {
		return nil, fmt.Errorf("peer TLS CA certificate contains no valid PEM certificate")
	}

	transportCredentials := credentials.NewClientTLSFromCert(
		certificatePool,
		profile.PeerHostname,
	)
	connection, err := grpc.NewClient(
		profile.GatewayEndpoint,
		grpc.WithTransportCredentials(transportCredentials),
	)
	if err != nil {
		return nil, fmt.Errorf("create TLS gRPC connection: %w", err)
	}
	return connection, nil
}

func newIdentity(profile config.Profile) (*identity.X509Identity, error) {
	certificatePEM, err := os.ReadFile(profile.CertificatePath)
	if err != nil {
		return nil, fmt.Errorf("read client certificate: %w", err)
	}
	certificateBlock, _ := pem.Decode(certificatePEM)
	if certificateBlock == nil {
		return nil, fmt.Errorf("client certificate contains no PEM block")
	}
	certificate, err := x509.ParseCertificate(certificateBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse client certificate: %w", err)
	}

	clientIdentity, err := identity.NewX509Identity(profile.MSPID, certificate)
	if err != nil {
		return nil, fmt.Errorf("create X.509 identity: %w", err)
	}
	return clientIdentity, nil
}

func newSign(privateKeyPath string) (identity.Sign, error) {
	// privateKeyPath proviene exclusivamente del keystore User1 resuelto por
	// config.Resolve, que además exige un único archivo regular.
	//nolint:gosec // lectura intencional de la clave privada configurada
	privateKeyPEM, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read client private key: %w", err)
	}
	privateKey, err := identity.PrivateKeyFromPEM(privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse client private key: %w", err)
	}
	sign, err := identity.NewPrivateKeySign(privateKey)
	if err != nil {
		return nil, fmt.Errorf("create transaction signer: %w", err)
	}
	return sign, nil
}
