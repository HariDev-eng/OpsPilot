package kafka

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
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

	var caCert []byte

	// Prefer the certificate contents supplied by the deployment environment.
	caPEM := os.Getenv("KAFKA_CA_CERT")
	if caPEM != "" {
		caPEM = strings.ReplaceAll(caPEM, `\n`, "\n")
		caCert = []byte(caPEM)
	} else {
		caPath := os.Getenv("KAFKA_CA_CERT_PATH")
		if caPath == "" {
			return nil, fmt.Errorf("KAFKA_CA_CERT or KAFKA_CA_CERT_PATH is required")
		}

		absPath, err := filepath.Abs(caPath)
		if err != nil {
			return nil, fmt.Errorf("resolve Kafka CA path: %w", err)
		}

		caCert, err = os.ReadFile(absPath)
		if err != nil {
			return nil, fmt.Errorf("read Kafka CA certificate at %s: %w", absPath, err)
		}

		log.Printf("Kafka CA certificate loaded from file: %s", absPath)
	}

	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse Kafka CA certificate")
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
	}

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
