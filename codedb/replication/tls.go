package replication

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type ClusterTLS struct {
	cfg      TLSConfig
	caPool   *x509.CertPool
	caCert   *x509.Certificate
	caKey    *ecdsa.PrivateKey
	mu       sync.RWMutex
	nodeCert *tls.Certificate
}

func NewClusterTLS(cfg TLSConfig) *ClusterTLS { return &ClusterTLS{cfg: cfg} }

func (ct *ClusterTLS) Bootstrap(nodeID, dataDir string) error {
	dir := ct.cfg.AutoGenDir
	if dir == "" {
		dir = filepath.Join(dataDir, "cluster-tls")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("cluster-tls: mkdir %s: %w", dir, err)
	}
	caFile := ct.cfg.CAFile
	if caFile == "" {
		caFile = filepath.Join(dir, "ca.crt")
	}
	caKeyFile := filepath.Join(dir, "ca.key")
	if ct.cfg.CAFile != "" {
		caKeyFile = ""
	}
	if ct.cfg.CAFile != "" {
		if err := ct.loadCA(caFile); err != nil {
			return err
		}
	} else if err := ct.bootstrapCA(caFile, caKeyFile); err != nil {
		return err
	}
	certFile := ct.cfg.CertFile
	if certFile == "" {
		certFile = filepath.Join(dir, "node.crt")
	}
	keyFile := ct.cfg.KeyFile
	if keyFile == "" {
		keyFile = filepath.Join(dir, "node.key")
	}
	if ct.cfg.CertFile != "" && ct.cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return fmt.Errorf("cluster-tls: load node cert: %w", err)
		}
		ct.mu.Lock()
		ct.nodeCert = &cert
		ct.mu.Unlock()
	} else if err := ct.generateNodeCert(nodeID, certFile, keyFile); err != nil {
		return err
	}
	return nil
}

func (ct *ClusterTLS) bootstrapCA(certPath, keyPath string) error {
	if _, err := os.Stat(certPath); err == nil {
		return ct.loadCAWithKey(certPath, keyPath)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("cluster-tls: generate CA key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("cluster-tls: generate serial: %w", err)
	}
	tpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "muninndb-cluster-ca"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(10 * 365 * 24 * time.Hour), KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, IsCA: true, BasicConstraintsValid: true, MaxPathLen: 1}
	certDER, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("cluster-tls: create CA cert: %w", err)
	}
	if err := writePEM(certPath, "CERTIFICATE", certDER); err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("cluster-tls: marshal CA key: %w", err)
	}
	if err := writePEM(keyPath, "EC PRIVATE KEY", keyDER); err != nil {
		return err
	}
	caCert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return fmt.Errorf("cluster-tls: parse CA cert: %w", err)
	}
	ct.caCert = caCert
	ct.caKey = key
	ct.caPool = x509.NewCertPool()
	ct.caPool.AddCert(caCert)
	return nil
}

func (ct *ClusterTLS) loadCA(certPath string) error {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("cluster-tls: read CA cert: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("cluster-tls: no PEM block in %s", certPath)
	}
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("cluster-tls: parse CA cert: %w", err)
	}
	ct.caCert = caCert
	ct.caPool = x509.NewCertPool()
	ct.caPool.AddCert(caCert)
	return nil
}

func (ct *ClusterTLS) loadCAWithKey(certPath, keyPath string) error {
	if err := ct.loadCA(certPath); err != nil {
		return err
	}
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("cluster-tls: read CA key: %w", err)
	}
	block, _ := pem.Decode(keyData)
	if block == nil {
		return fmt.Errorf("cluster-tls: no PEM block in %s", keyPath)
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("cluster-tls: parse CA key: %w", err)
	}
	ct.caKey = key
	return nil
}

func (ct *ClusterTLS) generateNodeCert(nodeID, certPath, keyPath string) error {
	if ct.caCert == nil || ct.caKey == nil {
		return fmt.Errorf("cluster-tls: cannot generate node cert without CA key")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("cluster-tls: generate node key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("cluster-tls: generate serial: %w", err)
	}
	tpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: nodeID}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(365 * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, DNSNames: []string{nodeID}, IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}}
	certDER, err := x509.CreateCertificate(rand.Reader, tpl, ct.caCert, &key.PublicKey, ct.caKey)
	if err != nil {
		return fmt.Errorf("cluster-tls: create node cert: %w", err)
	}
	if err := writePEM(certPath, "CERTIFICATE", certDER); err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("cluster-tls: marshal node key: %w", err)
	}
	if err := writePEM(keyPath, "EC PRIVATE KEY", keyDER); err != nil {
		return err
	}
	tlsCert, err := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		return fmt.Errorf("cluster-tls: build tls.Certificate: %w", err)
	}
	ct.mu.Lock()
	ct.nodeCert = &tlsCert
	ct.mu.Unlock()
	return nil
}

func (ct *ClusterTLS) ServerTLSConfig() (*tls.Config, error) {
	if ct.caPool == nil {
		return nil, fmt.Errorf("cluster-tls: CA not loaded")
	}
	return &tls.Config{GetCertificate: ct.getCertificate, ClientCAs: ct.caPool, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS13}, nil
}

func (ct *ClusterTLS) ClientTLSConfig() (*tls.Config, error) {
	if ct.caPool == nil {
		return nil, fmt.Errorf("cluster-tls: CA not loaded")
	}
	return &tls.Config{GetClientCertificate: ct.getClientCertificate, RootCAs: ct.caPool, MinVersion: tls.VersionTLS13}, nil
}

func (ct *ClusterTLS) RotateCert(nodeID string) error {
	if ct.caCert == nil || ct.caKey == nil {
		return fmt.Errorf("cluster-tls: cannot rotate without CA key")
	}
	dir := ct.cfg.AutoGenDir
	certPath := ct.cfg.CertFile
	if certPath == "" {
		certPath = filepath.Join(dir, "node.crt")
	}
	keyPath := ct.cfg.KeyFile
	if keyPath == "" {
		keyPath = filepath.Join(dir, "node.key")
	}
	return ct.generateNodeCert(nodeID, certPath, keyPath)
}

func (ct *ClusterTLS) getCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	if ct.nodeCert == nil {
		return nil, fmt.Errorf("cluster-tls: no node certificate loaded")
	}
	return ct.nodeCert, nil
}
func (ct *ClusterTLS) getClientCertificate(_ *tls.CertificateRequestInfo) (*tls.Certificate, error) {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	if ct.nodeCert == nil {
		return nil, fmt.Errorf("cluster-tls: no node certificate loaded")
	}
	return ct.nodeCert, nil
}
func (ct *ClusterTLS) CACert() *x509.Certificate { return ct.caCert }
func (ct *ClusterTLS) NodeCert() *tls.Certificate {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.nodeCert
}
func writePEM(path, blockType string, data []byte) error {
	return os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: data}), 0600)
}
