package kafka

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	kafkago "github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"
)

func NewTransportFromEnv() (*kafkago.Transport, error) {
	username := os.Getenv("KAFKA_USERNAME")
	password := os.Getenv("KAFKA_PASSWORD")

	if username == "" || password == "" {
		return nil, fmt.Errorf("KAFKA_USERNAME and KAFKA_PASSWORD are required")
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// Optional custom CA certificate supplied by the Kafka provider.
	caPath := os.Getenv("KAFKA_CA_CERT_PATH")
	if caPath == "" {
		return nil, fmt.Errorf("KAFKA_CA_CERT_PATH is not set")
	}

	absPath, err := filepath.Abs(caPath)
	if err != nil {
		return nil, fmt.Errorf("resolve Kafka CA path: %w", err)
	}

	caCert, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read Kafka CA certificate at %s: %w", absPath, err)
	}

	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to add Kafka CA certificate from %s", absPath)
	}

	tlsConfig.RootCAs = roots

	log.Printf("Kafka CA certificate loaded successfully from %s", absPath)

	return &kafkago.Transport{
		SASL: plain.Mechanism{
			Username: username,
			Password: password,
		},
		TLS: tlsConfig,
	}, nil
}

func NewDialer(transport *kafkago.Transport) *kafkago.Dialer {
	return &kafkago.Dialer{
		Timeout:       5 * time.Second,
		DualStack:     true,
		SASLMechanism: transport.SASL,
		TLS:           transport.TLS,
	}
}
